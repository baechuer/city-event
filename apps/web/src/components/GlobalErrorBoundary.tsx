import { Component, type ErrorInfo, type ReactNode } from "react";
import { Button } from "./ui/button";

interface Props {
    children: ReactNode;
}

interface State {
    hasError: boolean;
    error: Error | null;
}

export class GlobalErrorBoundary extends Component<Props, State> {
    public state: State = {
        hasError: false,
        error: null,
    };

    public static getDerivedStateFromError(error: Error): State {
        return { hasError: true, error };
    }

    public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
        console.error("Uncaught error:", error, errorInfo);
    }

    public render() {
        if (this.state.hasError) {
            return (
                <div className="flex h-screen w-full flex-col items-center justify-center p-4 bg-red-50 text-red-900">
                    <h1 className="text-2xl font-bold mb-4">Something went wrong.</h1>
                    <p className="mb-4 text-sm font-mono bg-white p-4 rounded border border-red-200 whitespace-pre-wrap max-w-2xl overflow-auto">
                        {this.state.error?.message}
                        {this.state.error?.stack}
                    </p>
                    <Button onClick={() => window.location.reload()} variant="destructive">
                        Reload Application
                    </Button>
                </div>
            );
        }

        return this.props.children;
    }
}
