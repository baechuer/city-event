// Strict TypeScript definitions mirroring docs/bff-api.md

/**
 * Unified API Error Structure
 * Matches: docs/bff-api.md Section 1
 */
export interface ApiErrorResponse {
    error: {
        code: string;
        message: string;
        request_id: string;
        meta?: Record<string, any>;
    };
}

// Type Guard for ApiErrorResponse
export function isApiErrorResponse(data: unknown): data is ApiErrorResponse {
    return (
        typeof data === 'object' &&
        data !== null &&
        'error' in data &&
        typeof (data as any).error?.code === 'string'
    );
}

// Enums for stable error codes
export const ApiErrorCode = {
    UNAUTHENTICATED: 'unauthenticated', // 401
    FORBIDDEN: 'forbidden',             // 403
    VALIDATION_FAILED: 'validation_failed', // 400
    CONFLICT_STATE: 'conflict_state',   // 409
    INTERNAL_ERROR: 'internal_error',   // 500
    UPSTREAM_TIMEOUT: 'upstream_timeout', // 504
} as const;

export type ApiErrorCode = typeof ApiErrorCode[keyof typeof ApiErrorCode];

/**
 * Pagination Envelope
 * Matches: docs/bff-api.md Section 2
 */
export interface CursorParams {
    cursor?: string;
    limit?: number;
}

export interface CursorEnvelope<T> {
    items: T[];
    next_cursor: string | null;
    has_more: boolean;
}

/**
 * View Models
 * Matches: docs/bff-api.md Section 4
 */

export type ParticipationStatus = 'active' | 'waitlisted' | 'cancelled' | 'none';

export interface EventCard {
    id: string;
    title: string;
    cover_image?: string;
    start_time: string; // ISO8601
    city: string;
    category: string;
    score?: number;
}

export interface ViewerContext {
    participation_status: ParticipationStatus;
    can_join: boolean;
    can_cancel: boolean;
}

export interface EventView {
    event: EventCard & {
        description: string;
        capacity: number;
        filled_count: number;
        organizer_id: string;
    };
    viewer_context: ViewerContext;
}

export interface User {
    id: string;
    email: string;
    name: string;
}
