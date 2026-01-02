// In-memory Token Store (Closure) - B1 Architecture
// NOT stored in localStorage/sessionStorage/Redux.

let accessToken: string | null = null;
let isLoggingOut = false; // Flag to prevent concurrent logout loops

export const tokenStore = {
    getToken: () => accessToken,
    setToken: (token: string | null) => {
        accessToken = token;
    },

    // Flag to indicate we are in the middle of a logout process
    // Used to prevent multiple 401s triggering multiple redirects
    isLoggingOut: () => isLoggingOut,
    setLoggingOut: (value: boolean) => {
        isLoggingOut = value;
    },
};
