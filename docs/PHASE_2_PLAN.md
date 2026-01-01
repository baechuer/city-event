# Phase 2: Frontend Foundation Implementation Plan

## Goal
Build a modern, responsive Single Page Application (SPA) using React that allows users to Register, Login, and generate real-time events. The SPA will communicate exclusively with the `bff-service`.

## Technology Stack
- **Framework**: React 18+ (Vite)
- **Language**: TypeScript
- **Styling**: TailwindCSS (Utility-first, responsive) + Clean/Premium Aesthetics
- **Routing**: React Router v6
- **State Management**: 
  - **Server State**: TanStack Query (React Query)
  - **Auth State**: React Context + Axios Interceptors
- **Icons**: Lucide React / Heroicons

## User Workflows (The "Happy Paths")

### 1. Authentication Flow
- **Registration**:
  - User lands on `/register`.
  - Enters Email/Password -> Clicks "Sign Up".
  - System sends verification email (simulated via Mailpit).
  - User sees "Check your email" prompt.
- **Verification**:
  - User clicks link in pseudo-email -> `/verify-email?token=xyz`.
  - App calls API to verify -> Auto-redirects to Login.
- **Login**:
  - User lands on `/login`.
  - Enters credentials -> Clicks "Sign In".
  - **Success**: JWT Cookie set (HttpOnly). App redirects to `/dashboard`.
  - **Failure**: Show global toast notification (e.g., "Invalid credentials").

### 2. Dashboard & Event Simulation
- **Dashboard (`/dashboard`)**:
  - **User Profile**: Displays "Welcome, [Email]" (Fetched via `/api/auth/me`).
  - **Status Card**: Shows "Verified: Yes/No", "Role: User/Admin".
- **Event Generator**:
  - A conceptual "Shop" or "Feed" UI.
  - User clicks "View Item" or "Add to Cart".
  - App sends background POST to `/api/events` (fire-and-forget).
  - **Visual Feedback**: instant "toast" or distinct UI animation showing "Event Sent".

## Implementation Steps

### Step 1: Project Configuration & Styling
- [ ] **Install Dependencies**: `react-router-dom`, `axios`, `@tanstack/react-query`, `lucide-react`, `clsx`, `tailwind-merge`.
- [ ] **Configure Tailwind**: Set up a premium color palette (Slate/Zinc, Indigo/Violet accents) and font (Inter/Outfit).
- [ ] **Setup Axios Client**:
  - Base URL: `/` (Proxied by Vite to BFF).
  - Interceptors for global error handling (401 -> Redirect to Login).

### Step 2: Global Components & Layout
- [ ] **UI Kit**: Button (variants), Input, Card, Toast/Notification System.
- [ ] **Layouts**:
  - `AuthLayout`: Centered card for Login/Register.
  - `DashboardLayout`: Navbar (with Logout), Sidebar (optional), Main Content area.

### Step 3: Authentication Feature
- [ ] **AuthProvider**: Context to hold `user` object and `isAuthenticated` state.
- [ ] **Login Page**: Form validation, API integration.
- [ ] **Register Page**: Form, Success state.
- [ ] **Route Protection**: `RequireAuth` wrapper component to protect dashboard routes.

### Step 4: Event Simulation Feature
- [ ] **EventService**: API wrapper for `/api/events`.
- [ ] **Simulation Component**: Buttons to trigger 'item_viewed', 'item_purchased' events.
- [ ] **Live Feed (Optional/Future)**: If we have WS later, show events appearing. For now, just show "Event Sent" logs in a UI console.

### 5. Integration Verification
- [ ] **End-to-End Manual Test**:
  - Register -> Verify -> Login -> Click Buttons -> Check Backend Logs (BFF/Event Service).
