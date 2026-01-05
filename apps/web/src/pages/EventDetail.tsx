import { useParams, useNavigate } from "react-router-dom";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { bffClient } from "@/api/bff/client";
import { LoadingState } from "@/components/LoadingState";
import { ErrorState } from "@/components/ErrorState";
import { ActionButtons } from "@/components/ActionButtons";
import { getPublicUrl } from "@/lib/mediaApi";
import { Calendar, MapPin, Users, User, ArrowLeft, AlertCircle, FileText, ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";

export function EventDetail() {
    const { id } = useParams<{ id: string }>();
    const navigate = useNavigate();
    const [currentImageIndex, setCurrentImageIndex] = useState(0);

    const { data, isLoading, isError, error, refetch } = useQuery({
        queryKey: ['events', 'view', id],
        queryFn: ({ signal }) => bffClient.getEventView(id!, signal),
        enabled: !!id,
        // 5.2 Aggressive revalidation during degradation
        refetchInterval: (query) => {
            const data = query.state.data;
            if (data?.degraded?.participation) return 15000; // 15s recovery check
            return false;
        },
        staleTime: 10000,
    });

    if (isLoading) return <LoadingState />;
    if (isError || !data) return (
        <ErrorState
            message={isError ? (error as Error).message : "Event not found"}
            onRetry={() => refetch()}
        />
    );

    const { event, participation, actions, degraded } = data;

    // Support both single cover_image and array cover_image_ids
    const coverImages: string[] = [];
    if (event.cover_image_ids && event.cover_image_ids.length > 0) {
        event.cover_image_ids.forEach((id: string) => {
            coverImages.push(getPublicUrl(id, 'event_cover', '800'));
        });
    } else if (event.cover_image) {
        coverImages.push(
            event.cover_image.startsWith('http')
                ? event.cover_image
                : getPublicUrl(event.cover_image, 'event_cover', '800')
        );
    }

    const nextImage = () => {
        setCurrentImageIndex((prev) => (prev + 1) % coverImages.length);
    };

    const prevImage = () => {
        setCurrentImageIndex((prev) => (prev - 1 + coverImages.length) % coverImages.length);
    };

    return (
        <div className="pb-24">
            {/* 5.1 Degraded Mode Banner */}
            {degraded?.participation && (
                <div className="bg-amber-500 text-white py-3 px-4 flex items-center justify-center gap-3 animate-in fade-in slide-in-from-top duration-500">
                    <AlertCircle className="h-5 w-5" />
                    <span className="text-sm font-medium">
                        Participation services are currently slow. You can still view event details, but joining may be delayed.
                    </span>
                </div>
            )}

            {/* Unpublished/Draft Banner */}
            {event.status === 'draft' && (
                <div className="bg-blue-600 dark:bg-blue-700 text-white py-3 px-4 flex items-center justify-center gap-3 animate-in fade-in slide-in-from-top duration-500">
                    <FileText className="h-5 w-5" />
                    <span className="text-sm font-medium">
                        This is an unpublished draft. It is not visible to the public.
                    </span>
                    <Button
                        variant="secondary"
                        size="sm"
                        className="ml-2 h-7 text-xs font-bold uppercase tracking-widest bg-white text-blue-700 hover:bg-blue-50"
                        onClick={() => navigate(`/events/new?id=${event.id}`)}
                    >
                        Edit Draft
                    </Button>
                </div>
            )}

            {/* Hero / Header */}
            <div className="relative overflow-hidden bg-emerald-600/5 border-b min-h-[450px] flex items-center">
                <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_right,rgba(16,185,129,0.1),transparent)]" />

                <div className="container mx-auto py-12 px-4 relative z-10">
                    <Button variant="ghost" className="mb-8 pl-0 hover:pl-2 transition-all hover:bg-transparent text-emerald-700 dark:text-emerald-400 group" onClick={() => navigate('/events')}>
                        <ArrowLeft className="mr-2 h-4 w-4 transition-transform group-hover:-translate-x-1" /> Back to Discover
                    </Button>

                    <div className="grid grid-cols-1 lg:grid-cols-3 gap-12 items-center">
                        {/* Cover Image Carousel */}
                        <div className="aspect-[4/3] glass-card rounded-3xl lg:col-span-1 shadow-2xl p-2 group overflow-hidden relative">
                            <div className="w-full h-full rounded-2xl overflow-hidden bg-emerald-100 dark:bg-emerald-950/50 flex items-center justify-center relative">
                                {coverImages.length > 0 ? (
                                    <>
                                        <img
                                            src={coverImages[currentImageIndex]}
                                            alt={`${event.title} - Image ${currentImageIndex + 1}`}
                                            className="w-full h-full object-cover transition-transform duration-700 group-hover:scale-105"
                                        />

                                        {/* Navigation Arrows (only show if multiple images) */}
                                        {coverImages.length > 1 && (
                                            <>
                                                <button
                                                    onClick={prevImage}
                                                    className="absolute left-3 top-1/2 -translate-y-1/2 w-10 h-10 rounded-full bg-white/80 dark:bg-black/50 flex items-center justify-center shadow-lg opacity-0 group-hover:opacity-100 transition-opacity hover:bg-white dark:hover:bg-black/70"
                                                >
                                                    <ChevronLeft className="h-6 w-6 text-slate-700 dark:text-white" />
                                                </button>
                                                <button
                                                    onClick={nextImage}
                                                    className="absolute right-3 top-1/2 -translate-y-1/2 w-10 h-10 rounded-full bg-white/80 dark:bg-black/50 flex items-center justify-center shadow-lg opacity-0 group-hover:opacity-100 transition-opacity hover:bg-white dark:hover:bg-black/70"
                                                >
                                                    <ChevronRight className="h-6 w-6 text-slate-700 dark:text-white" />
                                                </button>

                                                {/* Dots Indicator */}
                                                <div className="absolute bottom-3 left-1/2 -translate-x-1/2 flex gap-2">
                                                    {coverImages.map((_, idx) => (
                                                        <button
                                                            key={idx}
                                                            onClick={() => setCurrentImageIndex(idx)}
                                                            className={`w-2.5 h-2.5 rounded-full transition-all ${idx === currentImageIndex
                                                                    ? 'bg-white scale-110 shadow-lg'
                                                                    : 'bg-white/50 hover:bg-white/70'
                                                                }`}
                                                        />
                                                    ))}
                                                </div>
                                            </>
                                        )}
                                    </>
                                ) : (
                                    <div className="text-emerald-600/20 font-bold text-4xl">
                                        CityEvents
                                    </div>
                                )}
                            </div>
                        </div>

                        {/* Info */}
                        <div className="lg:col-span-2 space-y-8">
                            <div className="flex flex-wrap gap-3">
                                <span className="inline-block rounded-full bg-emerald-50 dark:bg-emerald-900/30 px-5 py-2 text-xs font-bold uppercase tracking-widest text-emerald-700 dark:text-emerald-400 border border-emerald-100 dark:border-emerald-800/30">
                                    {event.category}
                                </span>
                            </div>

                            <h1 className="text-5xl md:text-6xl lg:text-7xl font-extrabold tracking-tight text-slate-900 dark:text-white leading-[1.1]">
                                {event.title}
                            </h1>

                            <div className="grid grid-cols-1 sm:grid-cols-2 gap-5 pt-4">
                                <div className="flex items-center gap-4 glass-card px-5 py-4 rounded-2xl">
                                    <div className="p-3 rounded-xl bg-emerald-100 dark:bg-emerald-900/30">
                                        <Calendar className="h-6 w-6 text-emerald-600" />
                                    </div>
                                    <div className="flex flex-col">
                                        <span className="text-[10px] uppercase font-bold text-muted-foreground tracking-widest leading-none mb-1">Date & Time</span>
                                        <span className="text-base font-semibold">
                                            {new Date(event.start_time).toLocaleString(undefined, { dateStyle: 'long', timeStyle: 'short' })}
                                            {event.end_time && (
                                                <>
                                                    {" - "}
                                                    {new Date(event.start_time).toDateString() === new Date(event.end_time).toDateString()
                                                        ? new Date(event.end_time).toLocaleTimeString(undefined, { timeStyle: 'short' })
                                                        : new Date(event.end_time).toLocaleString(undefined, { dateStyle: 'long', timeStyle: 'short' })
                                                    }
                                                </>
                                            )}
                                        </span>
                                    </div>
                                </div>
                                <div className="flex items-center gap-4 glass-card px-5 py-4 rounded-2xl">
                                    <div className="p-3 rounded-xl bg-emerald-100 dark:bg-emerald-900/30">
                                        <MapPin className="h-6 w-6 text-emerald-600" />
                                    </div>
                                    <div className="flex flex-col">
                                        <span className="text-[10px] uppercase font-bold text-muted-foreground tracking-widest leading-none mb-1">Local Venue</span>
                                        <span className="text-base font-semibold truncate">{event.city}</span>
                                    </div>
                                </div>
                                <div className="flex items-center gap-4 glass-card px-5 py-4 rounded-2xl">
                                    <div className="p-3 rounded-xl bg-emerald-100 dark:bg-emerald-900/30">
                                        <Users className="h-6 w-6 text-emerald-600" />
                                    </div>
                                    <div className="flex flex-col">
                                        <span className="text-[10px] uppercase font-bold text-muted-foreground tracking-widest leading-none mb-1">Availability</span>
                                        <span className="text-base font-semibold">{event.capacity - event.active_participants} / {event.capacity} Spots Remaining</span>
                                    </div>
                                </div>
                                <div className="flex items-center gap-4 glass-card px-5 py-4 rounded-2xl">
                                    <div className="p-3 rounded-xl bg-emerald-100 dark:bg-emerald-900/30">
                                        <User className="h-6 w-6 text-emerald-600" />
                                    </div>
                                    <div className="flex flex-col">
                                        <span className="text-[10px] uppercase font-bold text-muted-foreground tracking-widest leading-none mb-1">Hosted By</span>
                                        <span className="text-base font-semibold truncate">{event.organizer_name}</span>
                                    </div>
                                </div>
                            </div>

                            <div className="pt-10">
                                <ActionButtons
                                    eventId={event.id}
                                    status={participation?.status || 'none'}
                                    canJoin={actions.can_join && !degraded?.participation} // 5.1 Write Blocking
                                    canCancel={actions.can_cancel && !degraded?.participation}
                                    canCancelEvent={actions.can_cancel_event}
                                    canUnpublish={actions.can_unpublish}
                                    canEdit={actions.can_edit}
                                    reason={actions.reason}
                                />
                                {degraded?.participation && (
                                    <p className="mt-3 text-sm text-amber-600 font-medium">
                                        Join/Cancel actions temporarily restricted due to service maintenance.
                                    </p>
                                )}
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            {/* Description */}
            <div className="container mx-auto py-16 px-4">
                <div className="max-w-4xl space-y-8">
                    <h2 className="text-3xl font-bold tracking-tight">Event Details</h2>
                    <div className="prose dark:prose-invert max-w-none text-muted-foreground leading-relaxed text-lg whitespace-pre-line">
                        {event.description}
                    </div>
                </div>
            </div>
        </div>
    );
}
