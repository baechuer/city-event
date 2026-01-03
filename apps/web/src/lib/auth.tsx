import { createContext, useContext, useEffect, useState, useCallback, useRef, type ReactNode } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { apiClient } from './apiClient';
import { tokenStore } from '../auth/tokenStore';
import { authEvents, eventBus } from './events';
import { queryClient } from './queryClient';
import type { User, RefreshResponse, BaseResponse, AuthData } from '../types/api';

interface AuthContextType {
    user: User | null;
    isAuthenticated: boolean;
    loading: boolean;
    login: (credentials: any) => Promise<void>;
    register: (data: any) => Promise<void>;
    logout: () => Promise<void>;
    setOAuthUser: (accessToken: string, user: User) => void; // For OAuth callback
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
                const res = await apiClient.post<BaseResponse<RefreshResponse>>('/auth/refresh');

                // Robust extraction: Check both wrapped and unwrapped
                const data = res.data?.data || (res.data as any);
                if (data?.tokens?.access_token) {
                    tokenStore.setToken(data.tokens.access_token);
                    setUser(data.user);
                }
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
        const res = await apiClient.post<BaseResponse<AuthData>>('/auth/login', credentials);

        const data = res.data?.data || (res.data as any);
        if (data?.tokens?.access_token) {
            tokenStore.setToken(data.tokens.access_token);
            setUser(data.user);
            navigate('/events');
        } else {
            throw new Error("Invalid login response format");
        }
    };

    // OAuth callback: set user state from OAuth popup result
    const setOAuthUser = useCallback((accessToken: string, oauthUser: User) => {
        tokenStore.setToken(accessToken);
        setUser(oauthUser);
        navigate('/events');
    }, [navigate]);

    const register = async (data: any) => {
        await apiClient.post('/auth/register', data);
        // Auto-login after register is common, or redirect to login.
        // For now, let's redirect to login to be explicit.
        navigate('/login');
    };

    // Subscribe to Global Events
    useEffect(() => {
        const unsubUnauthorized = eventBus.on(authEvents.UNAUTHORIZED, () => {
            if (tokenStore.getToken() || user) {
                logout();
            }
        });

        const unsubLogout = eventBus.on(authEvents.LOGOUT, () => {
            logout();
        });

        const unsubUserUpdate = eventBus.on(authEvents.USER_UPDATE, (newUser: User) => {
            setUser(newUser);
        });

        return () => {
            unsubUnauthorized();
            unsubLogout();
            unsubUserUpdate();
        };
    }, [logout, user]);

    return (
        <AuthContext.Provider value={{
            user,
            isAuthenticated: !!user,
            loading, // Expose loading state
            login,
            register,
            logout,
            setOAuthUser
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
