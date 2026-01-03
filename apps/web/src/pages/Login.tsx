import { useState, useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import * as z from "zod";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Link } from "react-router-dom";
import { ApiError } from "@/lib/apiClient";

const loginSchema = z.object({
    email: z.string().email("Invalid email address"),
    password: z.string().min(1, "Password is required"),
});

type LoginValues = z.infer<typeof loginSchema>;

// Google OAuth message type
interface OAuthMessage {
    type: 'oauth_success' | 'oauth_error';
    access_token?: string;
    user?: {
        id: string;
        email: string;
        role: string;
        email_verified: boolean;
    };
    redirect_to?: string;
    error?: string;
}

export function Login() {
    const { login, setOAuthUser } = useAuth();
    const [error, setError] = useState<string | null>(null);
    const [isGoogleLoading, setIsGoogleLoading] = useState(false);

    const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm<LoginValues>({
        resolver: zodResolver(loginSchema),
    });

    // Listen for OAuth callback messages from popup
    useEffect(() => {
        const handleMessage = (event: MessageEvent<OAuthMessage>) => {
            // Verify origin matches our app
            if (event.origin !== window.location.origin) return;

            const data = event.data;
            if (!data || typeof data !== 'object') return;

            if (data.type === 'oauth_success' && data.access_token && data.user) {
                setIsGoogleLoading(false);
                setOAuthUser(data.access_token, data.user);
            } else if (data.type === 'oauth_error') {
                setIsGoogleLoading(false);
                setError(data.error || 'OAuth login failed. Please try again.');
            }
        };

        window.addEventListener('message', handleMessage);
        return () => window.removeEventListener('message', handleMessage);
    }, [setOAuthUser]);

    // Check for OAuth result in sessionStorage (fallback for non-popup flows)
    useEffect(() => {
        const oauthResult = sessionStorage.getItem('oauth_result');
        if (oauthResult) {
            try {
                const data = JSON.parse(oauthResult) as OAuthMessage;
                if (data.type === 'oauth_success' && data.access_token && data.user) {
                    sessionStorage.removeItem('oauth_result');
                    setOAuthUser(data.access_token, data.user);
                }
            } catch {
                sessionStorage.removeItem('oauth_result');
            }
        }
    }, [setOAuthUser]);

    const handleGoogleLogin = () => {
        setError(null);
        setIsGoogleLoading(true);

        // Open Google OAuth in a popup
        const width = 500;
        const height = 600;
        const left = window.screenX + (window.outerWidth - width) / 2;
        const top = window.screenY + (window.outerHeight - height) / 2;

        const popup = window.open(
            '/api/auth/oauth/google/start?redirect_to=/events',
            'oauth',
            `width=${width},height=${height},left=${left},top=${top},popup=1`
        );

        // Monitor popup close (user cancelled)
        if (popup) {
            const checkClosed = setInterval(() => {
                if (popup.closed) {
                    clearInterval(checkClosed);
                    setIsGoogleLoading(false);
                }
            }, 500);
        } else {
            setIsGoogleLoading(false);
            setError('Unable to open login popup. Please check your popup blocker settings.');
        }
    };

    const onSubmit = async (data: LoginValues) => {
        setError(null);
        try {
            await login(data);
        } catch (err: any) {
            // Check for invalid credentials error code directly
            if (err?.code === 'invalid_credentials' || err?.code === 'unauthenticated') {
                setError("Invalid email or password. Please check your credentials and try again.");
            } else if (err instanceof ApiError) {
                // Show the specific error message from the API
                setError(err.message || "Login failed. Please try again.");
            } else if (err?.message) {
                // Fallback to error message if available
                setError(err.message);
            } else {
                setError("Unable to connect to the server. Please try again later.");
            }
        }
    };

    return (
        <div className="flex flex-1 items-center justify-center bg-muted/20 px-4 pb-14">
            <div className="w-full max-w-md space-y-8 rounded-xl border bg-card p-8 shadow-sm">
                <div className="text-center">
                    <h1 className="text-3xl font-bold tracking-tight text-emerald-600">CityEvents</h1>
                    <h2 className="mt-2 text-xl font-semibold tracking-tight text-foreground">Welcome back</h2>
                    <p className="mt-2 text-sm text-muted-foreground">
                        Sign in to your account
                    </p>
                </div>

                {error && (
                    <div className="rounded-md bg-destructive/15 p-3 text-sm text-destructive">
                        {error}
                    </div>
                )}

                {/* Google OAuth Button */}
                <Button
                    type="button"
                    variant="outline"
                    className="w-full flex items-center justify-center gap-3 py-5"
                    onClick={handleGoogleLogin}
                    disabled={isGoogleLoading || isSubmitting}
                >
                    <svg className="h-5 w-5" viewBox="0 0 24 24">
                        <path
                            fill="currentColor"
                            d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
                        />
                        <path
                            fill="currentColor"
                            d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
                        />
                        <path
                            fill="currentColor"
                            d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"
                        />
                        <path
                            fill="currentColor"
                            d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
                        />
                    </svg>
                    {isGoogleLoading ? "Signing in..." : "Continue with Google"}
                </Button>

                <div className="relative">
                    <div className="absolute inset-0 flex items-center">
                        <span className="w-full border-t" />
                    </div>
                    <div className="relative flex justify-center text-xs uppercase">
                        <span className="bg-card px-2 text-muted-foreground">Or continue with email</span>
                    </div>
                </div>

                <form onSubmit={handleSubmit(onSubmit)} className="space-y-6">
                    <div className="space-y-2">
                        <Label htmlFor="email">Email</Label>
                        <Input id="email" type="email" placeholder="you@example.com" {...register("email")} />
                        {errors.email && <p className="text-xs text-destructive">{errors.email.message}</p>}
                    </div>

                    <div className="space-y-2">
                        <div className="flex justify-between">
                            <Label htmlFor="password">Password</Label>
                            <span className="text-xs text-muted-foreground">Forgot password?</span>
                        </div>
                        <Input id="password" type="password" {...register("password")} />
                        {errors.password && <p className="text-xs text-destructive">{errors.password.message}</p>}
                    </div>

                    <Button type="submit" className="w-full" disabled={isSubmitting || isGoogleLoading}>
                        {isSubmitting ? "Signing in..." : "Sign in"}
                    </Button>
                </form>

                <div className="text-center text-sm">
                    Don't have an account?{" "}
                    <Link to="/register" className="font-semibold text-emerald-600 hover:text-emerald-500">
                        Sign up
                    </Link>
                </div>
            </div>
        </div>
    );
}
