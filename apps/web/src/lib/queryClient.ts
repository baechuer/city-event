import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { ApiError } from './apiClient';
import { ApiErrorCode } from '../types/api';
import { authEvents, eventBus } from './events';

/**
 * Toast Deduplication Logic
 * Prevents flooding the UI with the same error message (e.g. 10 failed queries).
 */
const toastHistory = new Map<string, number>();
const DEDUP_WINDOW_MS = 2000; // 2 seconds

function showErrorToast(message: string) {
    const now = Date.now();
    const lastShown = toastHistory.get(message);

    if (lastShown && now - lastShown < DEDUP_WINDOW_MS) {
        return; // Skip duplicate toast
    }

    toastHistory.set(message, now);
    toast.error(message);
}

function handleGlobalError(error: unknown) {
    // 1. Handle ApiError known types
    if (error instanceof ApiError) {
        if (error.code === ApiErrorCode.UNAUTHENTICATED) {
            // Logic Layer (AuthProvider) handles this. NO TOAST.
            eventBus.emit(authEvents.UNAUTHORIZED);
            return;
        }

        // For other errors (validation, forbidden, timeouts, 500s), show toast.
        showErrorToast(error.message);
        return;
    }

    // 2. Handle generic errors
    if (error instanceof Error) {
        showErrorToast(error.message);
    } else {
        showErrorToast('An unexpected error occurred.');
    }
}

export const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            retry: (failureCount, error) => {
                // Don't retry client errors (4xx)
                if (error instanceof ApiError) {
                    if (error.code === ApiErrorCode.UNAUTHENTICATED ||
                        error.code === ApiErrorCode.FORBIDDEN ||
                        error.code === ApiErrorCode.VALIDATION_FAILED) {
                        return false;
                    }
                }
                return failureCount < 2;
            },
            refetchOnWindowFocus: false, // Cleaner dev experience
            staleTime: 1000 * 60 * 1, // 1 minute default stale
        },
    },
    // Global Error Handlers (for both Queries and Mutations)
    queryCache: new QueryCache({
        onError: (error) => handleGlobalError(error),
    }),
    mutationCache: new MutationCache({
        onError: (error) => handleGlobalError(error),
    }),
});
