import { useState } from "react";
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

export function Login() {
    const { login } = useAuth();
    const [error, setError] = useState<string | null>(null);

    const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm<LoginValues>({
        resolver: zodResolver(loginSchema),
    });

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

                    <Button type="submit" className="w-full" disabled={isSubmitting}>
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
