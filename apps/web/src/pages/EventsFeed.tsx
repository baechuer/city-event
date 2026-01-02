import { useRef, useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { useInfiniteQuery, keepPreviousData } from "@tanstack/react-query";
import { bffClient } from "@/api/bff/client";
import { EventCard } from "@/components/EventCard";
import { LoadingState } from "@/components/LoadingState";
import { ErrorState } from "@/components/ErrorState";
import { EmptyState } from "@/components/EmptyState";
import { FilterBar } from "@/components/FilterBar";
import { Loader2 } from "lucide-react";

export function EventsFeed() {
    const [searchParams] = useSearchParams();
    const loadMoreRef = useRef<HTMLDivElement>(null);

    // 3.1 Treat URL as the single source of truth
    const category = searchParams.get('category') || 'All';
    const city = searchParams.get('city') || 'All';
    const search = searchParams.get('q') || '';

    const {
        data,
        fetchNextPage,
        hasNextPage,
        isFetchingNextPage,
        isPending,
        isFetching,
        isError,
        error,
        refetch
    } = useInfiniteQuery({
        // 3.2 Key includes filters to force reset on change
        queryKey: ['feed', { category, city, search }],
        queryFn: ({ pageParam, signal }) => bffClient.listFeed({
            cursor: pageParam,
            limit: 10,
            category: category !== 'All' ? category : undefined,
            city: city !== 'All' ? city : undefined,
            q: search || undefined
        }, signal),
        initialPageParam: undefined as string | undefined,
        getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
        // Stale time to prevent flickering during quick filter changes
        staleTime: 5000,
        // Keep showing previous data while fetching new results for a seamless experience
        placeholderData: keepPreviousData,
    });

    // 3.3 Intersection Observer sentinel for infinite scroll
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
        if (currentTarget) {
            observer.observe(currentTarget);
        }

        return () => {
            if (currentTarget) observer.unobserve(currentTarget);
        };
    }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

    const allEvents = data?.pages.flatMap((page) => page?.items || []) || [];

    // Only show the hard LoadingState on initial load (when no data exists at all)
    if (isPending && allEvents.length === 0) return (
        <main className="container mx-auto py-8 px-4">
            <FilterBar />
            <LoadingState />
        </main>
    );

    if (isError) return (
        <main className="container mx-auto py-8 px-4">
            <FilterBar />
            <ErrorState
                message={error instanceof Error ? error.message : "Failed to load feed."}
                onRetry={() => refetch()}
            />
        </main>
    );

    return (
        <main className="container mx-auto py-8 px-4 sm:px-6 lg:px-8">
            <div className="mb-12">
                <h1 className="text-4xl md:text-5xl font-extrabold tracking-tight mb-4 text-slate-900 dark:text-white">
                    Discover <span className="text-emerald-600">Events</span>
                </h1>
                <p className="text-muted-foreground text-lg max-w-2xl">
                    Join amazing activities, meet new people, and explore your city.
                </p>
            </div>

            <FilterBar />

            {allEvents.length === 0 && !isPending ? (
                <EmptyState title="No events found" description="Try adjusting your filters or search query to find more events." />
            ) : (
                <div className={`grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6 transition-opacity duration-300 ${isFetching && !isFetchingNextPage ? 'opacity-50 pointer-events-none' : 'opacity-100'}`}>
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
                        <span className="text-sm font-medium">Loading more events...</span>
                    </div>
                ) : hasNextPage ? (
                    <div className="h-1 w-1" /> // Invisible trigger
                ) : allEvents.length > 0 ? (
                    <div className="text-center py-4 px-8 glass-card rounded-full text-muted-foreground text-sm font-medium">
                        You've reached the end of the matches.
                    </div>
                ) : null}
            </div>
        </main>
    );
}
