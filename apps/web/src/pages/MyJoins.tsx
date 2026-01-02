import { useRef, useEffect, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { bffClient } from "@/api/bff/client";
import { EventCard } from "@/components/EventCard";
import { LoadingState } from "@/components/LoadingState";
import { ErrorState } from "@/components/ErrorState";
import { EmptyState } from "@/components/EmptyState";
import { Loader2 } from "lucide-react";

type Tab = 'joined' | 'created';

export function MyJoins() {
    const loadMoreRef = useRef<HTMLDivElement>(null);
    const [activeTab, setActiveTab] = useState<Tab>('joined');

    const {
        data,
        fetchNextPage,
        hasNextPage,
        isFetchingNextPage,
        isLoading,
        isError,
        error,
        refetch
    } = useInfiniteQuery({
        queryKey: ['my-activities', activeTab],
        queryFn: ({ pageParam, signal }) => {
            if (activeTab === 'created') {
                return bffClient.listCreatedEvents({ cursor: pageParam, limit: 10 }, signal);
            }
            return bffClient.listMyJoins({ cursor: pageParam, limit: 10 }, signal);
        },
        initialPageParam: undefined as string | undefined,
        getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
        staleTime: 30000,
    });

    // Intersection Observer for Infinite Scroll
    useEffect(() => {
        const observer = new IntersectionObserver(
            (entries) => {
                if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
                    fetchNextPage();
                }
            },
            { threshold: 0.1 }
        );

        const currentTarget = loadMoreRef.current;
        if (currentTarget) observer.observe(currentTarget);

        return () => {
            if (currentTarget) observer.unobserve(currentTarget);
        };
    }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

    const allEvents = data?.pages.flatMap((page) => page?.items || []) || [];

    return (
        <main className="container mx-auto py-8 px-4 sm:px-6 lg:px-8">
            <div className="mb-8">
                <h1 className="text-4xl md:text-5xl font-extrabold tracking-tight mb-4 text-slate-900 dark:text-white">
                    My <span className="text-emerald-600">Activities</span>
                </h1>
                <p className="text-muted-foreground text-lg max-w-2xl mb-6">
                    Manage your event participations and organized events.
                </p>

                {/* Tabs */}
                <div className="flex gap-4 border-b border-border/40">
                    <button
                        onClick={() => setActiveTab('joined')}
                        className={`pb-3 px-1 text-sm font-medium transition-colors border-b-2 ${activeTab === 'joined'
                                ? 'border-emerald-600 text-emerald-600'
                                : 'border-transparent text-muted-foreground hover:text-foreground'
                            }`}
                    >
                        Participating
                    </button>
                    <button
                        onClick={() => setActiveTab('created')}
                        className={`pb-3 px-1 text-sm font-medium transition-colors border-b-2 ${activeTab === 'created'
                                ? 'border-emerald-600 text-emerald-600'
                                : 'border-transparent text-muted-foreground hover:text-foreground'
                            }`}
                    >
                        Organized
                    </button>
                </div>
            </div>

            {isLoading ? (
                <LoadingState />
            ) : isError ? (
                <ErrorState
                    message={error instanceof Error ? error.message : "Failed to load your activities."}
                    onRetry={() => refetch()}
                />
            ) : allEvents.length === 0 ? (
                <EmptyState
                    title={activeTab === 'joined' ? "No activities yet" : "No events created"}
                    description={activeTab === 'joined'
                        ? "You haven't joined any events. Head over to the Discovery feed to find your next adventure!"
                        : "You haven't created any events yet. Host your first event today!"}
                />
            ) : (
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
                    {allEvents.map((event) => (
                        <EventCard key={event.id} event={event} />
                    ))}
                </div>
            )}

            {/* sentinel component */}
            <div ref={loadMoreRef} className="mt-12 py-12 flex justify-center min-h-[120px]">
                {isFetchingNextPage ? (
                    <div className="flex flex-col items-center gap-2 text-muted-foreground animate-pulse">
                        <Loader2 className="h-8 w-8 animate-spin text-emerald-600" />
                        <span className="text-sm font-medium">Loading more...</span>
                    </div>
                ) : hasNextPage ? (
                    <div className="h-1 w-1" />
                ) : allEvents.length > 0 ? (
                    <div className="text-center py-4 px-8 glass-card rounded-full text-muted-foreground text-sm font-medium">
                        End of list.
                    </div>
                ) : null}
            </div>
        </main>
    );
}
