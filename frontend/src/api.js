import { defaultConfig, normalizeAuthResult } from './state.js';

const storageKeys = {
  auth: 'cityevents.auth',
};

export function createApiClient(config = {}, fetchImpl = globalThis.fetch, storage = globalThis.localStorage) {
  const bases = resolveBases(config);
  let memoryAuth = { user: null, accessToken: '' };

  async function request(base, path, options = {}) {
    const { retryOnUnauthorized = false, ...fetchOptions } = options;
    const headers = { ...(options.headers || {}) };
    if (options.body && !headers['Content-Type']) {
      headers['Content-Type'] = 'application/json';
    }
    const response = await fetchImpl(base + path, { ...fetchOptions, credentials: 'include', headers });
    if (response.status === 401 && retryOnUnauthorized) {
      await refresh();
      return request(base, path, {
        ...options,
        headers: { ...(options.headers || {}), ...authHeaders() },
        retryOnUnauthorized: false,
      });
    }
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
    memoryAuth = auth;
    storage?.removeItem?.(storageKeys.auth);
    return auth;
  }

  function loadAuth() {
    return memoryAuth;
  }

  function clearAuth() {
    memoryAuth = { user: null, accessToken: '' };
    storage?.removeItem?.(storageKeys.auth);
  }

  function authHeaders(auth = loadAuth()) {
    return auth.accessToken ? { Authorization: `Bearer ${auth.accessToken}` } : {};
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
    async refresh() {
      return refresh();
    },
    async me() {
      const payload = await request(bases.authBase, '/v1/auth/me', { headers: authHeaders(), retryOnUnauthorized: true });
      return payload.user;
    },
    async logout() {
      await request(bases.authBase, '/v1/auth/logout', {
        method: 'POST',
        headers: authHeaders(),
      });
      clearAuth();
    },
    updateUserRole(userID, role) {
      return request(bases.authBase, `/v1/auth/users/${encodeURIComponent(userID)}/role`, {
        method: 'PATCH',
        headers: authHeaders(),
        body: JSON.stringify({ role }),
        retryOnUnauthorized: true,
      });
    },
    listFeed(city = '') {
      const params = new URLSearchParams({ limit: '20', offset: '0' });
      if (city.trim()) params.set('city', city.trim());
      return request(bases.feedBase, `/v1/feed/events?${params.toString()}`);
    },
    getEvent(eventID) {
      return request(bases.eventBase, `/v1/events/${encodeURIComponent(eventID)}`, {
        headers: authHeaders(),
        retryOnUnauthorized: true,
      });
    },
    createEvent(data) {
      return request(bases.eventBase, '/v1/events', {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify(data),
        retryOnUnauthorized: true,
      });
    },
    joinEvent(eventID) {
      return request(bases.eventBase, `/v1/events/${encodeURIComponent(eventID)}/join`, {
        method: 'POST',
        headers: authHeaders(),
        retryOnUnauthorized: true,
      });
    },
    cancelJoin(eventID) {
      return request(bases.eventBase, `/v1/events/${encodeURIComponent(eventID)}/join`, {
        method: 'DELETE',
        headers: authHeaders(),
        retryOnUnauthorized: true,
      });
    },
    cancelRegistration(eventID, userID) {
      return request(bases.eventBase, `/v1/events/${encodeURIComponent(eventID)}/registrations/${encodeURIComponent(userID)}`, {
        method: 'DELETE',
        headers: authHeaders(),
        retryOnUnauthorized: true,
      });
    },
    createUpload(data) {
      return request(bases.mediaBase, '/v1/media/uploads', {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify(data),
        retryOnUnauthorized: true,
      });
    },
  };

  async function refresh() {
    return saveAuth(await request(bases.authBase, '/v1/auth/refresh', {
      method: 'POST',
      retryOnUnauthorized: false,
    }));
  }
}

function resolveBases(config = {}) {
  const merged = { ...defaultConfig, ...config };
  const apiBase = cleanBase(merged.apiBase);
  return {
    authBase: cleanBase(merged.authBase || apiBase),
    eventBase: cleanBase(merged.eventBase || apiBase),
    feedBase: cleanBase(merged.feedBase || apiBase),
    mediaBase: cleanBase(merged.mediaBase || apiBase),
  };
}

function cleanBase(value) {
  return String(value || '').trim().replace(/\/+$/, '');
}
