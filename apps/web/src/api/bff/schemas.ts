import { z } from "zod";

/**
 * 1. Unified Error Format
 */
export const ErrorSchema = z.object({
    error: z.object({
        code: z.string(),
        message: z.string(),
        request_id: z.string().optional(),
        meta: z.record(z.string(), z.any()).optional(),
    }),
});

/**
 * 2. Pagination Envelope
 * Robust against null items or missing cursors
 */
export const PaginatedResponseSchema = <T extends z.ZodTypeAny>(itemSchema: T) =>
    z.object({
        items: z.array(itemSchema).nullish().transform(v => v ?? []),
        next_cursor: z.string().nullish().catch(null),
        has_more: z.boolean().catch(false),
    }).passthrough();

/**
 * 3. View Models
 */
export const EventCardSchema = z.preprocess(
    (val: any) => {
        if (!val) return val;

        const updates: any = {};

        // Map tags[0] to category if category is missing
        if (val.tags && Array.isArray(val.tags) && val.tags.length > 0 && !val.category) {
            updates.category = val.tags[0];
        }

        // Map cover_image_ids[0] to cover_image if cover_image is missing
        if (val.cover_image_ids && Array.isArray(val.cover_image_ids) && val.cover_image_ids.length > 0 && !val.cover_image) {
            updates.cover_image = val.cover_image_ids[0];
        }

        return { ...val, ...updates };
    },
    z.object({
        id: z.string(),
        title: z.string().catch("Untitled Event"),
        cover_image: z.string().nullish(),
        cover_image_ids: z.array(z.string()).nullish().catch([]),
        start_time: z.string().catch(() => new Date().toISOString()),
        end_time: z.string().nullish().catch(null),
        city: z.string().catch("Unknown City"),
        category: z.string().catch("General"),
    }).passthrough()
);

export const ParticipationSchema = z.object({
    // status is critical but we catch it to "none" if it drifts
    status: z.enum(["none", "joined", "active", "canceled", "waitlisted", "rejected", "expired", "cancelled"])
        .catch("none" as any),
    joined_at: z.string().nullish(),
}).passthrough();

export const ActionPolicySchema = z.object({
    can_join: z.boolean().catch(false),
    can_cancel: z.boolean().catch(false),
    can_cancel_event: z.boolean().catch(false),
    can_unpublish: z.boolean().catch(false),
    can_edit: z.boolean().catch(false),
    reason: z.string().nullish().transform(v => v ?? ""),
}).passthrough();

export const DegradedInfoSchema = z.object({
    participation: z.string().nullish(),
}).passthrough();

export const EventSchema = z.object({
    id: z.string(),
    title: z.string().catch("Untitled Event"),
    description: z.string().catch(""),
    city: z.string().catch(""),
    category: z.string().catch(""),
    cover_image: z.string().nullish().catch(null),
    cover_image_ids: z.array(z.string()).nullish().catch([]),
    start_time: z.string().catch(() => new Date().toISOString()),
    end_time: z.string().nullish().catch(null),
    location: z.string().catch(""),
    capacity: z.number().catch(0),
    active_participants: z.number().catch(0),
    status: z.enum(["draft", "published", "canceled"]).catch("published"),
    owner_id: z.string().catch(""),
    organizer_name: z.string().catch("-"),
    created_by: z.string().catch(""),
}).passthrough();

export const EventViewSchema = z.object({
    event: EventSchema,
    participation: ParticipationSchema.nullable().catch(null),
    actions: ActionPolicySchema.catch({
        can_join: false,
        can_cancel: false,
        can_cancel_event: false,
        can_unpublish: false,
        can_edit: false,
        reason: "internal_error"
    }),
    degraded: DegradedInfoSchema.optional(),
}).passthrough();

export const JoinStateSchema = z.object({
    status: z.string().catch("none"),
    event_id: z.string(),
    user_id: z.string(),
    updated_at: z.string().nullish(),
}).passthrough();

/**
 * 4. Derived Types
 */
export type APIError = z.infer<typeof ErrorSchema>;
export type EventCard = z.infer<typeof EventCardSchema>;
export type EventView = z.infer<typeof EventViewSchema>;
export type Participation = z.infer<typeof ParticipationSchema>;
export type ActionPolicy = z.infer<typeof ActionPolicySchema>;
export type DegradedInfo = z.infer<typeof DegradedInfoSchema>;
export type JoinState = z.infer<typeof JoinStateSchema>;
export type PaginatedEvents = z.infer<ReturnType<typeof PaginatedResponseSchema<typeof EventCardSchema>>>;
