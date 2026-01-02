import { createContext, useContext, useEffect, useState, useCallback, useRef, type ReactNode } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { apiClient } from './apiClient';
import { tokenStore } from '../auth/tokenStore';
import { authEvents, eventBus } from './events';
import { queryClient } from './queryClient';
import type { User, RefreshResponse } from '../types/api';

interface AuthContextType {
    user: User | null;
    isAuthenticated: boolean;
    loading: boolean;
    login: (credentials: any) => Promise<void>;
    register: (data: any) => Promise<void>;
    logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
    const [user, setUser] = useState<User | null>(null);
    const [loading, setLoading] = useState(true);
    const navigate = useNavigate();
    const location = useLocation();
    const bootstrapRef = useRef(false);

    // Bootstrap Session (Single Flight)
    useEffect(() => {
        if (bootstrapRef.current) return;
        bootstrapRef.current = true;

        const initAuth = async () => {
            try {
                // Attempt to refresh session (HttpOnly cookie)
                // Now returns { tokens, user }
                const res = await apiClient.post<RefreshResponse>('/auth/refresh');

                tokenStore.setToken(res.data.access_token);
                setUser(res.data.user);
            } catch (err) {
                // If refresh fails (401/403), we are logged out
                tokenStore.setToken(null);
                setUser(null);
            } finally {
                setLoading(false);
            }
        };

        initAuth();
    }, []);

    // Helper to safely navigate to login
    const navigateToLogin = useCallback(() => {
        // Prevent redirect loop if already on login
        if (location.pathname !== '/login' && location.pathname !== '/register') {
            navigate('/login');
        }
    }, [navigate, location.pathname]);

    const logout = useCallback(async () => {
        // Idempotency check: if already logging out, do nothing
        if (tokenStore.isLoggingOut()) return;

        try {
            tokenStore.setLoggingOut(true);

            // 1. Call API to clear HttpOnly cookie
            await apiClient.post('/auth/logout').catch(() => { });

            // 2. Clean up Client State (Critical!)
            tokenStore.setToken(null);
            setUser(null);

            // 3. Clear Sensitive Data from Cache
            queryClient.removeQueries();

            // 4. Redirect
            navigateToLogin();

        } finally {
            tokenStore.setLoggingOut(false);
        }
    }, [navigateToLogin]);

    const login = async (credentials: any) => {
        const res = await apiClient.post('/auth/login', credentials);
        // B1: Store token in memory only
        tokenStore.setToken(res.data.access_token);
        setUser(res.data.user);
        navigate('/events');
    };

    const register = async (data: any) => {
        await apiClient.post('/auth/register', data);
        // Auto-login after register is common, or redirect to login.
        // For now, let's redirect to login to be explicit.
        navigate('/login');
    };

    // Subscribe to Global 401 Events
    useEffect(() => {
        const unsubscribe = eventBus.on(authEvents.UNAUTHORIZED, () => {
            // Only logout if we think we are logged in (have a user or token)
            // This prevents loops where a public endpoint returns 401
            if (tokenStore.getToken() || user) {
                logout();
            }
        });
        return unsubscribe;
    }, [logout, user]);

    return (
        <AuthContext.Provider value={{
            user,
            isAuthenticated: !!user,
            loading, // Expose loading state
            login,
            register,
            logout
        }}>
            {children}
        </AuthContext.Provider>
    );
}

export function useAuth() {
    const context = useContext(AuthContext);
    if (!context) {
        throw new Error('useAuth must be used within an AuthProvider');
    }
    return context;
}
