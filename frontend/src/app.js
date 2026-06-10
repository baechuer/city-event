import { createApiClient } from './api.js';
import { canCancel, canJoin, createInitialState, formatDateTime, statusLabel, statusTone } from './state.js';

const api = createApiClient(window.CITYEVENTS_CONFIG || {});
const state = createInitialState();
state.auth = api.loadAuth();
state.feed.city = state.feed.city || 'Sydney';
state.filters = { keyword: '', city: 'Sydney', category: '', date: 'any' };
state.returnTo = '';

const app = document.querySelector('#app');
const quickCities = ['Sydney', 'Melbourne', 'Brisbane'];

const categories = [
  {
    slug: 'networking',
    name: 'Networking',
    short: 'Career rooms',
    line: 'Founder nights, industry mixers, job leads, and serious conversations.',
    image: '/assets/categories/networking.png',
    accent: 'coral',
  },
  {
    slug: 'new-friends',
    name: 'Meet New Friends',
    short: 'Low-pressure socials',
    line: 'Brunch, board games, walking groups, and new-in-town tables.',
    image: '/assets/categories/friends.png',
    accent: 'teal',
  },
  {
    slug: 'sports-outdoors',
    name: 'Sports',
    short: 'Move together',
    line: 'Pickleball, running, hikes, social leagues, and weekend outdoors.',
    image: '/assets/categories/sports.png',
    accent: 'green',
  },
  {
    slug: 'hobbies',
    name: 'Hobbies',
    short: 'Do the thing',
    line: 'Books, photography, games, crafts, anime, cooking, and music circles.',
    image: '/assets/categories/hobbies.png',
    accent: 'amber',
  },
  {
    slug: 'learning-tech',
    name: 'Learning & Tech',
    short: 'Build skills',
    line: 'AI workshops, coding nights, study rooms, product and design meetups.',
    image: '/assets/categories/tech.png',
    accent: 'blue',
  },
  {
    slug: 'food-nightlife',
    name: 'Food & Nightlife',
    short: 'After-hours plans',
    line: 'Supper clubs, markets, tastings, music bars, and late city plans.',
    image: '/assets/categories/food.png',
    accent: 'rose',
  },
];

const categoryBySlug = new Map(categories.map((category) => [category.slug, category]));

const demoEvents = [
  {
    id: 'demo-sydney-founder-signal',
    source: 'demo',
    category: 'networking',
    rank: 99,
    title: 'Founder Signal Night',
    description: 'A compact room for students, builders, and early founders to trade ideas and meet useful people.',
    city: 'Sydney',
    venue: 'Haymarket Studio',
    startsAt: relativeDate(2, 18, 30),
    capacity: 80,
    confirmedCount: 62,
    status: 'PUBLISHED',
    label: 'Major this week',
  },
  {
    id: 'demo-sydney-new-table',
    source: 'demo',
    category: 'new-friends',
    rank: 95,
    title: 'New in Town Dinner Table',
    description: 'A hosted dinner for people who want easy conversation without awkward icebreakers.',
    city: 'Sydney',
    venue: 'Surry Hills Kitchen',
    startsAt: relativeDate(1, 19, 0),
    capacity: 24,
    confirmedCount: 18,
    status: 'PUBLISHED',
    label: 'Filling fast',
  },
  {
    id: 'demo-sydney-pickleball',
    source: 'demo',
    category: 'sports-outdoors',
    rank: 92,
    title: 'Social Pickleball Rally',
    description: 'Beginner-friendly doubles rotation with spare paddles and post-game coffee.',
    city: 'Sydney',
    venue: 'Moore Park Courts',
    startsAt: relativeDate(3, 8, 30),
    capacity: 32,
    confirmedCount: 21,
    status: 'PUBLISHED',
    label: 'Morning plan',
  },
  {
    id: 'demo-melbourne-ai-build',
    source: 'demo',
    category: 'learning-tech',
    rank: 89,
    title: 'AI Builders Lightning Lab',
    description: 'Short demos, practical prompts, and small teams building useful prototypes in one evening.',
    city: 'Melbourne',
    venue: 'Collingwood Workshop',
    startsAt: relativeDate(4, 18, 0),
    capacity: 70,
    confirmedCount: 48,
    status: 'PUBLISHED',
    label: 'Featured',
  },
  {
    id: 'demo-brisbane-craft-night',
    source: 'demo',
    category: 'hobbies',
    rank: 86,
    title: 'Make Something Night',
    description: 'Bring a small project or start one there. Paper craft, sketching, journaling, and low-key music.',
    city: 'Brisbane',
    venue: 'West End Hall',
    startsAt: relativeDate(5, 17, 30),
    capacity: 36,
    confirmedCount: 24,
    status: 'PUBLISHED',
    label: 'Creative',
  },
  {
    id: 'demo-sydney-supper-club',
    source: 'demo',
    category: 'food-nightlife',
    rank: 84,
    title: 'Laneway Supper Club',
    description: 'Small plates, shared tables, and a short city walk after dinner.',
    city: 'Sydney',
    venue: 'Darlinghurst Laneway',
    startsAt: relativeDate(6, 20, 0),
    capacity: 30,
    confirmedCount: 26,
    status: 'PUBLISHED',
    label: 'Almost full',
  },
];

function render() {
  const route = parseRoute();
  app.innerHTML = `
    <a class="skip-link" href="#main-content">Skip to main content</a>
    ${renderTopbar(route)}
    <main id="main-content" class="page-shell page-${escapeHTML(route.name)}">
      ${renderPage(route)}
    </main>
    ${renderNotice()}
  `;
  bind();
}

function renderTopbar(route) {
  return `
    <header class="topbar">
      <a class="brand-lockup" href="/" data-route>
        <span class="brand-mark" aria-hidden="true">CE</span>
        <span>
          <span class="brand-name">CityEvents</span>
          <span class="brand-subtitle">Nearby plans, clear spots</span>
        </span>
      </a>
      <nav class="site-nav" aria-label="Main">
        ${navLink('Discover', '/', route)}
        ${navLink('Events', '/events', route)}
        ${navLink('Categories', '/categories', route)}
        ${navLink('Publish', '/publish', route)}
        ${navLink(state.auth.user ? 'Me' : 'Sign in', '/me', route)}
      </nav>
    </header>
  `;
}

function navLink(label, href, route) {
  const active = href === '/'
    ? route.name === 'home'
    : route.path === href || route.path.startsWith(`${href}/`);
  return `<a class="${active ? 'active' : ''}" href="${href}" data-route>${escapeHTML(label)}</a>`;
}

function renderPage(route) {
  if (route.name === 'eventDetail') return renderEventDetailPage(route.eventID);
  if (route.name === 'events') return renderEventsPage();
  if (route.name === 'categories') return renderCategoriesPage();
  if (route.name === 'category') return renderCategoryPage(route.slug);
  if (route.name === 'publish') return renderPublishPage();
  if (route.name === 'me') return renderAccountPage();
  return renderHomePage();
}

function renderHomePage() {
  const events = catalogEvents();
  const featureEvents = demoEvents.map((item) => normalizeEvent(item, 'demo', item.rank));
  const spotlight = featureEvents[0];
  const secondary = featureEvents.slice(1, 4);
  const cityEvents = events.filter((event) => event.city === state.filters.city).slice(0, 4);

  return `
    <section class="home-board" aria-label="CityEvents discovery">
      <div class="hero-panel">
        <div class="hero-copy">
          <p class="eyebrow">City radar</p>
          <h1>Pick a plan before the night disappears.</h1>
          <p>Fast discovery for social events, useful rooms, active weekends, and low-pressure ways to meet people.</p>
        </div>
        ${renderSearchForm('hero-search', 'Find events, categories, or venues')}
        <div class="hero-proof" aria-label="Discovery summary">
          <span><strong>${events.length}</strong> visible events</span>
          <span><strong>${categories.length}</strong> category lanes</span>
          <span><strong>${state.filters.city}</strong> default city</span>
        </div>
      </div>

      <section class="spotlight-card ${categoryAccent(spotlight)}" aria-labelledby="spotlight-title">
        ${renderEventImage(spotlight, 'spotlight-image')}
        <div class="spotlight-copy">
          <span class="label-pill">${escapeHTML(spotlight.label || 'Featured')}</span>
          <h2 id="spotlight-title">${escapeHTML(spotlight.title)}</h2>
          <p>${escapeHTML(spotlight.description)}</p>
          <div class="event-facts">
            <span>${escapeHTML(formatCompactDate(spotlight.startsAt))}</span>
            <span>${escapeHTML(spotlight.venue)}</span>
            <span>${escapeHTML(spotsLeft(spotlight))} spots left</span>
          </div>
          <a class="primary-button" href="/events/${encodeURIComponent(spotlight.id)}" data-route>View event</a>
        </div>
      </section>

      <aside class="signal-stack" aria-label="Popular events">
        <div class="section-heading compact">
          <span class="eyebrow">Popular now</span>
          <a href="/events" data-route>See all</a>
        </div>
        ${secondary.map(renderSignalEvent).join('')}
      </aside>

      <section class="category-strip" aria-label="Explore by category">
        ${categories.map(renderCategoryTile).join('')}
      </section>

      <section class="city-strip" aria-label="Upcoming near you">
        <div class="section-heading compact">
          <span><strong>${escapeHTML(state.filters.city)}</strong> upcoming</span>
          <button type="button" class="text-button" data-action="refresh-feed">${isPending('feed') ? 'Refreshing' : 'Refresh live feed'}</button>
        </div>
        <div class="city-event-row">
          ${(cityEvents.length ? cityEvents : demoEvents.slice(0, 4)).map((event) => renderMiniEvent(event)).join('')}
        </div>
      </section>
    </section>
  `;
}

function renderEventsPage() {
  const events = filteredEvents();
  return `
    <section class="browse-layout">
      <div class="browse-command">
        <p class="eyebrow">Browse events</p>
        <h1>Scan the city by category, date, and spots left.</h1>
        ${renderSearchForm('browse-filter', 'Search events')}
        ${renderFilterChips()}
      </div>
      <div class="browse-results">
        <div class="section-heading">
          <span><strong>${events.length}</strong> results</span>
          <button type="button" class="secondary-button" data-action="refresh-feed">${isPending('feed') ? 'Refreshing' : 'Refresh live feed'}</button>
        </div>
        <div class="event-grid">
          ${events.length ? events.map((event) => renderEventCard(event)).join('') : renderEmptyEvents()}
        </div>
      </div>
    </section>
  `;
}

function renderCategoriesPage() {
  return `
    <section class="category-page">
      <div class="page-intro">
        <p class="eyebrow">Category lanes</p>
        <h1>Choose the reason you want to leave the house.</h1>
        <p>Each lane starts with strong visual cues and compact event cards so the user does not need to hunt.</p>
      </div>
      <div class="category-grid">
        ${categories.map((category) => renderCategoryFeature(category)).join('')}
      </div>
    </section>
  `;
}

function renderCategoryPage(slug) {
  const category = categoryBySlug.get(slug) || categories[0];
  const events = filteredEvents({ category: category.slug });
  return `
    <section class="category-detail">
      <div class="category-hero ${category.accent}">
        <img src="${escapeHTML(category.image)}" alt="" />
        <div>
          <p class="eyebrow">Category</p>
          <h1>${escapeHTML(category.name)}</h1>
          <p>${escapeHTML(category.line)}</p>
          <a class="secondary-button" href="/categories" data-route>All categories</a>
        </div>
      </div>
      <div class="event-grid">
        ${events.length ? events.map((event) => renderEventCard(event)).join('') : renderEmptyEvents(category.name)}
      </div>
    </section>
  `;
}

function renderEventDetailPage(eventID) {
  const event = getEventForDisplay(eventID);
  if (!event) {
    return `
      <section class="detail-page">
        <div class="state-message">
          <strong>Event not found</strong>
          <span>Go back to discovery and choose another event.</span>
          <a class="primary-button" href="/events" data-route>Browse events</a>
        </div>
      </section>
    `;
  }

  const isLive = event.source !== 'demo';
  const detail = isLive && state.eventDetail?.event?.id === event.id ? state.eventDetail : null;
  const joinStatus = detail?.viewerJoinStatus || 'NOT_JOINED';
  const confirmedCount = Number(detail?.confirmedCount ?? event.confirmedCount ?? 0);
  const joined = joinStatus === 'CONFIRMED' || joinStatus === 'WAITLISTED';
  const joinDisabled = isLive && state.auth.user?.id ? !canJoin(joinStatus) || isPending('join') : false;
  const cancelDisabled = !isLive || !state.auth.user?.id || !canCancel(joinStatus) || isPending('cancel');

  return `
    <section class="detail-page">
      <article class="event-detail-card ${categoryAccent(event)}">
        ${renderEventImage(event, 'detail-image')}
        <div class="detail-copy">
          <div class="detail-topline">
            <span class="label-pill">${escapeHTML(categoryName(event.category))}</span>
            <span class="label-pill soft">${isLive ? 'Live RSVP' : 'Preview'}</span>
          </div>
          <h1>${escapeHTML(event.title)}</h1>
          <p>${escapeHTML(event.description || 'No description provided yet.')}</p>
          <dl class="fact-grid">
            <div><dt>When</dt><dd>${escapeHTML(formatDateTime(event.startsAt))}</dd></div>
            <div><dt>Where</dt><dd>${escapeHTML(event.city)} / ${escapeHTML(event.venue)}</dd></div>
            <div><dt>Capacity</dt><dd>${escapeHTML(String(confirmedCount))} / ${escapeHTML(String(event.capacity || '-'))}</dd></div>
          </dl>
        </div>
      </article>

      <aside class="rsvp-panel">
        <div>
          <p class="eyebrow">Your spot</p>
          <h2>${escapeHTML(rsvpHeadline(event, joinStatus, isLive))}</h2>
          <p>${escapeHTML(rsvpDetail(event, joinStatus, isLive))}</p>
        </div>
        <div class="spot-meter" aria-label="Spots left">
          <span style="--fill:${spotFill(event, confirmedCount)}%"></span>
        </div>
        <div class="rsvp-actions">
          <button type="button" class="primary-button" data-action="${isLive && state.auth.user?.id ? 'join' : isLive ? 'need-auth' : 'preview-event'}" ${joinDisabled ? 'disabled' : ''}>
            ${isPending('join') ? 'Joining' : isLive ? state.auth.user?.id ? joined ? 'Joined' : 'Join event' : 'Sign in to join' : 'Preview only'}
          </button>
          <button type="button" class="secondary-button" data-action="cancel-join" ${cancelDisabled ? 'disabled' : ''}>
            ${isPending('cancel') ? 'Canceling' : 'Cancel RSVP'}
          </button>
        </div>
        <a class="text-button" href="/events" data-route>Back to events</a>
      </aside>
    </section>
  `;
}

function renderPublishPage() {
  const signedIn = Boolean(state.auth.user?.id);
  return `
    <section class="publish-layout">
      <div class="page-intro">
        <p class="eyebrow">Organizer</p>
        <h1>Publish a real event into the live feed.</h1>
        <p>Keep creation focused: title, place, time, capacity, and a short reason to show up.</p>
      </div>
      <div class="publish-card">
        ${signedIn ? renderCreateEventForm() : renderAuthPanel('Sign in to publish', 'Create your account first, then the publish form unlocks.')}
      </div>
    </section>
  `;
}

function renderAccountPage() {
  if (!state.auth.user) {
    return `
      <section class="account-layout">
        <div class="page-intro">
          <p class="eyebrow">Account</p>
          <h1>Sign in only when you are ready to act.</h1>
          <p>Visitors can browse first. Joining, canceling, publishing, and media upload intents require identity.</p>
        </div>
        ${renderAuthPanel('Enter CityEvents', 'Use a 12+ character password with mixed case and a number.')}
      </section>
    `;
  }

  return `
    <section class="account-layout signed-in">
      <div class="profile-card">
        <p class="eyebrow">Signed in</p>
        <h1>${escapeHTML(state.auth.user.displayName || 'CityEvents user')}</h1>
        <p>${escapeHTML(state.auth.user.email || state.auth.user.id)}</p>
        <button type="button" class="secondary-button" data-action="logout">Log out</button>
      </div>
      <div class="account-actions">
        <section class="compact-panel">
          <div class="section-heading">
            <span><strong>Selected event</strong></span>
            <a href="/events" data-route>Choose event</a>
          </div>
          ${renderSelectedEventSummary()}
        </section>
        ${renderMediaPanel()}
      </div>
    </section>
  `;
}

function renderSearchForm(id, placeholder) {
  return `
    <form id="${id}" class="search-form">
      <label>
        <span>Search</span>
        <input name="keyword" value="${escapeHTML(state.filters.keyword)}" placeholder="${escapeHTML(placeholder)}" />
      </label>
      <label>
        <span>City</span>
        <select name="city">
          ${quickCities.map((city) => `<option value="${escapeHTML(city)}" ${state.filters.city === city ? 'selected' : ''}>${escapeHTML(city)}</option>`).join('')}
        </select>
      </label>
      <label>
        <span>Category</span>
        <select name="category">
          <option value="">All</option>
          ${categories.map((category) => `<option value="${escapeHTML(category.slug)}" ${state.filters.category === category.slug ? 'selected' : ''}>${escapeHTML(category.name)}</option>`).join('')}
        </select>
      </label>
      <button type="submit" class="primary-button">Explore</button>
    </form>
  `;
}

function renderFilterChips() {
  return `
    <div class="filter-chips" aria-label="Quick filters">
      ${categories.slice(0, 5).map((category) => `
        <button type="button" class="chip-button ${state.filters.category === category.slug ? 'selected' : ''}" data-filter-category="${escapeHTML(category.slug)}">
          ${escapeHTML(category.name)}
        </button>
      `).join('')}
      <button type="button" class="chip-button ${state.filters.category === '' ? 'selected' : ''}" data-filter-category="">All</button>
    </div>
  `;
}

function renderSignalEvent(event) {
  return `
    <a class="signal-event" href="/events/${encodeURIComponent(event.id)}" data-route>
      <span class="signal-date">${escapeHTML(dayLabel(event.startsAt))}</span>
      <span>
        <strong>${escapeHTML(event.title)}</strong>
        <small>${escapeHTML(categoryName(event.category))} / ${escapeHTML(event.city)}</small>
      </span>
      <em>${escapeHTML(spotsLeft(event))}</em>
    </a>
  `;
}

function renderCategoryTile(category) {
  return `
    <a class="category-tile ${escapeHTML(category.accent)}" href="/categories/${escapeHTML(category.slug)}" data-route>
      <img src="${escapeHTML(category.image)}" alt="" />
      <span>
        <strong>${escapeHTML(category.name)}</strong>
        <small>${escapeHTML(category.short)}</small>
      </span>
    </a>
  `;
}

function renderCategoryFeature(category) {
  const count = catalogEvents().filter((event) => event.category === category.slug).length;
  return `
    <a class="category-feature ${escapeHTML(category.accent)}" href="/categories/${escapeHTML(category.slug)}" data-route>
      <img src="${escapeHTML(category.image)}" alt="" />
      <span class="label-pill">${count} events</span>
      <h2>${escapeHTML(category.name)}</h2>
      <p>${escapeHTML(category.line)}</p>
    </a>
  `;
}

function renderMiniEvent(event) {
  return `
    <a class="mini-event" href="/events/${encodeURIComponent(event.id)}" data-route>
      <strong>${escapeHTML(event.title)}</strong>
      <span>${escapeHTML(formatCompactDate(event.startsAt))} / ${escapeHTML(spotsLeft(event))} spots</span>
    </a>
  `;
}

function renderEventCard(event) {
  return `
    <a class="event-card ${categoryAccent(event)}" href="/events/${encodeURIComponent(event.id)}" data-route>
      ${renderEventImage(event, 'card-image')}
      <span class="label-pill">${escapeHTML(event.label || categoryName(event.category))}</span>
      <h2>${escapeHTML(event.title)}</h2>
      <p>${escapeHTML(event.description)}</p>
      <div class="event-card-meta">
        <span>${escapeHTML(formatCompactDate(event.startsAt))}</span>
        <span>${escapeHTML(event.city)}</span>
        <span>${escapeHTML(spotsLeft(event))} spots</span>
      </div>
    </a>
  `;
}

function renderEventImage(event, className) {
  const category = categoryBySlug.get(event.category) || categories[0];
  return `<img class="${escapeHTML(className)}" src="${escapeHTML(category.image)}" alt="" />`;
}

function renderEmptyEvents(categoryNameValue = 'this lane') {
  return `
    <div class="state-message">
      <strong>No live events in ${escapeHTML(categoryNameValue)} yet</strong>
      <span>Publish one to test the live RSVP and feed projection flow.</span>
      <a class="primary-button" href="/publish" data-route>Publish event</a>
    </div>
  `;
}

function renderAuthPanel(title, detail) {
  return `
    <section class="auth-card">
      <div>
        <p class="eyebrow">Account</p>
        <h2>${escapeHTML(title)}</h2>
        <p>${escapeHTML(detail)}</p>
      </div>
      <form id="auth-form" class="stacked-form">
        <label>
          <span>Display name</span>
          <input name="displayName" placeholder="Avery" autocomplete="name" />
        </label>
        <label>
          <span>Email</span>
          <input name="email" type="email" placeholder="avery@example.com" autocomplete="email" required />
        </label>
        <label>
          <span>Password</span>
          <input name="password" type="password" placeholder="12+ chars, mixed case, number" autocomplete="current-password" required />
        </label>
        <div class="button-pair">
          <button type="submit" class="primary-button" data-mode="login" ${isPending('auth') ? 'disabled' : ''}>Log in</button>
          <button type="submit" class="secondary-button" data-mode="register" ${isPending('auth') ? 'disabled' : ''}>Register</button>
        </div>
      </form>
    </section>
  `;
}

function renderCreateEventForm() {
  return `
    <form id="create-event-form" class="stacked-form publish-form">
      <label>
        <span>Title</span>
        <input name="title" placeholder="Rooftop board game night" required />
      </label>
      <div class="form-pair">
        <label>
          <span>City</span>
          <input name="city" placeholder="Sydney" value="${escapeHTML(state.filters.city)}" required />
        </label>
        <label>
          <span>Venue</span>
          <input name="venue" placeholder="Surry Hills" required />
        </label>
      </div>
      <div class="form-pair">
        <label>
          <span>Start time</span>
          <input name="startsAt" type="datetime-local" value="${escapeHTML(defaultStartAt())}" required />
        </label>
        <label>
          <span>Capacity</span>
          <input name="capacity" type="number" min="1" value="24" required />
        </label>
      </div>
      <label>
        <span>Description</span>
        <textarea name="description" placeholder="What should people expect?"></textarea>
      </label>
      <button type="submit" class="primary-button" ${isPending('create') ? 'disabled' : ''}>${isPending('create') ? 'Publishing' : 'Publish event'}</button>
    </form>
  `;
}

function renderSelectedEventSummary() {
  const event = getEventForDisplay(state.selectedEvent);
  if (!event) {
    return `<div class="state-message"><strong>No event selected</strong><span>Open an event before creating media upload intent.</span></div>`;
  }
  return `
    <a class="selected-event" href="/events/${encodeURIComponent(event.id)}" data-route>
      ${renderEventImage(event, 'selected-image')}
      <span>
        <strong>${escapeHTML(event.title)}</strong>
        <small>${escapeHTML(formatCompactDate(event.startsAt))} / ${escapeHTML(event.venue)}</small>
      </span>
    </a>
  `;
}

function renderMediaPanel() {
  const event = getEventForDisplay(state.selectedEvent);
  const liveSelected = event && event.source !== 'demo';
  return `
    <section class="compact-panel">
      <div class="section-heading">
        <span><strong>Media upload</strong></span>
        <span class="label-pill soft">Intent</span>
      </div>
      <form id="media-form" class="stacked-form">
        <label>
          <span>Event ID</span>
          <input name="eventId" value="${escapeHTML(liveSelected ? state.selectedEvent : '')}" placeholder="Select a live event first" required />
        </label>
        <div class="form-pair">
          <label>
            <span>Filename</span>
            <input name="filename" placeholder="banner.jpg" value="banner.jpg" required />
          </label>
          <label>
            <span>Type</span>
            <select name="contentType">
              <option value="image/jpeg">image/jpeg</option>
              <option value="image/png">image/png</option>
              <option value="image/webp">image/webp</option>
            </select>
          </label>
        </div>
        <label>
          <span>Size bytes</span>
          <input name="sizeBytes" type="number" min="1" value="1024" required />
        </label>
        <button type="submit" class="secondary-button" ${!liveSelected || isPending('media') ? 'disabled' : ''}>${isPending('media') ? 'Creating' : 'Create upload intent'}</button>
        ${state.media ? renderMediaResult() : '<p class="form-hint">Media is available for live events created in CityEvents.</p>'}
      </form>
    </section>
  `;
}

function renderMediaResult() {
  const asset = state.media.asset || {};
  return `
    <div class="media-result">
      <span class="label-pill ${toneClass(asset.status)}">${escapeHTML(statusLabel(asset.status))}</span>
      <code>${escapeHTML(asset.id || 'media-id-pending')}</code>
    </div>
  `;
}

function renderNotice() {
  if (!state.notice) return '';
  return `
    <div class="notice-toast tone-${escapeHTML(state.noticeTone)}" aria-live="polite">
      <span>${escapeHTML(state.notice)}</span>
    </div>
  `;
}

function bind() {
  document.querySelectorAll('a[data-route]').forEach((anchor) => {
    anchor.addEventListener('click', handleRouteClick);
  });
  document.querySelector('#hero-search')?.addEventListener('submit', handleSearch);
  document.querySelector('#browse-filter')?.addEventListener('submit', handleSearch);
  document.querySelector('#auth-form')?.addEventListener('submit', handleAuth);
  document.querySelector('#create-event-form')?.addEventListener('submit', handleCreateEvent);
  document.querySelector('#media-form')?.addEventListener('submit', handleCreateUpload);
  document.querySelector('[data-action="logout"]')?.addEventListener('click', handleLogout);
  document.querySelector('[data-action="join"]')?.addEventListener('click', handleJoin);
  document.querySelector('[data-action="need-auth"]')?.addEventListener('click', handleNeedAuth);
  document.querySelector('[data-action="preview-event"]')?.addEventListener('click', () => setNotice('Preview events show the discovery experience. Publish a live event to test RSVP.', 'neutral'));
  document.querySelector('[data-action="cancel-join"]')?.addEventListener('click', handleCancelJoin);
  document.querySelectorAll('[data-action="refresh-feed"]').forEach((button) => {
    button.addEventListener('click', () => refreshFeed());
  });
  document.querySelectorAll('[data-filter-category]').forEach((button) => {
    button.addEventListener('click', () => {
      state.filters.category = button.dataset.filterCategory || '';
      navigate('/events');
    });
  });
}

function handleRouteClick(event) {
  if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  const href = event.currentTarget.getAttribute('href');
  if (!href || href.startsWith('http') || href.startsWith('mailto:')) return;
  event.preventDefault();
  navigate(href);
}

function navigate(path) {
  if (window.location.pathname !== path) {
    window.history.pushState({}, '', path);
  }
  render();
  syncRoute();
  window.scrollTo({ top: 0, behavior: 'auto' });
}

function parseRoute() {
  const path = window.location.pathname || '/';
  const segments = path.split('/').filter(Boolean).map(decodeURIComponent);
  if (!segments.length) return { name: 'home', path };
  if (segments[0] === 'events' && segments[1]) return { name: 'eventDetail', path, eventID: segments[1] };
  if (segments[0] === 'events') return { name: 'events', path };
  if (segments[0] === 'categories' && segments[1]) return { name: 'category', path, slug: segments[1] };
  if (segments[0] === 'categories') return { name: 'categories', path };
  if (segments[0] === 'publish') return { name: 'publish', path };
  if (segments[0] === 'me') return { name: 'me', path };
  return { name: 'home', path: '/' };
}

async function syncRoute() {
  const route = parseRoute();
  if (route.name !== 'eventDetail') return;
  const event = getEventForDisplay(route.eventID);
  if (!event) return;
  state.selectedEvent = route.eventID;
  if (event.source === 'demo') {
    state.eventDetail = null;
    state.media = null;
    return;
  }
  if (state.eventDetail?.event?.id === route.eventID || isPending('detail')) return;
  await loadEventDetail(route.eventID);
}

async function handleSearch(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  state.filters.keyword = String(form.get('keyword') || '').trim();
  state.filters.city = String(form.get('city') || state.filters.city || 'Sydney');
  state.filters.category = String(form.get('category') || '');
  state.feed.city = state.filters.city;
  await refreshFeed({ renderAfter: false });
  navigate('/events');
}

async function handleAuth(event) {
  event.preventDefault();
  const button = event.submitter;
  const form = new FormData(event.currentTarget);
  const data = Object.fromEntries(form.entries());
  await withPending('auth', async () => {
    state.auth = button.dataset.mode === 'register'
      ? await api.register(data)
      : await api.login({ email: data.email, password: data.password });
    setNotice(button.dataset.mode === 'register' ? 'Account created.' : 'Signed in.', 'good');
    await refreshFeed({ renderAfter: false });
    const returnTo = state.returnTo;
    state.returnTo = '';
    if (returnTo) {
      navigate(returnTo);
    }
  });
}

async function handleLogout() {
  await withPending('auth', async () => {
    try {
      await api.logout();
    } catch {
      api.clearAuth();
    }
    state.auth = api.loadAuth();
    state.eventDetail = null;
    state.media = null;
    setNotice('Signed out.', 'neutral');
    navigate('/');
  });
}

function handleNeedAuth() {
  state.returnTo = window.location.pathname;
  setNotice('Sign in to reserve a spot.', 'neutral');
  navigate('/me');
}

async function refreshFeed(options = {}) {
  const renderAfter = options.renderAfter !== false;
  state.feed.loading = true;
  state.feed.error = '';
  state.pendingAction = 'feed';
  if (renderAfter) render();
  try {
    const payload = await api.listFeed(state.feed.city || state.filters.city || '');
    state.feed.events = payload.events || [];
    state.feed.city = state.filters.city;
    if (renderAfter) setNotice('Live feed refreshed.', 'neutral');
  } catch (error) {
    state.feed.error = error.message;
    if (renderAfter) setNotice(error.message, 'bad');
  } finally {
    state.feed.loading = false;
    state.pendingAction = '';
    if (renderAfter) render();
  }
}

async function loadEventDetail(eventID) {
  state.selectedEvent = eventID;
  state.pendingAction = 'detail';
  render();
  try {
    state.eventDetail = await api.getEvent(eventID);
    state.media = null;
    setNotice('Event status loaded.', 'neutral');
  } catch (error) {
    setNotice(error.message, 'bad');
  } finally {
    state.pendingAction = '';
    render();
  }
}

async function handleCreateEvent(event) {
  event.preventDefault();
  if (!state.auth.user?.id) {
    state.returnTo = '/publish';
    setNotice('Sign in before publishing.', 'neutral');
    navigate('/me');
    return;
  }
  const form = new FormData(event.currentTarget);
  await withPending('create', async () => {
    const detail = await api.createEvent({
      title: String(form.get('title') || ''),
      description: String(form.get('description') || ''),
      city: String(form.get('city') || ''),
      venue: String(form.get('venue') || ''),
      startsAt: new Date(String(form.get('startsAt'))).toISOString(),
      capacity: Number(form.get('capacity') || 1),
    });
    state.selectedEvent = detail.event.id;
    state.eventDetail = detail;
    state.filters.city = detail.event.city || state.filters.city;
    state.feed.city = state.filters.city;
    setNotice('Event published.', 'good');
    await refreshFeed({ renderAfter: false });
    navigate(`/events/${detail.event.id}`);
  });
}

async function handleJoin() {
  if (!state.selectedEvent) return;
  if (!state.auth.user?.id) {
    handleNeedAuth();
    return;
  }
  const event = getEventForDisplay(state.selectedEvent);
  if (!event || event.source === 'demo') {
    setNotice('Preview events cannot be joined. Publish or open a live event.', 'neutral');
    return;
  }
  await withPending('join', async () => {
    await api.joinEvent(state.selectedEvent);
    state.eventDetail = await api.getEvent(state.selectedEvent);
    setNotice('You are going.', 'good');
  });
}

async function handleCancelJoin() {
  if (!state.selectedEvent) return;
  await withPending('cancel', async () => {
    await api.cancelJoin(state.selectedEvent);
    state.eventDetail = await api.getEvent(state.selectedEvent);
    setNotice('RSVP canceled.', 'neutral');
  });
}

async function handleCreateUpload(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  await withPending('media', async () => {
    state.media = await api.createUpload({
      eventId: String(form.get('eventId') || ''),
      filename: String(form.get('filename') || ''),
      contentType: String(form.get('contentType') || ''),
      sizeBytes: Number(form.get('sizeBytes') || 0),
    });
    setNotice('Media upload intent created.', 'good');
  });
}

async function withPending(action, fn) {
  state.pendingAction = action;
  render();
  try {
    await fn();
  } catch (error) {
    setNotice(error.message, 'bad');
  } finally {
    state.pendingAction = '';
    render();
  }
}

function catalogEvents() {
  const live = (state.feed.events || []).map((item, index) => normalizeEvent(item, 'live', 120 - index));
  const demos = demoEvents.map((item) => normalizeEvent(item, 'demo', item.rank));
  const seen = new Set();
  return [...live, ...demos]
    .filter((event) => {
      if (seen.has(event.id)) return false;
      seen.add(event.id);
      return true;
    })
    .sort((a, b) => Number(b.rank || 0) - Number(a.rank || 0));
}

function filteredEvents(overrides = {}) {
  const filters = { ...state.filters, ...overrides };
  const keyword = filters.keyword.trim().toLowerCase();
  return catalogEvents().filter((event) => {
    if (filters.city && event.city !== filters.city) return false;
    if (filters.category && event.category !== filters.category) return false;
    if (!keyword) return true;
    return [event.title, event.description, event.venue, event.city, categoryName(event.category)]
      .join(' ')
      .toLowerCase()
      .includes(keyword);
  });
}

function getEventForDisplay(eventID) {
  if (!eventID) return null;
  if (state.eventDetail?.event?.id === eventID) {
    return normalizeEvent(state.eventDetail.event, 'live', 130, state.eventDetail.confirmedCount);
  }
  return catalogEvents().find((event) => event.id === eventID) || null;
}

function normalizeEvent(item, source = 'live', rank = 0, confirmedOverride = undefined) {
  const event = item.event || item;
  const category = event.category || inferCategory(event);
  const confirmedCount = Number(confirmedOverride ?? item.confirmedCount ?? event.confirmedCount ?? estimateConfirmed(event, rank));
  return {
    id: event.id || event.eventId || '',
    source,
    category,
    rank,
    title: event.title || 'Untitled event',
    description: event.description || categoryBySlug.get(category)?.line || 'A local event worth checking out.',
    city: event.city || state.filters.city || 'Sydney',
    venue: event.venue || 'Venue pending',
    startsAt: event.startsAt || relativeDate(2, 18, 0),
    capacity: Number(event.capacity || 24),
    confirmedCount,
    status: event.status || 'PUBLISHED',
    label: event.label || (source === 'live' ? 'Live' : categoryName(category)),
  };
}

function inferCategory(event) {
  const text = [event.title, event.description, event.venue].join(' ').toLowerCase();
  if (/founder|career|business|network|startup|professional/.test(text)) return 'networking';
  if (/friend|social|brunch|board|new in town|hangout/.test(text)) return 'new-friends';
  if (/sport|run|hike|pickleball|football|basketball|outdoor|gym/.test(text)) return 'sports-outdoors';
  if (/book|craft|game|photo|anime|cook|music|art/.test(text)) return 'hobbies';
  if (/tech|ai|code|study|workshop|design|product/.test(text)) return 'learning-tech';
  if (/food|dinner|supper|bar|night|market|taste/.test(text)) return 'food-nightlife';
  return 'new-friends';
}

function estimateConfirmed(event, rank) {
  const capacity = Number(event.capacity || 24);
  return Math.max(0, Math.min(capacity - 1, Math.round(capacity * (0.42 + ((rank || 0) % 30) / 100))));
}

function categoryName(slug) {
  return categoryBySlug.get(slug)?.name || 'Event';
}

function categoryAccent(event) {
  return categoryBySlug.get(event.category)?.accent || 'teal';
}

function spotsLeft(event) {
  return Math.max(0, Number(event.capacity || 0) - Number(event.confirmedCount || 0));
}

function spotFill(event, confirmedCount) {
  const capacity = Number(event.capacity || 1);
  return Math.max(6, Math.min(100, Math.round((Number(confirmedCount || 0) / capacity) * 100)));
}

function rsvpHeadline(event, joinStatus, isLive) {
  if (!isLive) return 'Preview listing';
  if (!state.auth.user?.id) return 'Sign in to reserve a spot';
  switch (joinStatus) {
    case 'CONFIRMED': return 'You are going';
    case 'WAITLISTED': return 'You are waitlisted';
    case 'CANCELED': return 'You canceled this RSVP';
    default: return `${spotsLeft(event)} spots left`;
  }
}

function rsvpDetail(event, joinStatus, isLive) {
  if (!isLive) return 'This card shows the discovery experience. Live RSVP is available for events created in CityEvents.';
  if (!state.auth.user?.id) return 'Browsing stays open. Sign in only when you want to join, publish, or manage media.';
  switch (joinStatus) {
    case 'CONFIRMED': return 'Your spot is confirmed. Cancel if plans change.';
    case 'WAITLISTED': return 'You are in line if another attendee cancels.';
    case 'CANCELED': return 'You can join again if the event is still open.';
    default: return 'Join once and your status updates here.';
  }
}

function isPending(action) {
  return state.pendingAction === action;
}

function setNotice(message, tone = 'neutral') {
  state.notice = message;
  state.noticeTone = tone;
}

function defaultStartAt() {
  const date = new Date(Date.now() + 24 * 60 * 60 * 1000);
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60 * 1000);
  return local.toISOString().slice(0, 16);
}

function relativeDate(days, hour, minute) {
  const date = new Date();
  date.setDate(date.getDate() + days);
  date.setHours(hour, minute, 0, 0);
  return date.toISOString();
}

function formatCompactDate(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'Time pending';
  return date.toLocaleString('en-AU', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

function dayLabel(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--';
  return date.toLocaleString('en-AU', { day: '2-digit' });
}

function toneClass(status) {
  return `tone-${statusTone[status] || 'neutral'}`;
}

function escapeHTML(value) {
  return String(value ?? '').replace(/[&<>"']/g, (char) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }[char]));
}

window.addEventListener('popstate', () => {
  render();
  syncRoute();
});

render();
refreshFeed({ renderAfter: false }).finally(() => {
  render();
  syncRoute();
});
