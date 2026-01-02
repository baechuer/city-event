import { Link, useLocation } from "react-router-dom";
import { useAuth } from "@/lib/auth";
import { Button } from "./ui/button";

export function NavBar() {
    const { user, logout, isAuthenticated, loading } = useAuth();
    const location = useLocation();
    const isAuthPage = ['/login', '/register'].includes(location.pathname);

    return (
        <header className="sticky top-0 z-50 w-full border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
            <div className="container mx-auto flex h-14 items-center px-4">
                <div className="mr-4 flex">
                    <Link to="/" className="mr-6 flex items-center space-x-2 font-bold text-lg">
                        <span className="text-emerald-600">CityEvents</span>
                    </Link>
                    {!isAuthPage && (
                        <nav className="flex items-center space-x-6 text-sm font-medium">
                            <Link to="/events" className="transition-colors hover:text-foreground/80 text-foreground/60">
                                Feed
                            </Link>
                        </nav>
                    )}
                </div>

                <div className="flex flex-1 items-center justify-end space-x-4">
                    {loading ? (
                        // Loading State (Skeleton or Empty to prevent flash)
                        <div className="h-9 w-20 animate-pulse rounded bg-muted"></div>
                    ) : isAuthenticated ? (
                        <div className="flex items-center gap-4">
                            <span className="text-sm text-muted-foreground hidden sm:inline-block">
                                Welcome, {user?.name}
                            </span>
                            <Button onClick={() => logout()} variant="outline" size="sm">
                                Logout
                            </Button>
                        </div>
                    ) : (
                        !isAuthPage && (
                            <div className="flex items-center gap-2">
                                <Link to="/login">
                                    <Button variant="ghost" size="sm">Login</Button>
                                </Link>
                                <Link to="/register">
                                    <Button size="sm">Get Started</Button>
                                </Link>
                            </div>
                        )
                    )}
                </div>
            </div>
        </header>
    );
}
