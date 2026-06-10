# Product

## Register

CityEvents

## Users

CityEvents serves local event attendees, lightweight event organizers, and reviewers evaluating the project workflow. Attendees are browsing casually but need certainty once they join. Organizers need to publish small city events quickly and trust that capacity, waitlist, and cancellation state remain correct.

## Product Purpose

CityEvents is a compact event discovery and registration product that demonstrates a reliable distributed backend through a usable browser workflow. Success means a reviewer can register or log in, discover events, publish an event, join or cancel, inspect their status, and create a media upload intent without reading backend documentation.

## Brand Personality

Casual, precise, city-smart. The interface should feel like a modern transit board for spontaneous plans: social and approachable, but still operational enough to prove the backend workflow.

## Anti-references

Avoid generic admin dashboards, oversized marketing heroes, purple-blue SaaS gradients, card-heavy template layouts, and UI text that explains the architecture instead of helping the user act. The previous flat sidebar-and-card frontend is itself an anti-reference for this redesign.

## Design Principles

- Lead with the user journey: find an event, inspect the current status, act, then see the result.
- Make state impossible to miss: confirmed, waitlisted, canceled, loading, empty, and error states must have clear labels and affordances.
- Keep the app task-first: use density, alignment, and predictable controls rather than decorative chrome.
- Use technical polish sparingly: modern AI-tech styling should appear through crisp surfaces, OKLCH color, precise motion, and useful status treatment.
- Stay resume-honest: the UI demonstrates local workflows and should not imply production deployment, high availability, or exactly-once messaging.

## Reviewer Walkthrough

1. Open the local app and sign in or register.
2. Refresh the city feed or use a quick city filter.
3. Select an event row to open event detail.
4. Join the event and confirm the visible status change.
5. Cancel the join and confirm the cancellation state.
6. Publish a new event as an organizer.
7. Refresh the feed and verify the new event appears after projection catches up.
8. Create a media upload intent for the selected event.

## Accessibility & Inclusion

Target WCAG AA contrast, keyboard-visible focus states, 44px minimum touch targets, reduced-motion support, explicit form labels, and readable text at mobile and desktop sizes.
