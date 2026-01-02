import { useNavigate } from "react-router-dom";
import { Calendar, MapPin, Tag } from "lucide-react";
import type { EventCard as EventCardType } from "@/types/api";

interface EventCardProps {
    event: EventCardType;
}

export function EventCard({ event }: EventCardProps) {
    const navigate = useNavigate();

    return (
        <div
            className="group relative flex flex-col overflow-hidden rounded-lg border bg-card text-card-foreground shadow-sm transition-all hover:shadow-md cursor-pointer"
            onClick={() => navigate(`/events/${event.id}`)}
        >
            <div className="aspect-video w-full overflow-hidden bg-muted">
                {event.cover_image ? (
                    <img
                        src={event.cover_image}
                        alt={event.title}
                        className="h-full w-full object-cover transition-transform group-hover:scale-105"
                        loading="lazy"
                    />
                ) : (
                    <div className="flex h-full w-full items-center justify-center bg-emerald-100/50 text-emerald-600">
                        <Tag className="h-10 w-10 opacity-50" />
                    </div>
                )}
            </div>

            <div className="flex flex-1 flex-col p-4">
                <div className="flex items-start justify-between gap-2">
                    <h3 className="line-clamp-2 text-lg font-semibold tracking-tight text-foreground">
                        {event.title}
                    </h3>
                </div>

                <div className="mt-4 space-y-2 text-sm text-muted-foreground">
                    <div className="flex items-center gap-2">
                        <Calendar className="h-4 w-4 shrink-0 text-emerald-600" />
                        <span>{new Date(event.start_time).toLocaleDateString(undefined, {
                            weekday: 'short', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
                        })}</span>
                    </div>

                    <div className="flex items-center gap-2">
                        <MapPin className="h-4 w-4 shrink-0 text-emerald-600" />
                        <span className="truncate">{event.city}</span>
                    </div>
                </div>

                <div className="mt-auto pt-4 flex gap-2">
                    <span className="inline-flex items-center rounded-full border border-transparent bg-secondary px-2.5 py-0.5 text-xs font-semibold text-secondary-foreground transition-colors hover:bg-secondary/80">
                        {event.category}
                    </span>
                </div>
            </div>
        </div>
    );
}
