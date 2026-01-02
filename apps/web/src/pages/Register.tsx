import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import * as z from "zod";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Link } from "react-router-dom";

const registerSchema = z.object({
    name: z.string().min(2, "Name must be at least 2 characters"),
    email: z.string().email("Invalid email address"),
    password: z.string().min(6, "Password must be at least 6 characters"),
});

type RegisterValues = z.infer<typeof registerSchema>;

export function Register() {
    const { register: registerAuth } = useAuth();
    const [error, setError] = useState<string | null>(null);

    const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm<RegisterValues>({
        resolver: zodResolver(registerSchema),
    });

    const onSubmit = async (data: RegisterValues) => {
        setError(null);
        try {
            await registerAuth(data);
        } catch (err: any) {
            setError(err.message || "Registration failed. Please try again.");
        }
    };

    return (
        <div className="flex flex-1 items-center justify-center bg-muted/20 px-4 pb-14">
            <div className="w-full max-w-md space-y-8 rounded-xl border bg-card p-8 shadow-sm">
                <div className="text-center">
                    <h1 className="text-3xl font-bold tracking-tight text-emerald-600">CityEvents</h1>
                    <h2 className="mt-2 text-xl font-semibold tracking-tight text-foreground">Create an account</h2>
                    <p className="mt-2 text-sm text-muted-foreground">
                        Get started with your free account
                    </p>
                </div>

                {error && (
                    <div className="rounded-md bg-destructive/15 p-3 text-sm text-destructive">
                        {error}
                    </div>
                )}

                <form onSubmit={handleSubmit(onSubmit)} className="space-y-6">
                    <div className="space-y-2">
                        <Label htmlFor="name">Full Name</Label>
                        <Input id="name" placeholder="John Doe" {...register("name")} />
                        {errors.name && <p className="text-xs text-destructive">{errors.name.message}</p>}
                    </div>

                    <div className="space-y-2">
                        <Label htmlFor="email">Email</Label>
                        <Input id="email" type="email" placeholder="you@example.com" {...register("email")} />
                        {errors.email && <p className="text-xs text-destructive">{errors.email.message}</p>}
                    </div>

                    <div className="space-y-2">
                        <Label htmlFor="password">Password</Label>
                        <Input id="password" type="password" {...register("password")} />
                        {errors.password && <p className="text-xs text-destructive">{errors.password.message}</p>}
                    </div>

                    <Button type="submit" className="w-full" disabled={isSubmitting}>
                        {isSubmitting ? "Create account" : "Create account"}
                    </Button>
                </form>

                <div className="text-center text-sm">
                    Already have an account?{" "}
                    <Link to="/login" className="font-semibold text-emerald-600 hover:text-emerald-500">
                        Sign in
                    </Link>
                </div>
            </div>
        </div>
    );
}
