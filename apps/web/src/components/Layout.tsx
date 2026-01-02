import { Outlet } from "react-router-dom";
import { NavBar } from "./NavBar";

export function Layout() {
    return (
        <div className="flex min-h-screen flex-col bg-background font-sans antialiased">
            <NavBar />
            <main className="flex flex-1 flex-col">
                <Outlet />
            </main>
        </div>
    );
}
