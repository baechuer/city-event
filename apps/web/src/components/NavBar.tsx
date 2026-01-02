import { Link, useLocation } from "react-router-dom";
import { useAuth } from "@/lib/auth";
import { Button } from "./ui/button";
import { cn } from "@/lib/utils";

export function NavBar() {
    const { user, logout, isAuthenticated, loading } = useAuth();
    const location = useLocation();
    const isAuthPage = ['/login', '/register'].includes(location.pathname);

    return (
        <header className="sticky top-4 z-50 w-[calc(100%-2rem)] mx-auto glass-card rounded-full transition-all duration-300">
            <div className="container mx-auto flex h-14 items-center px-6">
                <div className="mr-8 flex items-center gap-8">
                    <Link to="/" className="flex items-center space-x-2 group">
                        <div className="w-8 h-8 rounded-lg bg-emerald-600 flex items-center justify-center text-white font-bold transition-transform group-hover:rotate-12">C</div>
                        <span className="font-extrabold text-xl tracking-tighter text-slate-900 dark:text-white">
                            City<span className="text-emerald-600">Events</span>
                        </span>
                    </Link>

                    {!isAuthPage && (
                        <nav className="hidden md:flex items-center space-x-1">
                            <NavLink to="/events" active={location.pathname === '/events'}>
                                Discovery
                            </NavLink>
                            {isAuthenticated && (
                                <>
                                    <NavLink to="/me/joins" active={location.pathname === '/me/joins'}>
                                        My Activities
                                    </NavLink>
                                    <NavLink to="/events/new" active={location.pathname === '/events/new'}>
                                        Publish
                                    </NavLink>
                                </>
                            )}
                        </nav>
                    )}
                </div>

                <div className="flex flex-1 items-center justify-end space-x-4">
                    {loading ? (
                        <div className="h-8 w-8 rounded-full animate-pulse bg-muted"></div>
                    ) : isAuthenticated ? (
                        <div className="flex items-center gap-2">
                            <Link
                                to="/profile"
                                className="flex items-center gap-3 h-10 pl-4 pr-1 rounded-full hover:bg-emerald-600/10 transition-all cursor-pointer group border border-transparent hover:border-emerald-600/20 active:scale-95"
                            >
                                <div className="flex flex-col items-end hidden sm:flex">
                                    <span className="text-[10px] font-bold text-slate-900 dark:text-white leading-none group-hover:text-emerald-600 transition-colors uppercase tracking-tight">
                                        {user?.name || user?.email.split('@')[0]}
                                    </span>
                                    <span className="text-[8px] text-muted-foreground font-mono uppercase tracking-tighter mt-1 opacity-60">
                                        ID: {user?.id.slice(0, 6)}
                                    </span>
                                </div>

                                <div className="h-8 w-8 rounded-full bg-emerald-600 flex items-center justify-center text-white font-bold text-[10px] shadow-sm group-hover:shadow-emerald-500/20 transition-all">
                                    {(user?.name?.[0] || user?.email?.[0] || 'U').toUpperCase()}
                                </div>
                            </Link>

                            <div className="h-6 w-px bg-slate-200 dark:bg-white/10 mx-1 hidden sm:block" />

                            <Button
                                onClick={() => logout()}
                                variant="ghost"
                                size="sm"
                                className="h-10 rounded-full px-5 text-[10px] font-bold uppercase tracking-widest hover:bg-destructive/10 hover:text-destructive border border-transparent hover:border-destructive/20 active:scale-95 transition-all"
                            >
                                Logout
                            </Button>
                        </div>
                    ) : (
                        !isAuthPage && (
                            <div className="flex items-center gap-2">
                                <Link to="/login">
                                    <Button variant="ghost" size="sm" className="font-bold text-xs uppercase tracking-widest">Login</Button>
                                </Link>
                                <Link to="/register">
                                    <Button size="sm" className="rounded-full font-bold text-xs uppercase tracking-widest px-6 shadow-md shadow-emerald-500/20">Join Us</Button>
                                </Link>
                            </div>
                        )
                    )}
                </div>
            </div>
        </header>
    );
}

function NavLink({ to, children, active }: { to: string, children: React.ReactNode, active?: boolean }) {
    return (
        <Link
            to={to}
            className={cn(
                "px-4 py-2 rounded-full text-sm font-bold transition-all duration-200",
                active
                    ? "bg-emerald-600/10 text-emerald-600"
                    : "text-slate-500 hover:text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-950/30"
            )}
        >
            {children}
        </Link>
    );
}
