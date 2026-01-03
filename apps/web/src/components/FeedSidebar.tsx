import { cn } from "@/lib/utils";
import { Flame, User, LayoutList, TrendingUp } from "lucide-react";

interface FeedSidebarProps {
    activeType: string;
    onTypeChange: (type: string) => void;
    isLoggedIn?: boolean;
}

const feedTypes = [
    { id: "trending", label: "Trending", icon: Flame, description: "Most popular right now" },
    { id: "personalized", label: "For You", icon: User, description: "Based on your interests", authRequired: true },
    { id: "latest", label: "All Events", icon: LayoutList, description: "Browse everything" },
];

export function FeedSidebar({ activeType, onTypeChange, isLoggedIn = false }: FeedSidebarProps) {
    return (
        <aside className="w-full h-full min-h-screen border-r border-slate-200 dark:border-slate-800 bg-white/50 dark:bg-slate-950/50 backdrop-blur-xl">
            <div className="sticky top-16 h-[calc(100vh-4rem)] flex flex-col overflow-y-auto no-scrollbar py-6 px-4">
                <div className="flex items-center gap-3 mb-8 px-2">
                    <div className="p-2 bg-emerald-100 dark:bg-emerald-900/30 rounded-lg">
                        <TrendingUp className="h-5 w-5 text-emerald-600 dark:text-emerald-400" />
                    </div>
                    <h2 className="font-bold text-lg text-slate-900 dark:text-white tracking-tight">Discover</h2>
                </div>

                <div className="space-y-2 flex-1">
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
                                    "group w-full flex items-center gap-3.5 px-4 py-3.5 rounded-2xl text-left transition-all duration-300 border border-transparent",
                                    isActive
                                        ? "bg-emerald-600 text-white shadow-lg shadow-emerald-500/20 scale-[1.02]"
                                        : "hover:bg-white dark:hover:bg-white/5 hover:border-slate-200 dark:hover:border-slate-800 text-slate-600 dark:text-slate-400 hover:shadow-sm",
                                    isDisabled && "opacity-50 cursor-not-allowed hover:scale-100 hover:shadow-none hover:bg-transparent"
                                )}
                            >
                                <Icon className={cn(
                                    "h-5 w-5 transition-colors duration-300",
                                    isActive ? "text-white" : "text-slate-400 group-hover:text-emerald-500"
                                )} />
                                <div className="flex-1 min-w-0">
                                    <div className={cn(
                                        "font-semibold text-sm transition-colors duration-300",
                                        isActive ? "text-white" : "text-slate-700 dark:text-slate-200 group-hover:text-slate-900 dark:group-hover:text-white"
                                    )}>
                                        {type.label}
                                    </div>
                                    <div className={cn(
                                        "text-xs truncate transition-colors duration-300",
                                        isActive ? "text-emerald-100" : "text-slate-500"
                                    )}>
                                        {isDisabled ? "Login required" : type.description}
                                    </div>
                                </div>
                            </button>
                        );
                    })}
                </div>

                {/* Trending Stats Card */}
                <div className="mt-auto pt-6">
                    <div className="p-5 rounded-2xl bg-gradient-to-br from-emerald-50 to-teal-50 dark:from-emerald-900/10 dark:to-teal-900/10 border border-emerald-100 dark:border-emerald-900/20">
                        <h3 className="flex items-center gap-2 text-sm font-bold text-slate-900 dark:text-white mb-4">
                            <span>🔥</span> Hot Right Now
                        </h3>
                        <div className="space-y-3">
                            <div className="flex items-center justify-between p-2 rounded-lg bg-white/50 dark:bg-black/20">
                                <span className="text-xs font-medium text-slate-600 dark:text-slate-400">Events this week</span>
                                <span className="text-sm font-bold text-emerald-600 dark:text-emerald-400">24</span>
                            </div>
                            <div className="flex items-center justify-between p-2 rounded-lg bg-white/50 dark:bg-black/20">
                                <span className="text-xs font-medium text-slate-600 dark:text-slate-400">People joining</span>
                                <span className="text-sm font-bold text-emerald-600 dark:text-emerald-400">1.2k</span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </aside>
    );
}
