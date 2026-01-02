import { Outlet } from "react-router-dom";
import { NavBar } from "./NavBar";
import { BffBanner } from "./BffBanner";

export function Layout() {
    return (
        <div className="flex min-h-screen flex-col bg-background antialiased selection:bg-emerald-100 selection:text-emerald-900">
            <NavBar />
            <main className="flex flex-1 flex-col">
                <Outlet />
            </main>
            <BffBanner />
        </div>
    );
}
