import { useInfiniteQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/apiClient";
import type { CursorEnvelope, EventCard as EventCardType } from "@/types/api";
import { EventCard } from "@/components/EventCard";
import { LoadingState } from "@/components/LoadingState";
import { ErrorState } from "@/components/ErrorState";
import { EmptyState } from "@/components/EmptyState";
import { Button } from "@/components/ui/button";

export function EventsFeed() {
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
        queryKey: ['feed', 'v2'],
        queryFn: async ({ pageParam }) => {
            // The API returns { data: CursorEnvelope }.
            // Axios res.data is the JSON body.
            // So we need res.data.data to get the envelope.
            const res = await apiClient.get<{ data: CursorEnvelope<EventCardType> }>('/feed', {
                params: {
                    cursor: pageParam,
                    limit: 10,
                },
            });
            return res.data.data;
        },
        initialPageParam: undefined as string | undefined,
        getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
    });

    if (isLoading) return <LoadingState />;
    if (isError) return (
        <ErrorState
            message={error instanceof Error ? error.message : "Failed to load feed."}
            onRetry={() => refetch()}
        />
    );

    const allEvents = data?.pages.flatMap((page) => page?.items || []) || [];

    return (
        <main className="container mx-auto py-8 px-4 sm:px-6 lg:px-8">
            <h1 className="text-3xl font-bold tracking-tight mb-8">Discover Events</h1>

            {allEvents.length === 0 ? (
                <EmptyState title="No events found" description="There represent currently no upcoming events." />
            ) : (
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
                    {allEvents.map((event) => (
                        <EventCard key={event.id} event={event} />
                    ))}
                </div>
            )}

            {hasNextPage && (
                <div className="mt-8 flex justify-center">
                    <Button
                        onClick={() => fetchNextPage()}
                        disabled={isFetchingNextPage}
                        variant="outline"
                        size="lg"
                    >
                        {isFetchingNextPage ? "Loading more..." : "Load More Events"}
                    </Button>
                </div>
            )}
        </main>
    );
}
