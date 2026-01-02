# Phase 3: Global Token Refresh Implementation Plan (Revised)

## Goal
Implement a "Silent Refresh" mechanism that is robust against loops, concurrency storms, and unsafe replays.

## Technical Design

### 1. `refreshClient` Isolation
To prevent interceptor pollution, we use a separate axios instance for the refresh call.
```typescript
const refreshClient = axios.create({ baseURL: '/api', withCredentials: true });
```

### 2. Module-Level Promise Queue
Instead of managing an array of callbacks, we share a single promise.
```typescript
let refreshPromise: Promise<void> | null = null;
```

### 3. Idempotency Gate
We must not blindly retry non-idempotent requests (POST/PUT/DELETE) as it could lead to double-payment or duplicate data creation if the server processed the first 401 (improbable for 401, but safer to be strict).
```typescript
const isSafeRequest = (method) => ['get', 'head', 'options'].includes(method);
const isIdempotent = (config) => config.headers['Idempotency-Key'] || config.headers['X-Idempotency-Key'];

if (!isSafeRequestAndNotIdempotent) return reject(error);
```

### 4. Logic Flow

#### Request Interceptor
```typescript
apiClient.interceptors.request.use(async (config) => {
    if (refreshPromise) {
         await refreshPromise.catch(() => {}); // Wait for ongoing refresh
    }
    // ... inject token ...
    return config;
});
```

#### Response Interceptor
```typescript
apiClient.interceptors.response.use(
    (res) => res,
    async (error) => {
        const status = error.response?.status;
        const original = error.config;

        if (status === 401 && original && !original._retry) {
             // 0. Prevent Loop
             if (original.url?.includes('/auth/refresh')) return Promise.reject(error);

             // 1. Check conditions (User logged in?)
             if (!tokenStore.getToken()) return Promise.reject(error);

             // 2. Idempotency Gate
             if (!isSafe(original.method) && !hasIdempotencyKey(original)) {
                 return Promise.reject(error);
             }

             original._retry = true;

             // 3. Start or Reuse Refresh
             if (!refreshPromise) {
                 refreshPromise = refreshClient.post('/auth/refresh')
                     .then(res => {
                         tokenStore.setToken(res.data.access_token);
                         eventBus.emit('auth:user-update', res.data.user);
                     })
                     .catch(err => {
                         eventBus.emit('auth:logout');
                         throw err;
                     })
                     .finally(() => {
                         refreshPromise = null;
                     });
             }

             // 4. Wait & Retry
             return refreshPromise.then(() => apiClient(original));
        }
    }
);
```

## Verification Steps
1.  **Dev Environment**: Expose `tokenStore` and `apiClient` to window.
2.  **Manual Trigger**: `tokenStore.setToken('expired')`.
3.  **Observation**:
    -   GET requests should pause and retry.
    -   POST requests should fail immediately (unless keyed).
    -   Refresh call happens exactly once.
