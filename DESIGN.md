# Design

## Visual Direction

CityEvents uses a light product interface with a "neon transit board for city plans" mood. The physical scene is a student or young professional checking tonight's plans on a laptop or phone in bright indoor light: fast scanning, low anxiety, clear status.

## Color

Use OKLCH tokens only. The surface stays near white so the app feels usable and fast. The brand anchor is sky-teal around hue 200, with coral used as a small energetic accent.

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

- Top bar: compact brand, primary actions, and local demo status.
- Workflow rail: numbered reviewer journey with current product actions.
- Feed panel: dense event rows with date, city, venue, and status.
- Detail panel: current event state, capacity, join/cancel actions, and status copy.
- Action column: account/session, publish event form, media upload intent.
- Notice strip: concise success/error feedback with `aria-live`.

## Layout

Desktop uses a three-column product layout: feed, detail, action column. Tablet collapses to two columns. Mobile becomes a single-column task flow with sticky top navigation and full-width controls.

## Motion

Use 150-220ms state transitions for hover, focus, panel emphasis, and selected rows. Respect `prefers-reduced-motion: reduce`; content must be visible without animation.

## Anti-patterns To Avoid

No nested cards, no colored side-stripe borders, no gray text on colored backgrounds, no architecture explanation blocks inside the app, no generic purple-blue gradients, and no hidden labels in forms.
