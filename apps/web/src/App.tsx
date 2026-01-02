import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { QueryClientProvider } from '@tanstack/react-query';
import { Toaster } from 'sonner';
import { queryClient } from './lib/queryClient';
import { AuthProvider } from './lib/auth';

import { Login } from './pages/Login';
import { Register } from './pages/Register';
import { EventsFeed } from './pages/EventsFeed';
import { EventDetail } from './pages/EventDetail';
import { MyJoins } from './pages/MyJoins';
import { Profile } from './pages/Profile';

import { CreateEvent } from './pages/CreateEvent';

import { Layout } from './components/Layout';

import { ProtectedRoute } from './components/ProtectedRoute';

function AppRoutes() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/events" element={<EventsFeed />} />
        <Route path="/events/:id" element={<EventDetail />} />

        {/* Protected Routes */}
        <Route
          path="/me/joins"
          element={
            <ProtectedRoute>
              <MyJoins />
            </ProtectedRoute>
          }
        />
        <Route
          path="/events/new"
          element={
            <ProtectedRoute>
              <CreateEvent />
            </ProtectedRoute>
          }
        />
        <Route
          path="/profile"
          element={
            <ProtectedRoute>
              <Profile />
            </ProtectedRoute>
          }
        />

        <Route path="/login" element={<Login />} />
        <Route path="/register" element={<Register />} />
        <Route path="/" element={<Navigate to="/events" replace />} />
      </Route>
    </Routes>
  );
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AuthProvider>
          <AppRoutes />
          <Toaster richColors position="top-right" />
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

export default App;
