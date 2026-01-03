import { useState, useEffect } from "react";
import { Link, useNavigate } from "react-router-dom";
import { getCreatedEvents } from "@/lib/apiClient";
import { Button } from "@/components/ui/button";
import { toast } from "sonner";
import {
    Calendar,
    MapPin,
    Plus,
    FileText,
    CheckCircle2,
    ChevronRight,
    Inbox
} from "lucide-react";

export function MyEvents() {
    const [status, setStatus] = useState<"published" | "draft">("published");
    const [events, setEvents] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const navigate = useNavigate();

    useEffect(() => {
        fetchEvents();
    }, [status]);

    const fetchEvents = async () => {
        setLoading(true);
        try {
            const data = await getCreatedEvents(status);
            setEvents(data.items);
        } catch (err) {
            toast.error("Failed to load your events");
        } finally {
            setLoading(false);
        }
    };

    return (
        <main className="container mx-auto py-12 px-4 max-w-5xl">
            <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 mb-10">
                <div>
                    <h1 className="text-4xl font-extrabold tracking-tight text-slate-900 dark:text-white">
                        My <span className="text-emerald-600">Events</span>
                    </h1>
                    <p className="text-muted-foreground mt-2">
                        Manage the events you've created and published.
                    </p>
                </div>
                <Button
                    onClick={() => navigate("/events/new")}
                    className="rounded-full bg-emerald-600 hover:bg-emerald-700 shadow-lg shadow-emerald-500/20 px-6 h-12 flex items-center gap-2 font-bold uppercase tracking-wider text-xs active:scale-95 transition-transform"
                >
                    <Plus className="w-4 h-4" /> Create Event
                </Button>
            </div>

            {/* Tabs */}
            <div className="flex gap-2 p-1.5 bg-slate-100 dark:bg-slate-900/50 rounded-2xl w-fit mb-8 border border-slate-200 dark:border-white/5">
                <button
                    onClick={() => setStatus("published")}
                    className={`px-6 py-2.5 rounded-xl text-sm font-bold transition-all flex items-center gap-2 ${status === "published"
                        ? "bg-white dark:bg-slate-800 text-emerald-600 shadow-sm"
                        : "text-muted-foreground hover:text-slate-900 dark:hover:text-white"
                        }`}
                >
                    <CheckCircle2 className="w-4 h-4" /> Published
                </button>
                <button
                    onClick={() => setStatus("draft")}
                    className={`px-6 py-2.5 rounded-xl text-sm font-bold transition-all flex items-center gap-2 ${status === "draft"
                        ? "bg-white dark:bg-slate-800 text-emerald-600 shadow-sm"
                        : "text-muted-foreground hover:text-slate-900 dark:hover:text-white"
                        }`}
                >
                    <FileText className="w-4 h-4" /> Drafts
                </button>
            </div>

            {loading ? (
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6 animate-pulse">
                    {[1, 2, 3, 4].map(i => (
                        <div key={i} className="h-48 glass-card rounded-3xl" />
                    ))}
                </div>
            ) : events.length > 0 ? (
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                    {events.map((event) => (
                        <Link
                            key={event.id}
                            to={status === 'published' ? `/events/${event.id}` : `/events/new?id=${event.id}`}
                            className="group glass-card p-6 rounded-3xl border-white/20 backdrop-blur-xl hover:border-emerald-500/50 transition-all flex flex-col h-full bg-white/40 dark:bg-slate-950/40"
                        >
                            <div className="flex justify-between items-start mb-4">
                                <span className="px-3 py-1 bg-slate-100 dark:bg-slate-800 rounded-full text-[10px] font-bold text-slate-500 uppercase tracking-widest border border-slate-200 dark:border-white/5">
                                    {event.category}
                                </span>
                                <div className="p-2 rounded-full bg-slate-50 dark:bg-slate-900 group-hover:bg-emerald-50 dark:group-hover:bg-emerald-950/30 text-slate-400 group-hover:text-emerald-500 transition-colors">
                                    <ChevronRight className="w-4 h-4" />
                                </div>
                            </div>

                            <h3 className="text-xl font-bold text-slate-900 dark:text-white mb-2 group-hover:text-emerald-600 transition-colors line-clamp-1">
                                {event.title}
                            </h3>
                            <p className="text-sm text-muted-foreground line-clamp-2 mb-6 flex-1">
                                {event.description}
                            </p>

                            <div className="flex items-center gap-4 text-[11px] font-bold text-slate-500 dark:text-slate-400 uppercase tracking-wider mt-auto pt-4 border-t border-slate-100 dark:border-white/5">
                                <div className="flex items-center gap-1.5">
                                    <MapPin className="w-3.5 h-3.5 text-emerald-500" />
                                    {event.city}
                                </div>
                                <div className="flex items-center gap-1.5">
                                    <Calendar className="w-3.5 h-3.5 text-emerald-500" />
                                    {new Date(event.start_time).toLocaleDateString()}
                                </div>
                            </div>
                        </Link>
                    ))}
                </div>
            ) : (
                <div className="glass-card p-16 rounded-[40px] text-center border-dashed border-2 border-slate-200 dark:border-white/10">
                    <div className="w-20 h-20 bg-slate-50 dark:bg-slate-900 rounded-3xl flex items-center justify-center mx-auto mb-6 text-slate-300 dark:text-slate-700">
                        <Inbox className="w-10 h-10" />
                    </div>
                    <h2 className="text-2xl font-bold text-slate-900 dark:text-white mb-2">No {status} events found</h2>
                    <p className="text-muted-foreground mb-8 max-w-sm mx-auto">
                        {status === "published"
                            ? "You haven't published any events yet. Get started by creating your first one!"
                            : "Your drafts will appear here. You can save your progress and finish later."}
                    </p>
                    {status === "published" && (
                        <Button
                            onClick={() => navigate("/events/new")}
                            className="rounded-full bg-emerald-600 hover:bg-emerald-700 px-8 h-12 font-bold uppercase tracking-widest text-xs active:scale-95 transition-transform"
                        >
                            Create Your First Event
                        </Button>
                    )}
                </div>
            )}
        </main>
    );
}
