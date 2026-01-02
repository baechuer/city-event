import { apiClient, ApiError } from "@/lib/apiClient";
import {
    EventViewSchema,
    EventCardSchema,
    PaginatedResponseSchema,
    JoinStateSchema,
    type EventView,
    type PaginatedEvents,
    type JoinState
} from "./schemas";
import { z } from "zod";
import { eventBus } from "@/lib/events";

/**
 * Internal helper to parse and validate responses
 */
function parseResponse<T extends z.ZodTypeAny>(schema: T, data: any): z.infer<T> {
    try {
        return schema.parse(data);
    } catch (err) {
        if (err instanceof z.ZodError) {
            console.error("[BFF Contract Mismatch]", err.issues);
            console.error("Raw Data:", data);

            // Trigger a global "API Incompatible" banner via event bus
            eventBus.emit('CONTRACT_MISMATCH', {
                errors: err.issues,
                data
            });

            // We still throw so the UI caller knows it failed
            throw new ApiError(
                "contract_mismatch",
                "The server returned data that doesn't match our expectations. Please refresh or contact support.",
                "client-validation-error",
                { errors: err.issues }
            );
        }
        throw err;
    }
}

/**
 * BFF API Client with strict runtime validation
 */
export const bffClient = {
    /**
     * Get aggregated event detail view
     */
    async getEventView(id: string, signal?: AbortSignal): Promise<EventView> {
        const res = await apiClient.get(`/events/${id}/view`, { signal });
        return parseResponse(EventViewSchema, res.data);
    },

    /**
     * List discovery feed with filters
     */
    async listFeed(params: {
        cursor?: string;
        limit?: number;
        category?: string;
        city?: string;
        q?: string;
    }, signal?: AbortSignal): Promise<PaginatedEvents> {
        const res = await apiClient.get('/feed', { params, signal });
        return parseResponse(PaginatedResponseSchema(EventCardSchema), res.data);
    },

    /**
     * List user's joined events
     */
    async listMyJoins(params: {
        cursor?: string;
        limit?: number;
    }, signal?: AbortSignal): Promise<PaginatedEvents> {
        const res = await apiClient.get('/me/joins', { params, signal });
        return parseResponse(PaginatedResponseSchema(EventCardSchema), res.data);
    },

    /**
     * Join an event (Idempotent)
     */
    async joinEvent(id: string, idempotencyKey: string): Promise<JoinState> {
        const res = await apiClient.post(`/events/${id}/join`, {}, {
            headers: { 'Idempotency-Key': idempotencyKey }
        });
        return parseResponse(JoinStateSchema, res.data);
    },

    /**
     * Cancel join mutation
     */
    async cancelJoin(id: string, idempotencyKey: string): Promise<JoinState> {
        const res = await apiClient.post(`/events/${id}/cancel`, {}, {
            headers: { 'Idempotency-Key': idempotencyKey }
        });
        return parseResponse(JoinStateSchema, res.data);
    },
};
