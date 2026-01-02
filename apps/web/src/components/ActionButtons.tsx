import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiClient } from "@/lib/apiClient";
import { Button } from "./ui/button";
import type { ParticipationStatus } from "@/types/api";

interface ActionButtonsProps {
    eventId: string;
    status: ParticipationStatus;
    canJoin: boolean;
    canCancel: boolean;
    onStatusChange?: (newStatus: ParticipationStatus) => void;
}

export function ActionButtons({
    eventId,
    status,
    canJoin,
    canCancel,
    onStatusChange
}: ActionButtonsProps) {
    const queryClient = useQueryClient();
    const [isPending, setIsPending] = useState(false);

    // Helper to generate Idempotency Key
    const generateIdempotencyKey = () => crypto.randomUUID();

    const joinMutation = useMutation({
        mutationFn: async () => {
            await apiClient.post(`/events/${eventId}/join`, {}, {
                headers: {
                    'X-Idempotency-Key': generateIdempotencyKey()
                }
            });
        },
        onMutate: () => {
            setIsPending(true);
            // Optimistic update could go here, but for now we rely on invalidation or just local loading state
        },
        onSuccess: () => {
            toast.success("Successfully joined event!");
            queryClient.invalidateQueries({ queryKey: ['events', eventId] });
            queryClient.invalidateQueries({ queryKey: ['feed'] });
            if (onStatusChange) onStatusChange('active');
        },
        onError: () => {
            // Toast handled by global QueryClient handler
        },
        onSettled: () => {
            setIsPending(false);
        }
    });

    const cancelMutation = useMutation({
        mutationFn: async () => {
            await apiClient.post(`/events/${eventId}/cancel`, {}, {
                headers: {
                    'X-Idempotency-Key': generateIdempotencyKey()
                }
            });
        },
        onMutate: () => {
            setIsPending(true);
        },
        onSuccess: () => {
            toast.info("Participation cancelled.");
            queryClient.invalidateQueries({ queryKey: ['events', eventId] });
            queryClient.invalidateQueries({ queryKey: ['feed'] });
            if (onStatusChange) onStatusChange('cancelled');
        },
        onSettled: () => {
            setIsPending(false);
        }
    });

    if (status === 'active' || status === 'waitlisted') {
        return (
            <Button
                variant="destructive"
                onClick={() => cancelMutation.mutate()}
                disabled={isPending || !canCancel}
            >
                {isPending ? "Concelling..." : "Cancel Participation"}
            </Button>
        );
    }

    return (
        <Button
            variant="default" // Emerald Primary
            onClick={() => joinMutation.mutate()}
            disabled={isPending || !canJoin}
            size="lg"
            className="w-full sm:w-auto"
        >
            {isPending ? "Joining..." : "Join Event"}
        </Button>
    );
}
