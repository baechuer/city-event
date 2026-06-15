import assert from 'node:assert/strict';
import test from 'node:test';
import { createApiClient } from './api';

interface Call {
  url: string;
  options?: RequestInit;
}

function memoryStorage() {
  return {
    removeItemCalls: [] as string[],
    removeItem(key: string) {
      this.removeItemCalls.push(key);
    },
  };
}

test('register stores auth result', async () => {
  const calls: Call[] = [];
  const fetchImpl = async (url: string, options?: RequestInit) => {
    calls.push({ url, options });
    return response(201, {
      user: { id: 'user-1', email: 'user@example.com', displayName: 'User', role: 'USER' },
      accessToken: 'token-1',
    });
  };
  const client = createApiClient({ authBase: 'http://auth' }, fetchImpl, memoryStorage());
  const auth = await client.register({ email: 'user@example.com', password: 'pass', displayName: 'User' });
  assert.equal(auth.user?.id, 'user-1');
  assert.equal(client.loadAuth().accessToken, 'token-1');
  assert.equal(calls[0].url, 'http://auth/v1/auth/register');
  assert.equal(calls[0].options?.credentials, 'include');
});

test('api base config fans out to gateway routes', async () => {
  const calls: Call[] = [];
  const client = createApiClient({ apiBase: 'http://cityevents.local/' }, async (url: string, options?: RequestInit) => {
    calls.push({ url, options });
    return response(200, { events: [] });
  }, memoryStorage());
  await client.listFeed('');
  assert.equal(client.bases.feedBase, 'http://cityevents.local');
  assert.equal(calls[0].url, 'http://cityevents.local/v1/feed/events?limit=20&offset=0');
});

test('service base override takes priority over api base', async () => {
  const calls: Call[] = [];
  const client = createApiClient({
    apiBase: 'http://gateway',
    feedBase: 'http://feed-service/',
  }, async (url: string, options?: RequestInit) => {
    calls.push({ url, options });
    return response(200, { events: [] });
  }, memoryStorage());
  await client.listFeed('');
  assert.equal(client.bases.feedBase, 'http://feed-service');
  assert.equal(calls[0].url, 'http://feed-service/v1/feed/events?limit=20&offset=0');
});

test('event mutations use bearer token through gateway', async () => {
  const storage = memoryStorage();
  const calls: Call[] = [];
  const fetchImpl = async (url: string, options?: RequestInit) => {
    calls.push({ url, options });
    if (url.endsWith('/v1/auth/login')) {
      return response(200, { user: { id: 'user-1' }, accessToken: 'token-1' });
    }
    return response(200, { status: 'CONFIRMED' });
  };
  const client = createApiClient({ apiBase: 'http://gateway', authBase: 'http://auth', eventBase: 'http://events' }, fetchImpl, storage);
  await client.login({ email: 'user@example.com', password: 'StrongerPass123' });
  await client.joinEvent('event-1');
  assert.equal(storage.removeItemCalls.includes('cityevents.auth'), true);
  assert.equal(calls[1].url, 'http://events/v1/events/event-1/join');
  assert.equal((calls[1].options?.headers as Record<string, string>).Authorization, 'Bearer token-1');
  assert.equal((calls[1].options?.headers as Record<string, string>)['X-User-ID'], undefined);
  assert.equal(calls[1].options?.credentials, 'include');
});

test('feed list includes city filter', async () => {
  const calls: Call[] = [];
  const client = createApiClient({ feedBase: 'http://feed' }, async (url: string, options?: RequestInit) => {
    calls.push({ url, options });
    return response(200, { events: [] });
  }, memoryStorage());
  await client.listFeed('Sydney');
  assert.equal(calls[0].url, 'http://feed/v1/feed/events?limit=20&offset=0&city=Sydney');
});

test('admin role update uses bearer token', async () => {
  const calls: Call[] = [];
  const client = createApiClient({ authBase: 'http://auth' }, async (url: string, options?: RequestInit) => {
    calls.push({ url, options });
    if (url.endsWith('/v1/auth/login')) {
      return response(200, { user: { id: 'admin-1', role: 'ADMIN' }, accessToken: 'token-1' });
    }
    return response(200, { user: { id: 'user-1', role: 'ORGANIZER' } });
  }, memoryStorage());
  await client.login({ email: 'admin@example.com', password: 'AdminPass12345' });
  await client.updateUserRole('user-1', 'ORGANIZER');
  assert.equal(calls[1].url, 'http://auth/v1/auth/users/user-1/role');
  assert.equal((calls[1].options?.headers as Record<string, string>).Authorization, 'Bearer token-1');
  assert.equal((calls[1].options?.headers as Record<string, string>)['Content-Type'], 'application/json');
});

test('cancel registration moderation uses bearer token', async () => {
  const calls: Call[] = [];
  const client = createApiClient({ authBase: 'http://auth', eventBase: 'http://events' }, async (url: string, options?: RequestInit) => {
    calls.push({ url, options });
    if (url.endsWith('/v1/auth/login')) {
      return response(200, { user: { id: 'organizer-1', role: 'ORGANIZER' }, accessToken: 'token-1' });
    }
    return response(200, { status: 'CANCELED' });
  }, memoryStorage());
  await client.login({ email: 'organizer@example.com', password: 'StrongerPass123' });
  await client.cancelRegistration('event-1', 'user-1');
  assert.equal(calls[1].url, 'http://events/v1/events/event-1/registrations/user-1');
  assert.equal(calls[1].options?.method, 'DELETE');
  assert.equal((calls[1].options?.headers as Record<string, string>).Authorization, 'Bearer token-1');
  assert.equal((calls[1].options?.headers as Record<string, string>)['X-User-ID'], undefined);
});

test('refresh stores access token in memory from HttpOnly cookie flow', async (t) => {
  setDocumentCookie(t, 'cityevents_csrf=csrf-1');
  const calls: Call[] = [];
  const client = createApiClient({ authBase: 'http://auth' }, async (url: string, options?: RequestInit) => {
    calls.push({ url, options });
    return response(200, { user: { id: 'user-1', role: 'USER' }, accessToken: 'fresh-token' });
  }, memoryStorage());
  const auth = await client.refresh();
  assert.equal(auth.accessToken, 'fresh-token');
  assert.equal(client.loadAuth().accessToken, 'fresh-token');
  assert.equal(calls[0].url, 'http://auth/v1/auth/refresh');
  assert.equal(calls[0].options?.method, 'POST');
  assert.equal(calls[0].options?.credentials, 'include');
  assert.equal((calls[0].options?.headers as Record<string, string>)['X-CSRF-Token'], 'csrf-1');
});

test('logout sends bearer token and csrf token', async (t) => {
  setDocumentCookie(t, 'cityevents_csrf=csrf-logout');
  const calls: Call[] = [];
  const client = createApiClient({ authBase: 'http://auth' }, async (url: string, options?: RequestInit) => {
    calls.push({ url, options });
    if (url.endsWith('/v1/auth/login')) {
      return response(200, { user: { id: 'user-1', role: 'USER' }, accessToken: 'token-1' });
    }
    return response(204, {});
  }, memoryStorage());
  await client.login({ email: 'user@example.com', password: 'StrongerPass123' });
  await client.logout();
  assert.equal(calls[1].url, 'http://auth/v1/auth/logout');
  assert.equal(calls[1].options?.method, 'POST');
  assert.equal((calls[1].options?.headers as Record<string, string>).Authorization, 'Bearer token-1');
  assert.equal((calls[1].options?.headers as Record<string, string>)['X-CSRF-Token'], 'csrf-logout');
  assert.equal(client.loadAuth().accessToken, '');
});

test('request errors expose backend message', async () => {
  const client = createApiClient({ authBase: 'http://auth' }, async () => response(400, {
    error: { message: 'invalid request' },
  }), memoryStorage());
  await assert.rejects(() => client.login({ email: 'bad', password: 'bad' }), /invalid request/);
});

function response(status: number, payload: unknown) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => payload,
  };
}

function setDocumentCookie(t: test.TestContext, cookie: string) {
  const hadDocument = Object.hasOwn(globalThis, 'document');
  const previousDocument = globalThis.document;
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    value: { cookie },
  });
  t.after(() => {
    if (hadDocument) {
      Object.defineProperty(globalThis, 'document', {
        configurable: true,
        value: previousDocument,
      });
    } else {
      delete (globalThis as { document?: unknown }).document;
    }
  });
}
