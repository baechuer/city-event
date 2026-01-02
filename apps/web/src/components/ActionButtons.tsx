import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { bffClient } from "@/api/bff/client";
import { Button } from "./ui/button";
import { Loader2 } from "lucide-react";

interface ActionButtonsProps {
    eventId: string;
    status: string;
    canJoin: boolean;
    canCancel: boolean;
    reason?: string;
}

export function ActionButtons({
    eventId,
    status,
    canJoin,
    canCancel,
    reason
}: ActionButtonsProps) {
    const queryClient = useQueryClient();

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

    const isPending = joinMutation.isPending || cancelMutation.isPending || isLockActive;

    const renderButton = () => {
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

        const joinButton = (
            <Button
                variant="default"
                onClick={() => joinMutation.mutate()}
                disabled={isPending || !canJoin}
                size="lg"
                className="w-full sm:w-auto min-w-[200px] h-12 text-base font-bold shadow-lg shadow-emerald-500/20"
            >
                {isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Join Now"}
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
        default: return "Joining is currently unavailable.";
    }
}
