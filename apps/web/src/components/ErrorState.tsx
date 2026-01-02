import { Button } from "./ui/button";
import { AlertCircle } from "lucide-react"; // Make sure to install lucide-react if not present, oh wait I did earlier.

interface ErrorStateProps {
    title?: string;
    message?: string;
    onRetry?: () => void;
}

export function ErrorState({
    title = "Something went wrong",
    message = "Please try again later.",
    onRetry
}: ErrorStateProps) {
    return (
        <div className="flex flex-col items-center justify-center p-8 text-center space-y-4 h-[50vh]">
            <div className="p-4 rounded-full bg-destructive/10 text-destructive">
                <AlertCircle className="w-8 h-8" />
            </div>
            <div className="space-y-2">
                <h3 className="text-lg font-semibold tracking-tight">{title}</h3>
                <p className="text-sm text-muted-foreground max-w-[300px]">
                    {message}
                </p>
            </div>
            {onRetry && (
                <Button onClick={onRetry} variant="outline">
                    Try Again
                </Button>
            )}
        </div>
    );
}
