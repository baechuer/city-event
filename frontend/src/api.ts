import { defaultConfig, normalizeAuthResult, type AuthResult, type RuntimeConfig, type UserRole } from './state';

const csrfCookieName = 'cityevents_csrf';

type FetchLike = (input: string, init?: RequestInit) => Promise<ResponseLike>;

interface ResponseLike {
  ok: boolean;
  status: number;
  json(): Promise<unknown>;
}

export interface EventCreateInput {
  title: string;
  description: string;
  city: string;
  venue: string;
  startsAt: string;
  capacity: number;
}

export interface UploadCreateInput {
  eventId: string;
  filename: string;
  contentType: string;
  sizeBytes: number;
}

export interface ApiClient {
  bases: ResolvedBases;
  loadAuth(): AuthResult;
  clearAuth(): void;
  register(data: Record<string, FormDataEntryValue | string>): Promise<AuthResult>;
  login(data: { email: FormDataEntryValue | string; password: FormDataEntryValue | string }): Promise<AuthResult>;
  refresh(): Promise<AuthResult>;
  me(): Promise<unknown>;
  logout(): Promise<void>;
  updateUserRole(userID: string, role: UserRole | string): Promise<unknown>;
  listFeed(city?: string): Promise<{ events?: unknown[] }>;
  getEvent(eventID: string): Promise<EventDetailPayload>;
  createEvent(data: EventCreateInput): Promise<EventDetailPayload>;
  joinEvent(eventID: string): Promise<unknown>;
  cancelJoin(eventID: string): Promise<unknown>;
  cancelRegistration(eventID: string, userID: string): Promise<unknown>;
  createUpload(data: UploadCreateInput): Promise<MediaUploadPayload>;
}

export interface EventDetailPayload {
  event?: unknown;
  viewerJoinStatus?: string;
  confirmedCount?: number;
}

export interface MediaUploadPayload {
  asset?: {
    id?: string;
    status?: string;
  };
}

interface RequestOptions extends RequestInit {
  retryOnUnauthorized?: boolean;
}

interface ResolvedBases {
  authBase: string;
  eventBase: string;
  feedBase: string;
  mediaBase: string;
}

export function createApiClient(
  config: Partial<RuntimeConfig> = {},
  fetchImpl: FetchLike = globalThis.fetch.bind(globalThis) as FetchLike,
  storage: Pick<Storage, 'removeItem'> | null | undefined = globalThis.localStorage,
): ApiClient {
  const bases = resolveBases(config);
  let memoryAuth: AuthResult = { user: null, accessToken: '' };

  async function request<T>(base: string, path: string, options: RequestOptions = {}): Promise<T> {
    const { retryOnUnauthorized = false, ...fetchOptions } = options;
    const headers = { ...(options.headers as Record<string, string> | undefined) };
    if (options.body && !headers['Content-Type']) {
      headers['Content-Type'] = 'application/json';
    }
    const response = await fetchImpl(base + path, { ...fetchOptions, credentials: 'include', headers });
    if (response.status === 401 && retryOnUnauthorized) {
      await refresh();
      return request<T>(base, path, {
        ...options,
        headers: { ...(options.headers as Record<string, string> | undefined), ...authHeaders() },
        retryOnUnauthorized: false,
      });
    }
    if (response.status === 204) {
      return null as T;
    }
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      const message = backendErrorMessage(payload, response.status);
      throw new Error(message);
    }
    return payload as T;
  }

  function saveAuth(result: unknown): AuthResult {
    const auth = normalizeAuthResult(result);
    memoryAuth = auth;
    storage?.removeItem?.('cityevents.auth');
    return auth;
  }

  function loadAuth(): AuthResult {
    return memoryAuth;
  }

  function clearAuth(): void {
    memoryAuth = { user: null, accessToken: '' };
    storage?.removeItem?.('cityevents.auth');
  }

  function authHeaders(auth = loadAuth()): Record<string, string> {
    return auth.accessToken ? { Authorization: `Bearer ${auth.accessToken}` } : {};
  }

  async function refresh(): Promise<AuthResult> {
    return saveAuth(await request<AuthResult>(bases.authBase, '/v1/auth/refresh', {
      method: 'POST',
      headers: csrfHeaders(),
      retryOnUnauthorized: false,
    }));
  }

  return {
    bases,
    loadAuth,
    clearAuth,
    async register(data) {
      return saveAuth(await request<AuthResult>(bases.authBase, '/v1/auth/register', {
        method: 'POST',
        body: JSON.stringify(data),
      }));
    },
    async login(data) {
      return saveAuth(await request<AuthResult>(bases.authBase, '/v1/auth/login', {
        method: 'POST',
        body: JSON.stringify(data),
      }));
    },
    refresh,
    async me() {
      const payload = await request<{ user: unknown }>(bases.authBase, '/v1/auth/me', { headers: authHeaders(), retryOnUnauthorized: true });
      return payload.user;
    },
    async logout() {
      await request<void>(bases.authBase, '/v1/auth/logout', {
        method: 'POST',
        headers: { ...authHeaders(), ...csrfHeaders() },
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
      return request<{ events?: unknown[] }>(bases.feedBase, `/v1/feed/events?${params.toString()}`);
    },
    getEvent(eventID) {
      return request<EventDetailPayload>(bases.eventBase, `/v1/events/${encodeURIComponent(eventID)}`, {
        headers: authHeaders(),
        retryOnUnauthorized: true,
      });
    },
    createEvent(data) {
      return request<EventDetailPayload>(bases.eventBase, '/v1/events', {
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
      return request<MediaUploadPayload>(bases.mediaBase, '/v1/media/uploads', {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify(data),
        retryOnUnauthorized: true,
      });
    },
  };

  function csrfHeaders(): Record<string, string> {
    const token = readCookie(csrfCookieName);
    return token ? { 'X-CSRF-Token': token } : {};
  }
}

export function resolveBases(config: Partial<RuntimeConfig> = {}): ResolvedBases {
  const merged = { ...defaultConfig, ...config };
  const apiBase = cleanBase(merged.apiBase);
  return {
    authBase: cleanBase(merged.authBase || apiBase),
    eventBase: cleanBase(merged.eventBase || apiBase),
    feedBase: cleanBase(merged.feedBase || apiBase),
    mediaBase: cleanBase(merged.mediaBase || apiBase),
  };
}

function cleanBase(value: string | undefined): string {
  return String(value || '').trim().replace(/\/+$/, '');
}

function readCookie(name: string): string {
  const raw = globalThis.document?.cookie || '';
  const prefix = `${name}=`;
  const match = raw
    .split(';')
    .map((part) => part.trim())
    .find((part) => part.startsWith(prefix));
  if (!match) return '';
  return decodeURIComponent(match.slice(prefix.length));
}

function backendErrorMessage(payload: unknown, status: number): string {
  if (typeof payload === 'object' && payload !== null && 'error' in payload) {
    const error = (payload as { error?: { message?: unknown } }).error;
    if (typeof error?.message === 'string' && error.message.trim()) {
      return error.message;
    }
  }
  return `Request failed with ${status}`;
}
