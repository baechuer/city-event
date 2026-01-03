import { useEffect } from "react";
import { useNavigate, Link } from "react-router-dom";
import { useAuth } from "@/lib/auth";
import { useQuery } from "@tanstack/react-query";
import { bffClient } from "@/api/bff/client";
import { EventCard } from "@/components/EventCard";
import { Button } from "@/components/ui/button";
import { ArrowRight, Calendar, Users, Sparkles, MapPin } from "lucide-react";

export function LandingPage() {
    const { isAuthenticated, loading } = useAuth();
    const navigate = useNavigate();

    // Auto-redirect authenticated users to /events
    useEffect(() => {
        if (!loading && isAuthenticated) {
            navigate("/events", { replace: true });
        }
    }, [isAuthenticated, loading, navigate]);

    // Fetch featured events
    const { data: featuredEvents } = useQuery({
        queryKey: ['featured-events'],
        queryFn: ({ signal }) => bffClient.listFeed({ limit: 6 }, signal),
        enabled: !loading && !isAuthenticated, // Only fetch for unauthenticated users
    });

    // Show nothing while checking auth or redirecting
    if (loading || isAuthenticated) {
        return null;
    }

    const events = featuredEvents?.items || [];

    return (
        <div className="min-h-screen">
            {/* Hero Section */}
            <section className="relative overflow-hidden bg-gradient-to-br from-emerald-50 via-white to-blue-50 dark:from-slate-900 dark:via-slate-800 dark:to-slate-900">
                <div className="absolute inset-0 bg-grid-slate-100 [mask-image:linear-gradient(0deg,white,rgba(255,255,255,0.6))] dark:bg-grid-slate-700/25" />

                <div className="relative container mx-auto px-4 sm:px-6 lg:px-8 py-20 md:py-32">
                    <div className="max-w-4xl mx-auto text-center">
                        <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300 text-sm font-medium mb-6">
                            <Sparkles className="h-4 w-4" />
                            <span>Join thousands of event-goers</span>
                        </div>

                        <h1 className="text-5xl md:text-7xl font-extrabold tracking-tight mb-6">
                            Discover Events,
                            <br />
                            <span className="text-emerald-600 dark:text-emerald-400">Connect with People</span>
                        </h1>

                        <p className="text-xl md:text-2xl text-muted-foreground mb-10 max-w-2xl mx-auto">
                            Find amazing activities in your city, meet new people, and create unforgettable memories.
                        </p>

                        <div className="flex flex-col sm:flex-row gap-4 justify-center">
                            <Button asChild size="lg" className="text-lg px-8 py-6">
                                <Link to="/register">
                                    Get Started
                                    <ArrowRight className="ml-2 h-5 w-5" />
                                </Link>
                            </Button>
                            <Button asChild size="lg" variant="outline" className="text-lg px-8 py-6">
                                <Link to="/events">
                                    Browse Events
                                </Link>
                            </Button>
                        </div>
                    </div>
                </div>
            </section>

            {/* Featured Events Section */}
            {events.length > 0 && (
                <section className="py-20 bg-white dark:bg-slate-900">
                    <div className="container mx-auto px-4 sm:px-6 lg:px-8">
                        <div className="text-center mb-12">
                            <h2 className="text-3xl md:text-4xl font-bold mb-4">
                                Trending Events
                            </h2>
                            <p className="text-lg text-muted-foreground max-w-2xl mx-auto">
                                Check out what's happening in your city this week
                            </p>
                        </div>

                        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6 mb-8">
                            {events.map((event) => (
                                <EventCard key={event.id} event={event} />
                            ))}
                        </div>

                        <div className="text-center">
                            <Button asChild size="lg" variant="outline">
                                <Link to="/events">
                                    View All Events
                                    <ArrowRight className="ml-2 h-5 w-5" />
                                </Link>
                            </Button>
                        </div>
                    </div>
                </section>
            )}

            {/* Features Section */}
            <section className="py-20 bg-slate-50 dark:bg-slate-800/50">
                <div className="container mx-auto px-4 sm:px-6 lg:px-8">
                    <div className="text-center mb-16">
                        <h2 className="text-3xl md:text-4xl font-bold mb-4">
                            Why Choose CityEvents?
                        </h2>
                        <p className="text-lg text-muted-foreground max-w-2xl mx-auto">
                            Everything you need to discover and join amazing events
                        </p>
                    </div>

                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-8">
                        <FeatureCard
                            icon={<MapPin className="h-8 w-8" />}
                            title="Discover Local Events"
                            description="Find activities happening right in your neighborhood"
                        />
                        <FeatureCard
                            icon={<Calendar className="h-8 w-8" />}
                            title="Easy RSVP"
                            description="Join events with just one click and manage your schedule"
                        />
                        <FeatureCard
                            icon={<Users className="h-8 w-8" />}
                            title="Meet New People"
                            description="Connect with like-minded individuals and expand your social circle"
                        />
                        <FeatureCard
                            icon={<Sparkles className="h-8 w-8" />}
                            title="Personalized Feed"
                            description="Get recommendations based on your interests and preferences"
                        />
                    </div>
                </div>
            </section>

            {/* CTA Section */}
            <section className="py-20 bg-gradient-to-r from-emerald-600 to-blue-600 text-white">
                <div className="container mx-auto px-4 sm:px-6 lg:px-8 text-center">
                    <h2 className="text-3xl md:text-5xl font-bold mb-6">
                        Ready to Get Started?
                    </h2>
                    <p className="text-xl md:text-2xl mb-10 opacity-90 max-w-2xl mx-auto">
                        Join our community and start discovering amazing events today
                    </p>
                    <Button asChild size="lg" variant="secondary" className="text-lg px-8 py-6">
                        <Link to="/register">
                            Create Free Account
                            <ArrowRight className="ml-2 h-5 w-5" />
                        </Link>
                    </Button>
                </div>
            </section>
        </div>
    );
}

function FeatureCard({ icon, title, description }: { icon: React.ReactNode; title: string; description: string }) {
    return (
        <div className="flex flex-col items-center text-center p-6 rounded-xl bg-white dark:bg-slate-900 shadow-sm hover:shadow-md transition-shadow">
            <div className="p-3 rounded-full bg-emerald-100 dark:bg-emerald-900/30 text-emerald-600 dark:text-emerald-400 mb-4">
                {icon}
            </div>
            <h3 className="text-xl font-semibold mb-2">{title}</h3>
            <p className="text-muted-foreground">{description}</p>
        </div>
    );
}
