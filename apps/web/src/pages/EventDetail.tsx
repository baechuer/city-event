import { useParams, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/apiClient";
import type { EventView } from "@/types/api";
import { LoadingState } from "@/components/LoadingState";
import { ErrorState } from "@/components/ErrorState";
import { ActionButtons } from "@/components/ActionButtons";
import { Calendar, MapPin, Users, User, ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";

export function EventDetail() {
    const { id } = useParams<{ id: string }>();
    const navigate = useNavigate();

    const { data, isLoading, isError, error, refetch } = useQuery({
        queryKey: ['events', id],
        queryFn: async () => {
            const res = await apiClient.get<EventView>(`/events/${id}`);
            return res.data;
        },
        enabled: !!id,
    });

    if (isLoading) return <LoadingState />;
    if (isError || !data) return (
        <ErrorState
            message={isError ? (error as Error).message : "Event not found"}
            onRetry={() => refetch()}
        />
    );

    const { event, viewer_context } = data;

    return (
        <div className="pb-12">

            {/* Hero / Header */}
            <div className="bg-muted/30 border-b">
                <div className="container mx-auto py-8 px-4">
                    <Button variant="ghost" className="mb-4 pl-0 hover:pl-2 transition-all" onClick={() => navigate('/events')}>
                        <ArrowLeft className="mr-2 h-4 w-4" /> Back to Feed
                    </Button>

                    <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
                        {/* Cover Image */}
                        <div className="aspect-video bg-emerald-100 rounded-lg overflow-hidden md:col-span-1 shadow-sm">
                            {event.cover_image ? (
                                <img src={event.cover_image} alt={event.title} className="w-full h-full object-cover" />
                            ) : (
                                <div className="w-full h-full flex items-center justify-center text-emerald-600 font-medium">
                                    No Cover Image
                                </div>
                            )}
                        </div>

                        {/* Info */}
                        <div className="md:col-span-2 space-y-4">
                            <div className="inline-block rounded-full bg-secondary px-3 py-1 text-sm font-medium text-secondary-foreground">
                                {event.category}
                            </div>
                            <h1 className="text-3xl md:text-4xl font-bold tracking-tight">{event.title}</h1>

                            <div className="space-y-2 text-muted-foreground pt-2">
                                <div className="flex items-center gap-2">
                                    <Calendar className="h-5 w-5 text-primary" />
                                    <span>{new Date(event.start_time).toLocaleString(undefined, { dateStyle: 'full', timeStyle: 'short' })}</span>
                                </div>
                                <div className="flex items-center gap-2">
                                    <MapPin className="h-5 w-5 text-primary" />
                                    <span>{event.city}</span>
                                </div>
                                <div className="flex items-center gap-2">
                                    <Users className="h-5 w-5 text-primary" />
                                    <span>{event.filled_count} / {event.capacity} Attendees</span>
                                </div>
                                <div className="flex items-center gap-2">
                                    <User className="h-5 w-5 text-primary" />
                                    <span>Organizer ID: {event.organizer_id}</span>
                                </div>
                            </div>

                            <div className="pt-6">
                                <ActionButtons
                                    eventId={event.id}
                                    status={viewer_context.participation_status}
                                    canJoin={viewer_context.can_join}
                                    canCancel={viewer_context.can_cancel}
                                />
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            {/* Description */}
            <div className="container mx-auto py-8 px-4">
                <h2 className="text-2xl font-semibold mb-4">About this Event</h2>
                <div className="prose dark:prose-invert max-w-none text-muted-foreground leading-relaxed whitespace-pre-line">
                    {event.description}
                </div>
            </div>
        </div>
    );
}
