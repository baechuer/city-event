import { z } from "zod";
import {
    EventCardSchema,
    EventViewSchema,
    ParticipationSchema,
    ActionPolicySchema,
    DegradedInfoSchema,
    JoinStateSchema,
    ErrorSchema
} from "@/api/bff/schemas";

/**
 * 1. Base Response Envelope
 */
export interface BaseResponse<T> {
    data: T;
}

/**
 * Unified API Error Structure
 */
export type ApiErrorResponse = z.infer<typeof ErrorSchema>;

export type RefreshResponse = {
    tokens: {
        access_token: string;
        token_type: string;
        expires_in: number;
    };
    user: User;
}

export interface AuthData {
    tokens: {
        access_token: string;
    };
    user: User;
}

// Enums for stable error codes
export const ApiErrorCode = {
    INVALID_CREDENTIALS: 'invalid_credentials', // 401 login failure
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
 */
export interface CursorParams {
    cursor?: string;
    limit?: number;
}

export type PaginatedResponse<T> = {
    items: T[];
    next_cursor?: string;
    has_more: boolean;
};

/**
 * View Models
 */
export type EventCard = z.infer<typeof EventCardSchema>;
export type EventView = z.infer<typeof EventViewSchema>;
export type Participation = z.infer<typeof ParticipationSchema>;
export type ActionPolicy = z.infer<typeof ActionPolicySchema>;
export type DegradedInfo = z.infer<typeof DegradedInfoSchema>;
export type ParticipationStatus = Participation['status'];
export type JoinState = z.infer<typeof JoinStateSchema>;

export type RefreshResponseLegacy = {
    tokens: {
        access_token: string;
        token_type: string;
        expires_in: number;
    };
    user: User;
}

export interface User {
    id: string;
    email: string;
    name?: string;
    role?: string;
    email_verified?: boolean;
    has_password?: boolean;
}

