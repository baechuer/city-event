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

import { Layout } from './components/Layout';

function AppRoutes() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/events" element={<EventsFeed />} />
        <Route path="/events/:id" element={<EventDetail />} />
        <Route path="/me/joins" element={<MyJoins />} />
        <Route path="/" element={<Navigate to="/events" replace />} />
      </Route>
      <Route path="/login" element={<Login />} />
      <Route path="/register" element={<Register />} />
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
