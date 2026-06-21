# Product

## Name

CityEvents

## Users

CityEvents serves local event attendees, lightweight event organizers, and reviewers evaluating the project workflow. Attendees are browsing casually but need certainty once they join. Organizers need to publish small city events quickly and trust that capacity, waitlist, and cancellation state remain correct.

## Product Purpose

CityEvents is a clean customer-facing event discovery product backed by the local distributed services. Success means a visitor can immediately see interesting events, browse category lanes, open a detail page, sign in only when ready to act, publish an event, join or cancel, and create a media upload intent without reading backend documentation.

## Brand Personality

Casual, precise, city-smart. The interface should feel like a polished local discovery app: strong first impression, useful choices, clear category intent, and customer language instead of system language.

## Anti-references

Avoid generic admin dashboards, oversized marketing heroes, Meetup cloning, purple-blue SaaS gradients, card-heavy template layouts, and UI text that explains the architecture instead of helping the user act.

## Design Principles

- Lead with discovery: the first viewport must show search, one strong event, and clear category entry points; deeper detail can scroll or move to a focused route.
- Make state impossible to miss: confirmed, waitlisted, canceled, loading, empty, and error states must have clear labels and affordances.
- Keep the app customer-first: browsing is open; sign-in appears only when joining, publishing, or managing media.
- Use technical polish sparingly: modern AI-tech styling should appear through crisp surfaces, OKLCH color, precise motion, and useful status treatment.
- Keep the product scope clear: the UI should demonstrate the core event workflows while the public documentation explains which architecture qualities are implemented locally and which are planned production evolutions.

## Page Model

- `/` is a clean discovery page with hero search, major featured event, popular events, category lanes, and upcoming city events.
- `/events` is the full browse/search page.
- `/events/:id` is the event detail and RSVP page.
- `/categories` lists major discovery lanes.
- `/categories/:slug` focuses one lane: Networking, Meet New Friends, Sports, Hobbies, Learning & Tech, or Food & Nightlife.
- `/publish` is the organizer create-event flow.
- `/me` contains sign-in, profile state, selected event summary, and media upload intent.

## Reviewer Walkthrough

1. Open the local app and inspect the discovery page.
2. Browse a category lane or use the search form.
3. Open an event detail page.
4. Sign in only when attempting to join or publish.
5. Publish a live event as an organizer.
6. Open the live event detail page and join it.
7. Cancel the RSVP and confirm the visible status change.
8. Refresh the feed and verify the new event appears after projection catches up.
9. Use `/me` to create a media upload intent for the selected live event.

## Accessibility & Inclusion

Target WCAG AA contrast, keyboard-visible focus states, 44px minimum touch targets, reduced-motion support, explicit form labels, and readable text at mobile and desktop sizes.
