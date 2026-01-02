// Simple Event Bus for global app events (decoupling Auth from API)

type EventCallback = (data?: any) => void;
const listeners: Record<string, Set<EventCallback>> = {};

export const authEvents = {
    UNAUTHORIZED: 'auth:unauthorized',
    LOGOUT: 'auth:logout',
    USER_UPDATE: 'auth:user-update',
};

export const eventBus = {
    on: (event: string, callback: EventCallback) => {
        if (!listeners[event]) {
            listeners[event] = new Set();
        }
        listeners[event].add(callback);
        return () => {
            listeners[event]?.delete(callback);
        };
    },

    emit: (event: string, data?: any) => {
        listeners[event]?.forEach((cb) => cb(data));
    },
};
