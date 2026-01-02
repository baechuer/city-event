import { NavBar } from "@/components/NavBar";
import { EmptyState } from "@/components/EmptyState";

export function MyJoins() {
    return (
        <div className="min-h-screen bg-background">
            <NavBar />
            <main className="container mx-auto py-8 px-4">
                <h1 className="text-3xl font-bold tracking-tight mb-8">My Events</h1>
                <EmptyState
                    title="Coming Soon"
                    description="Your joined events will appear here in Phase 5."
                />
            </main>
        </div>
    );
}
