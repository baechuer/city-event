import { defaultConfig, normalizeAuthResult } from './state.js';

const storageKeys = {
  auth: 'cityevents.auth',
};

export function createApiClient(config = {}, fetchImpl = globalThis.fetch, storage = globalThis.localStorage) {
  const bases = { ...defaultConfig, ...config };

  async function request(base, path, options = {}) {
    const headers = { ...(options.headers || {}) };
    if (options.body && !headers['Content-Type']) {
      headers['Content-Type'] = 'application/json';
    }
    const response = await fetchImpl(base + path, { ...options, headers });
    if (response.status === 204) {
      return null;
    }
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      const message = payload?.error?.message || `Request failed with ${response.status}`;
      throw new Error(message);
    }
    return payload;
  }

  function saveAuth(result) {
    const auth = normalizeAuthResult(result);
    storage?.setItem?.(storageKeys.auth, JSON.stringify(auth));
    return auth;
  }

  function loadAuth() {
    const raw = storage?.getItem?.(storageKeys.auth);
    if (!raw) return { user: null, accessToken: '' };
    try {
      return normalizeAuthResult(JSON.parse(raw));
    } catch {
      return { user: null, accessToken: '' };
    }
  }

  function clearAuth() {
    storage?.removeItem?.(storageKeys.auth);
  }

  function authHeaders(auth = loadAuth()) {
    return auth.accessToken ? { Authorization: `Bearer ${auth.accessToken}` } : {};
  }

  function userHeaders(auth = loadAuth()) {
    return auth.user?.id ? { 'X-User-ID': auth.user.id } : {};
  }

  return {
    bases,
    loadAuth,
    clearAuth,
    async register(data) {
      return saveAuth(await request(bases.authBase, '/v1/auth/register', {
        method: 'POST',
        body: JSON.stringify(data),
      }));
    },
    async login(data) {
      return saveAuth(await request(bases.authBase, '/v1/auth/login', {
        method: 'POST',
        body: JSON.stringify(data),
      }));
    },
    async me() {
      const payload = await request(bases.authBase, '/v1/auth/me', { headers: authHeaders() });
      return payload.user;
    },
    async logout() {
      await request(bases.authBase, '/v1/auth/logout', {
        method: 'POST',
        headers: authHeaders(),
      });
      clearAuth();
    },
    listFeed(city = '') {
      const params = new URLSearchParams({ limit: '20', offset: '0' });
      if (city.trim()) params.set('city', city.trim());
      return request(bases.feedBase, `/v1/feed/events?${params.toString()}`);
    },
    getEvent(eventID) {
      return request(bases.eventBase, `/v1/events/${encodeURIComponent(eventID)}`, {
        headers: userHeaders(),
      });
    },
    createEvent(data) {
      return request(bases.eventBase, '/v1/events', {
        method: 'POST',
        headers: userHeaders(),
        body: JSON.stringify(data),
      });
    },
    joinEvent(eventID) {
      return request(bases.eventBase, `/v1/events/${encodeURIComponent(eventID)}/join`, {
        method: 'POST',
        headers: userHeaders(),
      });
    },
    cancelJoin(eventID) {
      return request(bases.eventBase, `/v1/events/${encodeURIComponent(eventID)}/join`, {
        method: 'DELETE',
        headers: userHeaders(),
      });
    },
    createUpload(data) {
      return request(bases.mediaBase, '/v1/media/uploads', {
        method: 'POST',
        headers: userHeaders(),
        body: JSON.stringify(data),
      });
    },
  };
}
