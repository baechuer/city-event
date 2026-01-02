import { useState, useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { Search, MapPin, Grid, X } from "lucide-react";
import { Input } from "./ui/input";
import { Button } from "./ui/button";

const CATEGORIES = ["All", "Music", "Food", "Technology", "Art", "Sports", "Health"];
const CITIES = ["All", "Beijing", "Shanghai", "Chengdu", "Shenzhen", "Hangzhou"];

export function FilterBar() {
    const [searchParams, setSearchParams] = useSearchParams();

    // Local state for the search input to allow smooth typing
    const [localSearch, setLocalSearch] = useState(searchParams.get('q') || '');

    const category = searchParams.get('category') || 'All';
    const city = searchParams.get('city') || 'All';

    // Debounce search input updates to the URL
    useEffect(() => {
        const timer = setTimeout(() => {
            const newParams = new URLSearchParams(searchParams);
            const currentQ = searchParams.get('q') || '';

            if (localSearch !== currentQ) {
                if (!localSearch) {
                    newParams.delete('q');
                } else {
                    newParams.set('q', localSearch);
                }
                newParams.delete('cursor');
                setSearchParams(newParams);
            }
        }, 400); // 400ms debounce

        return () => clearTimeout(timer);
    }, [localSearch, setSearchParams, searchParams]);

    // Sync local search when URL expands/clears externally
    useEffect(() => {
        setLocalSearch(searchParams.get('q') || '');
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
        setSearchParams({});
    };

    const hasFilters = searchParams.size > 0;

    return (
        <div className="flex flex-col gap-4 mb-8 glass-card p-4 rounded-2xl">
            <div className="flex flex-col md:flex-row gap-4">
                <div className="relative flex-1">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                        placeholder="Search events..."
                        className="pl-10 bg-white/50 dark:bg-slate-900/50 border-white/20"
                        value={localSearch}
                        onChange={(e) => setLocalSearch(e.target.value)}
                    />
                </div>

                <div className="flex flex-wrap gap-4">
                    <div className="relative inline-block w-[140px]">
                        <Grid className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 opacity-50" />
                        <select
                            value={category}
                            className="w-full pl-10 pr-4 py-2 bg-white/50 dark:bg-slate-900/50 border border-white/20 rounded-md text-sm appearance-none focus:outline-none focus:ring-2 focus:ring-primary"
                            onChange={(e) => updateFilter('category', e.target.value)}
                        >
                            {CATEGORIES.map(c => (
                                <option key={c} value={c}>{c}</option>
                            ))}
                        </select>
                    </div>

                    <div className="relative inline-block w-[140px]">
                        <MapPin className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 opacity-50" />
                        <select
                            value={city}
                            className="w-full pl-10 pr-4 py-2 bg-white/50 dark:bg-slate-900/50 border border-white/20 rounded-md text-sm appearance-none focus:outline-none focus:ring-2 focus:ring-primary"
                            onChange={(e) => updateFilter('city', e.target.value)}
                        >
                            {CITIES.map(c => (
                                <option key={c} value={c}>{c}</option>
                            ))}
                        </select>
                    </div>

                    <div className="w-10 flex items-center justify-center">
                        <div className={`transition-all duration-200 ${hasFilters ? 'opacity-100 scale-100' : 'opacity-0 scale-90 pointer-events-none'}`}>
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
                </div>
            </div>
        </div>
    );
}
