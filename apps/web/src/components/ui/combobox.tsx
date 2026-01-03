import { useState, useEffect, useRef, type ReactNode } from "react";
import { Check, ChevronsUpDown, Loader2 } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "./command";
import { cn } from "@/lib/utils";

interface ComboboxProps {
    value: string;
    onChange: (value: string) => void;
    fetchSuggestions: (query: string) => Promise<string[]>;
    placeholder?: string;
    icon?: ReactNode;
    className?: string;
}

/**
 * Combobox component with async suggestions, debouncing, and race condition handling.
 * Uses AbortController to cancel stale requests.
 */
export function Combobox({
    value,
    onChange,
    fetchSuggestions,
    placeholder = "Select or type...",
    icon,
    className,
}: ComboboxProps) {
    const [open, setOpen] = useState(false);
    const [suggestions, setSuggestions] = useState<string[]>([]);
    const [loading, setLoading] = useState(false);
    const [inputValue, setInputValue] = useState(value);
    const abortControllerRef = useRef<AbortController | null>(null);
    const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

    // Sync inputValue with external value changes
    useEffect(() => {
        setInputValue(value);
    }, [value]);

    // Debounced fetch with race condition handling using AbortController
    useEffect(() => {
        // Cancel previous request
        if (abortControllerRef.current) {
            abortControllerRef.current.abort();
        }

        // Clear previous timer
        if (debounceTimerRef.current) {
            clearTimeout(debounceTimerRef.current);
        }

        // Don't fetch if input is too short
        if (inputValue.length < 2) {
            setSuggestions([]);
            setLoading(false);
            return;
        }

        // Set loading state immediately for better UX
        setLoading(true);

        // Debounce 250ms
        debounceTimerRef.current = setTimeout(async () => {
            const controller = new AbortController();
            abortControllerRef.current = controller;

            try {
                const results = await fetchSuggestions(inputValue);

                // Only update if this request wasn't aborted
                if (!controller.signal.aborted) {
                    setSuggestions(results);
                    setLoading(false);
                }
            } catch (err: unknown) {
                const error = err as { name?: string };
                if (error.name !== "AbortError" && error.name !== "CanceledError") {
                    console.error("Failed to fetch suggestions:", err);
                    if (!controller.signal.aborted) {
                        setSuggestions([]);
                        setLoading(false);
                    }
                }
            }
        }, 250);

        return () => {
            if (debounceTimerRef.current) {
                clearTimeout(debounceTimerRef.current);
            }
        };
    }, [inputValue, fetchSuggestions]);

    const handleSelect = (selectedValue: string) => {
        onChange(selectedValue);
        setInputValue(selectedValue);
        setOpen(false);
    };

    const handleInputChange = (newValue: string) => {
        setInputValue(newValue);
        onChange(newValue); // Allow free-form input
    };

    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger asChild>
                <button
                    type="button"
                    role="combobox"
                    aria-expanded={open}
                    className={cn(
                        "flex items-center justify-between w-full h-12 px-4",
                        "bg-white/50 dark:bg-slate-900/50 border border-white/30 rounded-xl",
                        "text-sm font-semibold focus:outline-none focus:ring-2 focus:ring-emerald-500/50",
                        "transition-all cursor-pointer hover:bg-white/70 dark:hover:bg-slate-800/70",
                        className
                    )}
                >
                    <div className="flex items-center gap-2 flex-1 overflow-hidden text-left">
                        {icon && <span className="text-muted-foreground">{icon}</span>}
                        <span className={cn("truncate", !inputValue && "text-muted-foreground")}>
                            {inputValue || placeholder}
                        </span>
                    </div>
                    <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                </button>
            </PopoverTrigger>
            <PopoverContent className="w-[--radix-popover-trigger-width] p-0" align="start">
                <Command shouldFilter={false}>
                    <CommandInput
                        placeholder={placeholder}
                        value={inputValue}
                        onValueChange={handleInputChange}
                    />
                    <CommandList>
                        {loading && (
                            <div className="py-6 text-center text-sm text-muted-foreground">
                                <Loader2 className="h-4 w-4 animate-spin inline mr-2" />
                                Loading...
                            </div>
                        )}
                        {!loading && suggestions.length === 0 && inputValue.length >= 2 && (
                            <CommandEmpty>No cities found. You can still use "{inputValue}".</CommandEmpty>
                        )}
                        {!loading && suggestions.length > 0 && (
                            <CommandGroup>
                                {suggestions.map((suggestion) => (
                                    <CommandItem
                                        key={suggestion}
                                        value={suggestion}
                                        onSelect={() => handleSelect(suggestion)}
                                        className="cursor-pointer"
                                    >
                                        <Check
                                            className={cn(
                                                "mr-2 h-4 w-4",
                                                value === suggestion ? "opacity-100" : "opacity-0"
                                            )}
                                        />
                                        {suggestion}
                                    </CommandItem>
                                ))}
                            </CommandGroup>
                        )}
                    </CommandList>
                </Command>
            </PopoverContent>
        </Popover>
    );
}
