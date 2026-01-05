import { Input } from "./ui/input";
import { Combobox } from "./ui/combobox";
import { Button } from "./ui/button";
import { Search, MapPin, X, Grid } from "lucide-react";
import { getCitySuggestions } from "@/lib/apiClient";
import { cn } from "@/lib/utils";

interface FilterControlsProps {
    search: string;
    onSearchChange: (val: string) => void;
    city: string;
    onCityChange: (val: string) => void;
    category: string;
    categories: string[];
    onCategoryChange: (val: string) => void;
    onClear: () => void;
    hasFilters: boolean;
    className?: string;
    variant?: "desktop" | "mobile";
}

export function FilterControls({
    search,
    onSearchChange,
    city,
    onCityChange,
    category,
    categories,
    onCategoryChange,
    onClear,
    hasFilters,
    className,
    variant = "desktop"
}: FilterControlsProps) {
    if (variant === "mobile") {
        return (
            <div className={cn("space-y-6", className)}>
                <div className="space-y-2">
                    <label className="text-xs font-semibold uppercase text-muted-foreground">Search</label>
                    <div className="relative">
                        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                        <Input
                            placeholder="Events, topics..."
                            className="pl-10 h-12"
                            value={search}
                            onChange={(e) => onSearchChange(e.target.value)}
                        />
                    </div>
                </div>

                <div className="space-y-2">
                    <label className="text-xs font-semibold uppercase text-muted-foreground">Location</label>
                    <Combobox
                        value={city}
                        onChange={onCityChange}
                        fetchSuggestions={getCitySuggestions}
                        placeholder="City or suburb"
                        icon={<MapPin className="w-4 h-4" />}
                        className="h-12"
                    />
                </div>

                <div className="space-y-2">
                    <label className="text-xs font-semibold uppercase text-muted-foreground">Category</label>
                    <div className="grid grid-cols-2 gap-2">
                        {categories.map((c) => (
                            <button
                                key={c}
                                onClick={() => onCategoryChange(c)}
                                className={cn(
                                    "px-4 py-3 rounded-xl text-sm font-medium transition-all text-left",
                                    category === c
                                        ? "bg-emerald-600 text-white shadow-md shadow-emerald-200/50"
                                        : "bg-secondary/50 text-muted-foreground hover:bg-secondary hover:text-foreground"
                                )}
                            >
                                {c}
                            </button>
                        ))}
                    </div>
                </div>

                {hasFilters && (
                    <Button
                        variant="outline"
                        className="w-full mt-4 border-dashed"
                        onClick={onClear}
                    >
                        <X className="w-4 h-4 mr-2" /> Clear Filters
                    </Button>
                )}
            </div>
        );
    }

    // Desktop Layout
    return (
        <div className={cn("space-y-6", className)}>
            <div className="flex flex-col md:flex-row gap-4 glass-card p-4 rounded-2xl">
                <div className="relative flex-1">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                        placeholder="Search events, topics, or keywords..."
                        className="pl-10 bg-white/50 dark:bg-slate-900/50 border-white/20 focus:ring-emerald-500"
                        value={search}
                        onChange={(e) => onSearchChange(e.target.value)}
                    />
                </div>

                <div className="w-full md:w-[240px]">
                    <Combobox
                        value={city}
                        onChange={onCityChange}
                        fetchSuggestions={getCitySuggestions}
                        placeholder="Location (city or suburb)"
                        icon={<MapPin className="w-4 h-4" />}
                    />
                </div>

                <div className={`flex items-center justify-center transition-all duration-200 ${hasFilters ? 'opacity-100 scale-100' : 'opacity-0 scale-90 pointer-events-none w-0'}`}>
                    <Button
                        variant="ghost"
                        size="icon"
                        onClick={onClear}
                        className="text-muted-foreground hover:text-destructive"
                        title="Clear all filters"
                    >
                        <X className="h-4 w-4" />
                    </Button>
                </div>
            </div>

            <div className="flex items-center gap-2 overflow-x-auto pb-2 scrollbar-none no-scrollbar">
                <Grid className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                <div className="flex gap-2">
                    {categories.map((c) => {
                        const active = category === c;
                        return (
                            <button
                                key={c}
                                onClick={() => onCategoryChange(c)}
                                className={`
                                    px-4 py-1.5 rounded-full text-sm font-medium whitespace-nowrap transition-all duration-200
                                    ${active
                                        ? 'bg-emerald-600 text-white shadow-md shadow-emerald-200 dark:shadow-none scale-105'
                                        : 'bg-white/80 dark:bg-slate-800/80 text-muted-foreground hover:bg-white dark:hover:bg-slate-700 hover:text-foreground border border-white/20'}
                                `}
                            >
                                {c}
                            </button>
                        );
                    })}
                </div>
            </div>
        </div>
    );
}
