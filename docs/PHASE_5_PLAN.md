# Phase 5 — Resilient Frontend & Pro‑Max UI

> **Principles**
> - BFF is the single source of truth
> - URL is the single source of discovery state
> - Frontend never guesses backend state

---

## 1. Type Safety & Contract Alignment (Zod‑First)

### 1.1 Contract Source
All BFF responses MUST be defined in:

```
src/api/bff/schemas.ts
```

```ts
export const EventCardSchema = z.object({...});
export const EventViewSchema = z.object({...});
export const JoinStateSchema = z.object({...});
export const FeedPageSchema = z.object({...});
```

### 1.2 Runtime Validation
All API calls must use:

```ts
schema.parse(response.data)
```

On failure:
- Show **“API contract mismatch”** banner
- Log raw response to console in dev
- Do **not** crash UI

### 1.3 Types
All UI types are derived from Zod:

```ts
export type EventView = z.infer<typeof EventViewSchema>;
```

---

## 2. Pro‑Max UI Foundation

### 2.1 Fonts
Use controlled font loading (no CSS `@import`):

```bash
npm install @fontsource/poppins @fontsource/open-sans
```

```ts
import "@fontsource/poppins/600.css";
import "@fontsource/open-sans/400.css";
```

### 2.2 Glassmorphism

```css
.glass-card {
  background: rgba(255,255,255,0.75);
  backdrop-filter: blur(14px);
  -webkit-backdrop-filter: blur(14px);
}

@supports not (backdrop-filter: blur(14px)) {
  .glass-card {
    background: rgb(245,245,245);
  }
}
```

---

## 3. Discovery & Feed (URL‑First + Race‑Safe)

- Filters derived from `useSearchParams`
- TanStack Query key includes all filters
- Abort in‑flight requests on filter change
- Infinite scroll with IntersectionObserver and skeleton loaders

---

## 4. Event View & Action Policy

- Fetch from `/api/events/{id}/view`
- Buttons gated by `ActionPolicy`
- Mutation locks scoped by `(eventId, action)`
- Degraded mode shows banner and disables writes
- Auto‑refetch every 15s when degraded

---

## 5. My Activities

- Infinite scroll of `/api/me/joins`
- Inline cancel actions
- Status pills driven by JoinState

---

## Verification

- Zod catches contract drift
- Filter changes never mix results
- Degraded banner clears after recovery
- Idempotent joins work under spam

