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
      user: { id: 'user-1', email: 'user@example.com', displayName: 'User' },
      accessToken: 'token-1',
    });
  };
  const client = createApiClient({ authBase: 'http://auth' }, fetchImpl, memoryStorage());
  const auth = await client.register({ email: 'user@example.com', password: 'pass', displayName: 'User' });
  assert.equal(auth.user.id, 'user-1');
  assert.equal(client.loadAuth().accessToken, 'token-1');
  assert.equal(calls[0].url, 'http://auth/v1/auth/register');
});

test('event mutations use X-User-ID header', async () => {
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
  assert.equal(calls[0].options.headers['X-User-ID'], 'user-1');
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
