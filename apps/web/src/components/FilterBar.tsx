import { useState, useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { SlidersHorizontal } from "lucide-react";
import { Button } from "./ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "@/components/ui/dialog";
import { FilterControls } from "./FilterControls";

const CATEGORIES = [
    "All",
    "Social",
    "Tech",
    "Career",
    "Health",
    "Music",
    "Creative",
    "Sports",
    "Food",
    "General",
    "Other",
];

export function FilterBar() {
    const [searchParams, setSearchParams] = useSearchParams();
    const [open, setOpen] = useState(false);

    // Local state for inputs
    const [localSearch, setLocalSearch] = useState(searchParams.get('q') || '');
    const [localCity, setLocalCity] = useState(searchParams.get('city') || '');
    const [localCategory, setLocalCategory] = useState(searchParams.get('category') || 'All');

    // Sync from URL on mount/update
    useEffect(() => {
        setLocalSearch(searchParams.get('q') || '');
        setLocalCity(searchParams.get('city') || '');
        setLocalCategory(searchParams.get('category') || 'All');
    }, [searchParams]);

    // Desktop: Debounce updates directly to URL
    useEffect(() => {
        const timer = setTimeout(() => {
            const newParams = new URLSearchParams(searchParams);
            const currentQ = searchParams.get('q') || '';
            const currentCity = searchParams.get('city') || '';

            // Check changes
            const qChanged = localSearch !== currentQ;
            const cityChanged = localCity !== currentCity;

            if ((qChanged || cityChanged) && !open) { // Only auto-update if NOT in mobile modal mode
                if (localSearch) newParams.set('q', localSearch);
                else newParams.delete('q');

                if (localCity) newParams.set('city', localCity);
                else newParams.delete('city');

                newParams.delete('cursor');
                setSearchParams(newParams);
            }
        }, 500);

        return () => clearTimeout(timer);
    }, [localSearch, localCity, searchParams, setSearchParams, open]);

    const updateCategory = (val: string) => {
        if (!open) {
            // Desktop immediate update
            const newParams = new URLSearchParams(searchParams);
            if (val === 'All') newParams.delete('category');
            else newParams.set('category', val);
            newParams.delete('cursor');
            setSearchParams(newParams);
            setLocalCategory(val);
        } else {
            // Mobile local update
            setLocalCategory(val);
        }
    };

    // Mobile: Apply all filters at once
    const handleMobileApply = () => {
        const newParams = new URLSearchParams(searchParams);

        if (localSearch) newParams.set('q', localSearch);
        else newParams.delete('q');

        if (localCity) newParams.set('city', localCity);
        else newParams.delete('city');

        if (localCategory && localCategory !== 'All') newParams.set('category', localCategory);
        else newParams.delete('category');

        newParams.delete('cursor');
        setSearchParams(newParams);
        setOpen(false);
    };

    const clearFilters = () => {
        setLocalSearch('');
        setLocalCity('');
        setLocalCategory('All');
        if (!open) {
            setSearchParams({});
        }
    };

    const hasFilters = searchParams.size > 0;

    return (
        <div className="mb-8">
            {/* Desktop View */}
            <div className="hidden lg:block">
                <FilterControls
                    variant="desktop"
                    search={localSearch}
                    onSearchChange={setLocalSearch}
                    city={localCity}
                    onCityChange={setLocalCity}
                    category={localCategory}
                    categories={CATEGORIES}
                    onCategoryChange={updateCategory}
                    onClear={clearFilters}
                    hasFilters={hasFilters}
                />
            </div>

            {/* Mobile View - Floating Action Button & Modal */}
            <div className="lg:hidden">
                {/* 
                   Sticky header/button? Or fixed bottom button? 
                   User asked for "tiny button that stays on screen... press it first then filter"
                */}
                <div className="fixed bottom-6 right-6 z-40">
                    <Dialog open={open} onOpenChange={setOpen}>
                        <DialogTrigger asChild>
                            <Button
                                size="lg"
                                className="h-14 w-14 rounded-full shadow-xl bg-emerald-600 hover:bg-emerald-700 text-white p-0 flex items-center justify-center animate-in zoom-in duration-300"
                            >
                                <SlidersHorizontal className="h-6 w-6" />
                            </Button>
                        </DialogTrigger>
                        <DialogContent className="sm:max-w-[425px] p-6 gap-6 w-[95vw] rounded-2xl max-h-[85vh] overflow-y-auto scrollbar-none no-scrollbar">
                            <DialogHeader>
                                <DialogTitle className="text-2xl font-bold">Filter Events</DialogTitle>
                                <DialogDescription>
                                    Narrow down events by location, category, or keyword.
                                </DialogDescription>
                            </DialogHeader>

                            <FilterControls
                                variant="mobile"
                                search={localSearch}
                                onSearchChange={setLocalSearch}
                                city={localCity}
                                onCityChange={setLocalCity}
                                category={localCategory}
                                categories={CATEGORIES}
                                onCategoryChange={updateCategory}
                                onClear={clearFilters}
                                hasFilters={hasFilters || !!localSearch || !!localCity || localCategory !== 'All'}
                            />

                            <Button onClick={handleMobileApply} className="w-full h-12 text-lg font-bold bg-emerald-600 hover:bg-emerald-700">
                                Apply Filters
                            </Button>
                        </DialogContent>
                    </Dialog>
                </div>
            </div>
        </div>
    );
}
