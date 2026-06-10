import assert from 'node:assert/strict';
import test from 'node:test';
import { createApiClient } from './api.js';

function memoryStorage() {
  const values = new Map();
  return {
    getItem: (key) => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
    removeItem: (key) => values.delete(key),
  };
}

test('register stores auth result', async () => {
  const calls = [];
  const fetchImpl = async (url, options) => {
    calls.push({ url, options });
    return response(201, {
      user: { id: 'user-1', email: 'user@example.com', displayName: 'User', role: 'USER' },
      accessToken: 'token-1',
    });
  };
  const client = createApiClient({ authBase: 'http://auth' }, fetchImpl, memoryStorage());
  const auth = await client.register({ email: 'user@example.com', password: 'pass', displayName: 'User' });
  assert.equal(auth.user.id, 'user-1');
  assert.equal(client.loadAuth().accessToken, 'token-1');
  assert.equal(calls[0].url, 'http://auth/v1/auth/register');
});

test('api base config fans out to gateway routes', async () => {
  const calls = [];
  const client = createApiClient({ apiBase: 'http://cityevents.local/' }, async (url, options) => {
    calls.push({ url, options });
    return response(200, { events: [] });
  }, memoryStorage());
  await client.listFeed('');
  assert.equal(client.bases.feedBase, 'http://cityevents.local');
  assert.equal(calls[0].url, 'http://cityevents.local/v1/feed/events?limit=20&offset=0');
});

test('service base override takes priority over api base', async () => {
  const calls = [];
  const client = createApiClient({
    apiBase: 'http://gateway',
    feedBase: 'http://feed-service/',
  }, async (url, options) => {
    calls.push({ url, options });
    return response(200, { events: [] });
  }, memoryStorage());
  await client.listFeed('');
  assert.equal(client.bases.feedBase, 'http://feed-service');
  assert.equal(calls[0].url, 'http://feed-service/v1/feed/events?limit=20&offset=0');
});

test('event mutations use bearer token through gateway', async () => {
  const storage = memoryStorage();
  storage.setItem('cityevents.auth', JSON.stringify({ user: { id: 'user-1' }, accessToken: 'token-1' }));
  const calls = [];
  const fetchImpl = async (url, options) => {
    calls.push({ url, options });
    return response(200, { status: 'CONFIRMED' });
  };
  const client = createApiClient({ eventBase: 'http://events' }, fetchImpl, storage);
  await client.joinEvent('event-1');
  assert.equal(calls[0].url, 'http://events/v1/events/event-1/join');
  assert.equal(calls[0].options.headers.Authorization, 'Bearer token-1');
  assert.equal(calls[0].options.headers['X-User-ID'], undefined);
});

test('feed list includes city filter', async () => {
  const calls = [];
  const client = createApiClient({ feedBase: 'http://feed' }, async (url, options) => {
    calls.push({ url, options });
    return response(200, { events: [] });
  }, memoryStorage());
  await client.listFeed('Sydney');
  assert.equal(calls[0].url, 'http://feed/v1/feed/events?limit=20&offset=0&city=Sydney');
});

test('admin role update uses bearer token', async () => {
  const storage = memoryStorage();
  storage.setItem('cityevents.auth', JSON.stringify({ user: { id: 'admin-1', role: 'ADMIN' }, accessToken: 'token-1' }));
  const calls = [];
  const client = createApiClient({ authBase: 'http://auth' }, async (url, options) => {
    calls.push({ url, options });
    return response(200, { user: { id: 'user-1', role: 'ORGANIZER' } });
  }, storage);
  await client.updateUserRole('user-1', 'ORGANIZER');
  assert.equal(calls[0].url, 'http://auth/v1/auth/users/user-1/role');
  assert.equal(calls[0].options.headers.Authorization, 'Bearer token-1');
  assert.equal(calls[0].options.headers['Content-Type'], 'application/json');
});

test('cancel registration moderation uses bearer token', async () => {
  const storage = memoryStorage();
  storage.setItem('cityevents.auth', JSON.stringify({ user: { id: 'organizer-1', role: 'ORGANIZER' }, accessToken: 'token-1' }));
  const calls = [];
  const client = createApiClient({ eventBase: 'http://events' }, async (url, options) => {
    calls.push({ url, options });
    return response(200, { status: 'CANCELED' });
  }, storage);
  await client.cancelRegistration('event-1', 'user-1');
  assert.equal(calls[0].url, 'http://events/v1/events/event-1/registrations/user-1');
  assert.equal(calls[0].options.method, 'DELETE');
  assert.equal(calls[0].options.headers.Authorization, 'Bearer token-1');
  assert.equal(calls[0].options.headers['X-User-ID'], undefined);
});

test('request errors expose backend message', async () => {
  const client = createApiClient({ authBase: 'http://auth' }, async () => response(400, {
    error: { message: 'invalid request' },
  }), memoryStorage());
  await assert.rejects(() => client.login({ email: 'bad', password: 'bad' }), /invalid request/);
});

function response(status, payload) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => payload,
  };
}
