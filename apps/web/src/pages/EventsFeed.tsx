import { useRef, useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { useInfiniteQuery } from "@tanstack/react-query";
import { bffClient } from "@/api/bff/client";
import { EventCard } from "@/components/EventCard";
import { LoadingState } from "@/components/LoadingState";
import { ErrorState } from "@/components/ErrorState";
import { EmptyState } from "@/components/EmptyState";
import { FilterBar } from "@/components/FilterBar";
import { FeedSidebar } from "@/components/FeedSidebar";
import { MobileFeedNav } from "@/components/MobileFeedNav";
import { Loader2 } from "lucide-react";
import { useAuth } from "@/lib/auth";

export function EventsFeed() {
    const [searchParams, setSearchParams] = useSearchParams();
    const loadMoreRef = useRef<HTMLDivElement>(null);
    const { user } = useAuth();

    // Feed type from URL or default based on auth
    const feedType = searchParams.get('type') || (user ? 'personalized' : 'trending');
    const category = searchParams.get('category') || 'All';
    const city = searchParams.get('city') || 'All';
    const search = searchParams.get('q') || '';

    const handleFeedTypeChange = (type: string) => {
        const newParams = new URLSearchParams(searchParams);
        newParams.set('type', type);
        setSearchParams(newParams);
    };

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
        queryKey: ['feed', { feedType, category, city, search }],
        queryFn: ({ pageParam, signal }) => bffClient.listFeed({
            cursor: pageParam,
            limit: 10,
            type: feedType,
            category: category !== 'All' ? category : undefined,
            city: city !== 'All' ? city : undefined,
            q: search || undefined
        }, signal),
        initialPageParam: undefined as string | undefined,
        getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
        staleTime: 5000,
    });

    // Scroll to top when filters change
    useEffect(() => {
        window.scrollTo(0, 0);
    }, [feedType, category, city, search]);

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

    const feedTitles: Record<string, { title: string; subtitle: string }> = {
        trending: { title: "Trending", subtitle: "Most popular events right now" },
        personalized: { title: "For You", subtitle: "Based on your interests and activity" },
        latest: { title: "All Events", subtitle: "Browse everything happening in your city" },
    };

    const currentFeed = feedTitles[feedType] || feedTitles.trending;

    return (
        <main className="container mx-auto py-4 lg:py-8 px-4 sm:px-6 lg:px-8">
            <MobileFeedNav
                activeType={feedType}
                onTypeChange={handleFeedTypeChange}
                isLoggedIn={!!user}
            />

            <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 lg:gap-12">
                {/* Desktop Sidebar */}
                <div className="hidden lg:block lg:col-span-3 -ml-4 lg:-ml-8 h-full">
                    <FeedSidebar
                        activeType={feedType}
                        onTypeChange={handleFeedTypeChange}
                        isLoggedIn={!!user}
                    />
                </div>

                {/* Main Content */}
                <div className="lg:col-span-9 min-w-0">
                    <div className="mb-8">
                        <h1 className="text-3xl font-bold tracking-tight text-slate-900 dark:text-white">
                            {currentFeed.title}
                        </h1>
                        <p className="text-muted-foreground mt-1">
                            {currentFeed.subtitle}
                        </p>
                    </div>

                    <FilterBar />

                    {isPending && allEvents.length === 0 ? (
                        <LoadingState />
                    ) : isError ? (
                        <ErrorState
                            message={error instanceof Error ? error.message : "Failed to load feed."}
                            onRetry={() => refetch()}
                        />
                    ) : allEvents.length === 0 ? (
                        <EmptyState title="No events found" description="Try adjusting your filters or check back later." />
                    ) : (
                        <div className={`grid grid-cols-1 lg:grid-cols-2 gap-6 transition-opacity duration-300 ${isFetching && !isFetchingNextPage ? 'opacity-50 pointer-events-none' : 'opacity-100'}`}>
                            {allEvents.map((event) => (
                                <EventCard key={event.id} event={event} />
                            ))}
                        </div>
                    )}

                    <div ref={loadMoreRef} className="mt-12 py-8 flex justify-center min-h-[80px]">
                        {isFetchingNextPage ? (
                            <div className="flex items-center gap-2 text-muted-foreground">
                                <Loader2 className="h-5 w-5 animate-spin text-emerald-600" />
                                <span className="text-sm">Loading more...</span>
                            </div>
                        ) : hasNextPage ? (
                            <div className="h-1 w-1" />
                        ) : allEvents.length > 0 ? (
                            <div className="text-center text-muted-foreground text-sm">
                                You've seen all events
                            </div>
                        ) : null}
                    </div>
                </div>
            </div>
        </main>
    );
}

