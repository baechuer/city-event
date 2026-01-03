import { cn } from "@/lib/utils";
import { Flame, User, LayoutList } from "lucide-react";

interface MobileFeedNavProps {
    activeType: string;
    onTypeChange: (type: string) => void;
    isLoggedIn?: boolean;
}

const feedTypes = [
    { id: "trending", label: "Trending", icon: Flame },
    { id: "personalized", label: "For You", icon: User, authRequired: true },
    { id: "latest", label: "All", icon: LayoutList },
];

export function MobileFeedNav({ activeType, onTypeChange, isLoggedIn = false }: MobileFeedNavProps) {
    return (
        <div className="flex overflow-x-auto pb-2 gap-2 no-scrollbar lg:hidden sticky top-20 z-30 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 py-3 -mx-4 px-4 sm:-mx-6 sm:px-6 mb-6 border-b border-border/40">
            {feedTypes.map((type) => {
                const Icon = type.icon;
                const isActive = activeType === type.id;
                const isDisabled = type.authRequired && !isLoggedIn;

                return (
                    <button
                        key={type.id}
                        onClick={() => !isDisabled && onTypeChange(type.id)}
                        disabled={isDisabled}
                        className={cn(
                            "flex items-center gap-2 px-4 py-2 rounded-full text-sm font-medium whitespace-nowrap transition-colors",
                            isActive
                                ? "bg-emerald-600 text-white shadow-sm"
                                : "bg-muted/50 text-slate-600 dark:text-slate-400 hover:bg-muted hover:text-slate-900 dark:hover:text-slate-100",
                            isDisabled && "opacity-50 cursor-not-allowed"
                        )}
                    >
                        <Icon className="h-4 w-4" />
                        {type.label}
                    </button>
                );
            })}
        </div>
    );
}
