import axios, { AxiosError } from 'axios';
import { tokenStore } from '../auth/tokenStore';
import type { ApiErrorResponse } from '../types/api';
import { ApiErrorCode } from '../types/api';

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

// Request Interceptor: Inject Access Token
apiClient.interceptors.request.use((config) => {
    const token = tokenStore.getToken();
    if (token) {
        config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
});

// Response Interceptor: Normalize Errors
apiClient.interceptors.response.use(
    (response) => response,
    (error: AxiosError<ApiErrorResponse>) => {
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
                data.error.request_id,
                data.error.meta,
                error
            );
        }

        // 3. Fallback for unhandled status codes (e.g. 500 HTML from Nginx, 404 default)
        const status = error.response.status;
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
