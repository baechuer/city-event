# Design

## Visual Direction

CityEvents uses a compact "city radar" interface. The first viewport should feel active and customer-facing: search, featured event, popular stack, categories, and upcoming events are all visible together on desktop. The design is not a Meetup clone; it is denser, more editorial, and built for fast plan selection.

## Color

Use OKLCH tokens only. The surface stays warm and light so the app feels usable and fast. The palette uses multiple accents by category: teal for social discovery, coral for networking, green for sports, amber for hobbies, blue for tech, and rose for food/nightlife.

```css
--bg: oklch(1 0 0);
--canvas: oklch(0.972 0.008 205);
--surface: oklch(0.992 0.004 205);
--surface-strong: oklch(0.955 0.018 205);
--ink: oklch(0.18 0.025 214);
--muted: oklch(0.46 0.026 216);
--primary: oklch(0.56 0.105 200);
--primary-dark: oklch(0.36 0.095 202);
--accent: oklch(0.66 0.145 28);
--line: oklch(0.88 0.015 210);
--success: oklch(0.52 0.12 150);
--warning: oklch(0.70 0.135 78);
--danger: oklch(0.58 0.16 25);
```

## Typography

Use a native system sans stack for speed and product familiarity. Keep fixed rem sizes, not viewport-scaled type. Reserve heavier weights for panel headings, selected event titles, and primary status labels.

## Components

- Top bar: compact brand and page navigation.
- Home board: search, hero copy, major spotlight event, popular event stack, category strip, and city row.
- Event card: local image, category/status label, title, description, time, city, and spots left.
- Category tile: image-led lane for Networking, Meet New Friends, Sports, Hobbies, Learning & Tech, and Food & Nightlife.
- Event detail: large visual, facts, capacity meter, and RSVP actions.
- Account pages: sign-in, profile, selected event, and media upload intent.
- Notice toast: concise success/error feedback with `aria-live` without consuming first-viewport layout space.

## Layout

Desktop home uses a compressed board designed to fit the primary discovery experience into the first viewport. Interior pages use focused two-column layouts where useful, then collapse to single-column mobile flows with full-width controls.

## Motion

Use 150-220ms state transitions for hover, focus, panel emphasis, and selected rows. Respect `prefers-reduced-motion: reduce`; content must be visible without animation.

## Anti-patterns To Avoid

No nested cards, no colored side-stripe borders, no gray text on colored backgrounds, no architecture explanation blocks inside the app, no generic purple-blue gradients, and no hidden labels in forms.
