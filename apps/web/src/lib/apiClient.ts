import axios, { AxiosError } from 'axios';
import { tokenStore } from '../auth/tokenStore';
import type { ApiErrorResponse, RefreshResponse, BaseResponse } from '../types/api';
import { ApiErrorCode } from '../types/api';
import { authEvents, eventBus } from './events';

/**
 * Custom Error Class for standardized handling
 */
export class ApiError extends Error {
    code: string;
    requestId: string;
    meta?: Record<string, any>;
    originalError?: unknown;

    constructor(code: string, message: string, requestId: string, meta?: any, originalError?: unknown) {
        super(message);
        this.name = 'ApiError';
        this.code = code;
        this.requestId = requestId;
        this.meta = meta;
        this.originalError = originalError;
    }
}

/**
 * Axios Instance Configuration
 * Hardened Rules:
 * 1. baseURL = /api (Proxied via Vite)
 * 2. withCredentials = true (Cookies for Refresh Token)
 */
export const apiClient = axios.create({
    baseURL: '/api',
    withCredentials: true,
    timeout: 10000,
    headers: {
        'Content-Type': 'application/json',
    },
});

// Queue to hold requests while refreshing
let refreshPromise: Promise<void> | null = null;

// Isolated Client for Refresh Calls (No Interceptors)
const refreshClient = axios.create({
    baseURL: '/api',
    withCredentials: true,
    headers: { 'Content-Type': 'application/json' },
});

// Request Interceptor: Inject Access Token & Gate
apiClient.interceptors.request.use(async (config) => {
    // GATE 1: If refreshing, wait (prevents 401 storms)
    if (refreshPromise) {
        try {
            await refreshPromise;
        } catch {
            // If refresh failed, we expect the request to likely fail too,
            // but we don't block it here. The response interceptor will handle the 401.
            // OR we could throw here to fail fast.
            // User feedback led to: "If refreshPromise exists, await refreshPromise; // 不吞"
            // So if refresh failed, this await throws, rejecting the request.
        }
    }

    const token = tokenStore.getToken();
    if (token) {
        config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
});

// Response Interceptor: Normalize Errors & Handle Token Refresh
apiClient.interceptors.response.use(
    (response) => response,
    async (error: AxiosError<ApiErrorResponse>) => {
        const status = error.response?.status;
        const originalRequest = error.config as any;

        // Handle 401 - Silent Refresh
        if (status === 401 && originalRequest && !originalRequest._retry) {
            // 1. Loop Prevention: Hard Stop
            if (originalRequest.url?.includes('/auth/refresh')) {
                return Promise.reject(error);
            }

            // 2. Skip refresh for public endpoints - they don't need auth
            const publicEndpoints = ['/auth/login', '/auth/register', '/feed', '/events/'];
            const isPublicEndpoint = publicEndpoints.some(endpoint => originalRequest.url?.includes(endpoint));

            if (isPublicEndpoint) {
                // For public endpoints, 401 might mean invalid token, but we can still access without auth
                // Remove the Authorization header and retry
                if (originalRequest.headers.Authorization) {
                    delete originalRequest.headers.Authorization;
                    tokenStore.setToken(null); // Clear invalid token
                    originalRequest._retry = true;
                    return apiClient(originalRequest);
                }
                // If no auth header was sent, fall through to error transformation
            } else {
                // 3. Auth Gate: If no token, we assume not logged in (or logout happened).
                // Bootstrap should handle initial load.
                if (!tokenStore.getToken()) {
                    return Promise.reject(error);
                }

                // 4. Idempotency Gate
                const method = originalRequest.method?.toLowerCase();
                const isSafeMethod = ['get', 'head', 'options'].includes(method || '');
                // Axios headers can be object or AxiosHeaders instance
                const idempotencyKey = originalRequest.headers['Idempotency-Key'] ||
                    originalRequest.headers['X-Idempotency-Key'] ||
                    originalRequest.headers?.get?.('Idempotency-Key') ||
                    originalRequest.headers?.get?.('X-Idempotency-Key');

                if (!isSafeMethod && !idempotencyKey) {
                    return Promise.reject(error);
                }

                originalRequest._retry = true;

                // 5. Single-Flight Refresh Logic
                if (!refreshPromise) {
                    refreshPromise = refreshClient.post<BaseResponse<RefreshResponse>>('/auth/refresh')
                        .then((res) => {
                            const data = res.data?.data || (res.data as any);
                            if (data?.tokens?.access_token) {
                                tokenStore.setToken(data.tokens.access_token);
                                eventBus.emit(authEvents.USER_UPDATE, data.user);
                            }
                        })
                        .catch((err) => {
                            // Critical Failure -> Logout
                            tokenStore.setToken(null);
                            eventBus.emit(authEvents.LOGOUT);
                            throw err;
                        })
                        .finally(() => {
                            refreshPromise = null;
                        });
                }

                // 6. Wait & Retry
                await refreshPromise;

                // Re-inject new token
                const newToken = tokenStore.getToken();
                if (newToken) {
                    originalRequest.headers.Authorization = `Bearer ${newToken}`;
                }

                return apiClient(originalRequest);
            }
        }

        // 1. Network / Timeout Errors (No Response)
        if (!error.response) {
            throw new ApiError(
                ApiErrorCode.UPSTREAM_TIMEOUT,
                'Network unreachable. Please check your connection.',
                'client-network-error',
                undefined,
                error
            );
        }

        // 2. Server returned a structured ApiErrorResponse
        const data = error.response.data;
        if (data && data.error && data.error.code) {
            throw new ApiError(
                data.error.code,
                data.error.message,
                data.error.request_id || 'unknown',
                data.error.meta,
                error
            );
        }

        // 3. Fallback for unhandled status codes (e.g. 500 HTML from Nginx, 404 default)
        // status is already defined
        let code: ApiErrorCode = ApiErrorCode.INTERNAL_ERROR;
        let message = `Request failed with status ${status}`;

        if (status === 401) {
            code = ApiErrorCode.UNAUTHENTICATED;
            message = 'Session expired or invalid.';
        } else if (status === 403) {
            code = ApiErrorCode.FORBIDDEN;
            message = 'You do not have permission to perform this action.';
        } else if (status === 504) {
            code = ApiErrorCode.UPSTREAM_TIMEOUT;
            message = 'Upstream service timeout.';
        }

        throw new ApiError(
            code,
            message,
            `gen-${status}`,
            undefined,
            error
        );
    }
);

/**
 * Fetch city suggestions for autocomplete.
 * Returns empty array on error (graceful degradation).
 */
export const getCitySuggestions = async (query: string): Promise<string[]> => {
    if (!query || query.length < 2) return [];

    try {
        const res = await apiClient.get(`/meta/cities?q=${encodeURIComponent(query)}`);
        // Response is {"data": [...]}
        return res.data?.data || [];
    } catch (err) {
        console.error('Failed to fetch city suggestions:', err);
        return [];
    }
};

/**
 * Fetch events created by the current user.
 * Supports status filtering (draft, published).
 */
export const getCreatedEvents = async (status?: string, cursor?: string): Promise<{ items: any[], hasMore: boolean, nextCursor: string }> => {
    try {
        const params = new URLSearchParams();
        if (status) params.append('status', status);
        if (cursor) params.append('cursor', cursor);

        const res = await apiClient.get(`/me/events?${params.toString()}`);
        // res.data is a PaginatedResponse
        return {
            items: res.data?.items || [],
            hasMore: res.data?.has_more || false,
            nextCursor: res.data?.next_cursor || '',
        };
    } catch (err) {
        console.error('Failed to fetch created events:', err);
        throw err;
    }
};

/**
 * Fetch detailed view of an event (public or owner's draft).
 */
export const getEventDetail = async (id: string): Promise<any> => {
    try {
        const res = await apiClient.get(`/events/${id}/view`);
        return res.data;
    } catch (err) {
        console.error(`Failed to fetch event ${id}:`, err);
        throw err;
    }
};

