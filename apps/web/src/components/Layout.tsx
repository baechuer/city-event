import { Outlet } from "react-router-dom";
import { NavBar } from "./NavBar";

export function Layout() {
    return (
        <div className="min-h-screen bg-background font-sans antialiased">
            <NavBar />
            <Outlet />
        </div>
    );
}
