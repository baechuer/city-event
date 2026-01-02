import { useState, useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { Search, MapPin, Grid, X } from "lucide-react";
import { Input } from "./ui/input";
import { Button } from "./ui/button";

const CATEGORIES = [
    "All",
    "Social",
    "Tech",
    "Career",
    "Wellness",
    "Outdoors",
    "Arts",
    "Music",
    "Food & Drink",
    "Sports",
];

const SUGGESTED_CITIES = ["Sydney", "Melbourne", "Brisbane", "Perth", "Adelaide"];

export function FilterBar() {
    const [searchParams, setSearchParams] = useSearchParams();

    // Local state for the search input to allow smooth typing
    const [localSearch, setLocalSearch] = useState(searchParams.get('q') || '');
    const [localCity, setLocalCity] = useState(searchParams.get('city') || '');

    const category = searchParams.get('category') || 'All';

    // Debounce search input updates to the URL
    useEffect(() => {
        const timer = setTimeout(() => {
            const newParams = new URLSearchParams(searchParams);
            const currentQ = searchParams.get('q') || '';

            // Optimization: Only update URL if search is empty or at least 2 characters
            const shouldUpdate = !localSearch || localSearch.length >= 2;

            if (shouldUpdate && localSearch !== currentQ) {
                if (!localSearch) {
                    newParams.delete('q');
                } else {
                    newParams.set('q', localSearch);
                }
                newParams.delete('cursor');
                setSearchParams(newParams);
            }
        }, 500);

        return () => clearTimeout(timer);
    }, [localSearch, setSearchParams, searchParams]);

    // Debounce city input updates to the URL
    useEffect(() => {
        const timer = setTimeout(() => {
            const newParams = new URLSearchParams(searchParams);
            const currentCity = searchParams.get('city') || '';

            // Optimization: Only update URL if city is empty or at least 2 characters
            const shouldUpdate = !localCity || localCity.length >= 2;

            if (shouldUpdate && localCity !== currentCity) {
                if (!localCity) {
                    newParams.delete('city');
                } else {
                    newParams.set('city', localCity);
                }
                newParams.delete('cursor');
                setSearchParams(newParams);
            }
        }, 500);

        return () => clearTimeout(timer);
    }, [localCity, setSearchParams, searchParams]);

    // Sync local states when URL changes externally
    useEffect(() => {
        setLocalSearch(searchParams.get('q') || '');
        setLocalCity(searchParams.get('city') || '');
    }, [searchParams]);

    const updateFilter = (key: string, value: string) => {
        const newParams = new URLSearchParams(searchParams);
        if (value === 'All' || !value) {
            newParams.delete(key);
        } else {
            newParams.set(key, value);
        }
        newParams.delete('cursor');
        setSearchParams(newParams);
    };

    const clearFilters = () => {
        setLocalSearch('');
        setLocalCity('');
        setSearchParams({});
    };

    const hasFilters = searchParams.size > 0;

    return (
        <div className="space-y-6 mb-8">
            <div className="flex flex-col md:flex-row gap-4 glass-card p-4 rounded-2xl">
                {/* Search Input */}
                <div className="relative flex-1">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                        placeholder="Search events, topics, or keywords..."
                        className="pl-10 bg-white/50 dark:bg-slate-900/50 border-white/20 focus:ring-emerald-500"
                        value={localSearch}
                        onChange={(e) => setLocalSearch(e.target.value)}
                    />
                </div>

                {/* City Input + Suggestions */}
                <div className="relative w-full md:w-[240px]">
                    <MapPin className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                        list="city-suggestions"
                        placeholder="Location (city or suburb)"
                        className="pl-10 bg-white/50 dark:bg-slate-900/50 border-white/20 focus:ring-emerald-500"
                        value={localCity}
                        onChange={(e) => setLocalCity(e.target.value)}
                    />
                    <datalist id="city-suggestions">
                        {SUGGESTED_CITIES.map(c => (
                            <option key={c} value={c} />
                        ))}
                    </datalist>
                </div>

                {/* Clear Button */}
                <div className={`flex items-center justify-center transition-all duration-200 ${hasFilters ? 'opacity-100 scale-100' : 'opacity-0 scale-90 pointer-events-none w-0'}`}>
                    <Button
                        variant="ghost"
                        size="icon"
                        onClick={clearFilters}
                        className="text-muted-foreground hover:text-destructive"
                        title="Clear all filters"
                    >
                        <X className="h-4 w-4" />
                    </Button>
                </div>
            </div>

            {/* Category Chips/Pills */}
            <div className="flex items-center gap-2 overflow-x-auto pb-2 scrollbar-none no-scrollbar">
                <Grid className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                <div className="flex gap-2">
                    {CATEGORIES.map((c) => {
                        const active = category === c;
                        return (
                            <button
                                key={c}
                                onClick={() => updateFilter('category', c)}
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
