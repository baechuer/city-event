import { useState, useEffect } from "react";
import { eventBus } from "@/lib/events";
import { AlertTriangle, X } from "lucide-react";
import { Button } from "./ui/button";

export function BffBanner() {
    const [mismatch, setMismatch] = useState<any | null>(null);

    useEffect(() => {
        const unsub = eventBus.on('CONTRACT_MISMATCH', (data) => {
            setMismatch(data);
        });
        return unsub;
    }, []);

    if (!mismatch) return null;

    return (
        <div className="fixed bottom-4 right-4 z-[100] max-w-md animate-in slide-in-from-bottom-5 duration-300">
            <div className="glass-card border-destructive/50 bg-destructive/10 backdrop-blur-xl p-4 rounded-2xl shadow-2xl flex gap-4">
                <div className="p-2 bg-destructive/20 rounded-xl h-fit">
                    <AlertTriangle className="h-6 w-6 text-destructive" />
                </div>
                <div className="flex-1 space-y-1">
                    <h4 className="font-bold text-slate-900 dark:text-white">API Contract Mismatch</h4>
                    <p className="text-xs text-muted-foreground leading-relaxed">
                        Validation failed at <code className="text-destructive font-bold">{mismatch.errors[0]?.path.join('.') || 'root'}</code>: {mismatch.errors[0]?.message}
                    </p>
                    <div className="pt-2 flex gap-2">
                        <Button size="sm" variant="outline" className="h-8 text-[10px]" onClick={() => console.dir(mismatch)}>
                            View Raw Data
                        </Button>
                        <Button size="sm" variant="ghost" className="h-8 text-[10px]" onClick={() => setMismatch(null)}>
                            Dismiss
                        </Button>
                    </div>
                </div>
                <button onClick={() => setMismatch(null)} className="h-fit opacity-50 hover:opacity-100">
                    <X className="h-4 w-4" />
                </button>
            </div>
        </div>
    );
}
