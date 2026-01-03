import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { bffClient } from "@/api/bff/client";
import { Button } from "./ui/button";
import { Loader2, Edit3, Trash2, EyeOff } from "lucide-react";

interface ActionButtonsProps {
    eventId: string;
    status: string;
    canJoin: boolean;
    canCancel: boolean;
    canCancelEvent?: boolean;
    canUnpublish?: boolean;
    canEdit?: boolean;
    reason?: string;
}

export function ActionButtons({
    eventId,
    status,
    canJoin,
    canCancel,
    canCancelEvent,
    canUnpublish,
    canEdit,
    reason
}: ActionButtonsProps) {
    const queryClient = useQueryClient();
    const navigate = useNavigate();

    // 4.2 Local mutation lock to prevent spikes
    const [isLockActive, setIsLockActive] = useState(false);

    // Helper to generate Idempotency Key
    const generateIdempotencyKey = () => crypto.randomUUID();

    const joinMutation = useMutation({
        mutationFn: () => {
            setIsLockActive(true);
            return bffClient.joinEvent(eventId, generateIdempotencyKey());
        },
        onSuccess: () => {
            toast.success("Successfully joined event!");
            queryClient.invalidateQueries({ queryKey: ['events', 'view', eventId] });
            queryClient.invalidateQueries({ queryKey: ['feed'] });
            queryClient.invalidateQueries({ queryKey: ['my-joins'] });
        },
        onError: (error: any) => {
            toast.error(error?.message || "Failed to join event.");
        },
        onSettled: () => {
            setIsLockActive(false);
        }
    });

    const cancelMutation = useMutation({
        mutationFn: () => {
            setIsLockActive(true);
            return bffClient.cancelJoin(eventId, generateIdempotencyKey());
        },
        onSuccess: () => {
            toast.info("Participation cancelled.");
            queryClient.invalidateQueries({ queryKey: ['events', 'view', eventId] });
            queryClient.invalidateQueries({ queryKey: ['feed'] });
            queryClient.invalidateQueries({ queryKey: ['my-joins'] });
        },
        onError: (error: any) => {
            toast.error(error?.message || "Failed to cancel participation.");
        },
        onSettled: () => {
            setIsLockActive(false);
        }
    });

    const cancelEventMutation = useMutation({
        mutationFn: () => {
            setIsLockActive(true);
            return bffClient.cancelEvent(eventId);
        },
        onSuccess: () => {
            toast.success("Event has been cancelled.");
            queryClient.invalidateQueries({ queryKey: ['events', 'view', eventId] });
            queryClient.invalidateQueries({ queryKey: ['feed'] });
            queryClient.invalidateQueries({ queryKey: ['my-events'] });
        },
        onError: (error: any) => {
            toast.error(error?.message || "Failed to cancel event.");
        },
        onSettled: () => {
            setIsLockActive(false);
        }
    });

    const unpublishMutation = useMutation({
        mutationFn: () => {
            setIsLockActive(true);
            return bffClient.unpublishEvent(eventId);
        },
        onSuccess: () => {
            toast.success("Event unpublished. It is now a draft.");
            queryClient.invalidateQueries({ queryKey: ['events', 'view', eventId] });
            queryClient.invalidateQueries({ queryKey: ['feed'] });
            queryClient.invalidateQueries({ queryKey: ['my-events'] });
        },
        onError: (error: any) => {
            toast.error(error?.message || "Failed to unpublish event.");
        },
        onSettled: () => {
            setIsLockActive(false);
        }
    });

    const isPending = joinMutation.isPending || cancelMutation.isPending || cancelEventMutation.isPending || unpublishMutation.isPending || isLockActive;

    const renderButton = () => {
        // Organizer View
        if (canEdit || canCancelEvent || canUnpublish) {
            return (
                <div className="flex flex-wrap gap-4">
                    {canEdit && (
                        <Button
                            variant="outline"
                            onClick={() => navigate(`/events/new?id=${eventId}`)}
                            disabled={isPending}
                            className="w-full sm:w-auto min-w-[140px] border-emerald-200 hover:bg-emerald-50 text-emerald-700"
                        >
                            <Edit3 className="mr-2 h-4 w-4" /> Edit Event
                        </Button>
                    )}
                    {canUnpublish && (
                        <Button
                            variant="secondary"
                            onClick={() => {
                                if (window.confirm("Unpublish this event? It will be hidden from the public feed but existing participants remain.")) {
                                    unpublishMutation.mutate();
                                }
                            }}
                            disabled={isPending}
                            className="w-full sm:w-auto min-w-[140px]"
                        >
                            {isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <><EyeOff className="mr-2 h-4 w-4" /> Unpublish</>}
                        </Button>
                    )}
                    {canCancelEvent && (
                        <Button
                            variant="destructive"
                            onClick={() => {
                                if (window.confirm("Are you sure you want to cancel this event? This cannot be undone.")) {
                                    cancelEventMutation.mutate();
                                }
                            }}
                            disabled={isPending}
                            className="w-full sm:w-auto min-w-[140px]"
                        >
                            {isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <><Trash2 className="mr-2 h-4 w-4" /> Cancel Event</>}
                        </Button>
                    )}
                </div>
            );
        }

        // Participant View
        if (status === 'joined' || status === 'active' || status === 'waitlisted') {
            return (
                <Button
                    variant="destructive"
                    onClick={() => cancelMutation.mutate()}
                    disabled={isPending || !canCancel}
                    className="w-full sm:w-auto min-w-[140px]"
                >
                    {isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Cancel Participation"}
                </Button>
            );
        }

        const getReasonLabel = (reason: string) => {
            switch (reason) {
                case 'event_ended': return "Event Ended";
                case 'event_full': return "Event Full";
                case 'event_closed': return "Registration Closed";
                case 'auth_required': return "Login to Join";
                case 'is_organizer': return "You are Host";
                default: return "Unavailable";
            }
        };

        const joinButtonText = (!canJoin && reason) ? getReasonLabel(reason) : "Join Now";

        const joinButton = (
            <Button
                variant="default"
                onClick={() => joinMutation.mutate()}
                disabled={isPending || !canJoin}
                size="lg"
                className="w-full sm:w-auto min-w-[200px] h-12 text-base font-bold shadow-lg shadow-emerald-500/20"
            >
                {isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : joinButtonText}
            </Button>
        );

        // 4.2 Gating with reasoning (Native title as fallback for Tooltip)
        if (!canJoin && reason && status === 'none') {
            return (
                <div className="w-full sm:w-auto" title={getReasonMessage(reason)}>
                    {joinButton}
                </div>
            );
        }

        return joinButton;
    };

    return (
        <div className="flex flex-col sm:flex-row gap-4">
            {renderButton()}
        </div>
    );
}

/**
 * Human-readable mapping for ActionPolicy reasons
 */
function getReasonMessage(reason: string): string {
    switch (reason) {
        case 'auth_required': return "Please login to join this event.";
        case 'participation_unavailable': return "Join service is currently undergoing maintenance.";
        case 'event_full': return "This event has reached its maximum capacity.";
        case 'event_canceled': return "This event has been canceled by the organizer.";
        case 'event_past': return "This event has already ended.";
        case 'is_organizer': return "You are the organizer of this event.";
        default: return "Joining is currently unavailable.";
    }
}
