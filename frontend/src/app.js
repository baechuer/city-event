import { createApiClient } from './api.js';
import { canCancel, canJoin, createInitialState, formatDateTime, statusLabel, statusTone } from './state.js';

const api = createApiClient(window.CITYEVENTS_CONFIG || {});
const state = createInitialState();
state.auth = api.loadAuth();

const app = document.querySelector('#app');
const quickCities = ['Sydney', 'Melbourne', 'Brisbane'];
const workflowSteps = [
  ['01', 'Sign in', 'Register or log in'],
  ['02', 'Discover', 'Filter the city feed'],
  ['03', 'Inspect', 'Open event details'],
  ['04', 'Act', 'Join, cancel, publish, upload'],
];

function render() {
  app.innerHTML = `
    <a class="skip-link" href="#main-content">Skip to main content</a>
    <header class="topbar">
      <div class="brand-lockup">
        <span class="brand-mark" aria-hidden="true">CE</span>
        <div>
          <div class="brand-name">CityEvents</div>
          <div class="brand-subtitle">Local plans, clear spots</div>
        </div>
      </div>
      <nav class="top-actions" aria-label="Primary">
        <button type="button" class="ghost-button" data-action="refresh-feed">Refresh feed</button>
        <button type="button" class="primary-button" data-action="focus-create">Publish event</button>
      </nav>
    </header>

    <main id="main-content" class="workspace">
      <section class="overview-band" aria-label="Workflow overview">
        <div class="overview-copy">
          <p class="eyebrow">Reviewer path</p>
          <h1>Find the plan, prove the spot, publish the next one.</h1>
          <p class="overview-text">A compact workspace for discovering local plans, reserving a spot, publishing events, and preparing event media.</p>
        </div>
        <ol class="journey-rail" aria-label="Suggested walkthrough">
          ${workflowSteps.map(([number, title, detail]) => `
            <li>
              <span>${number}</span>
              <strong>${title}</strong>
              <small>${detail}</small>
            </li>
          `).join('')}
        </ol>
      </section>

      <section class="notice-strip tone-${escapeHTML(state.noticeTone)}" aria-live="polite">
        <span>${escapeHTML(state.notice || defaultNotice())}</span>
        <span class="run-hint">Local demo mode</span>
      </section>

      <section class="app-grid" aria-label="CityEvents workspace">
        <div class="feed-column">
          ${renderFeedControls()}
          ${renderFeed()}
        </div>
        ${renderDetail()}
        <aside class="action-column" aria-label="Account and actions">
          ${renderAuthPanel()}
          ${renderCreateEvent()}
          ${renderMedia()}
        </aside>
      </section>
    </main>
  `;
  bind();
}

function defaultNotice() {
  return state.auth.user
    ? 'Select an event, join it, cancel it, or publish a new one.'
    : 'Sign in first to create events, join events, and create media upload intents.';
}

function renderFeedControls() {
  return `
    <section class="control-panel" aria-labelledby="feed-filter-title">
      <div>
        <p class="eyebrow">Discovery</p>
        <h2 id="feed-filter-title">City feed</h2>
      </div>
      <form id="feed-filter" class="feed-filter">
        <label>
          <span>City filter</span>
          <input name="city" value="${escapeHTML(state.feed.city)}" placeholder="Sydney" autocomplete="address-level2" />
        </label>
        <button type="submit" class="primary-button" ${isPending('feed') ? 'disabled' : ''}>${isPending('feed') ? 'Refreshing' : 'Refresh'}</button>
      </form>
      <div class="quick-cities" aria-label="Quick city filters">
        ${quickCities.map((city) => `
          <button type="button" class="chip-button ${state.feed.city === city ? 'selected' : ''}" data-city="${escapeHTML(city)}">${escapeHTML(city)}</button>
        `).join('')}
      </div>
    </section>
  `;
}

function renderAuthPanel() {
  if (state.auth.user) {
    return `
      <section class="action-panel account-panel">
        <div class="panel-title-row">
          <div>
            <p class="eyebrow">Account</p>
            <h2>Ready to act</h2>
          </div>
          <span class="status-dot good" aria-label="Signed in"></span>
        </div>
        <div class="identity-card">
          <strong>${escapeHTML(state.auth.user.displayName || state.auth.user.email || 'Signed-in user')}</strong>
          <span>${escapeHTML(state.auth.user.email || state.auth.user.id)}</span>
        </div>
        <button type="button" class="secondary-button" data-action="logout">Log out</button>
      </section>
    `;
  }

  return `
    <section class="action-panel account-panel">
      <div class="panel-title-row">
        <div>
          <p class="eyebrow">Account</p>
          <h2>Start here</h2>
        </div>
        <span class="status-dot neutral" aria-label="Signed out"></span>
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

function renderFeed() {
  let content = '';
  if (state.feed.loading) {
    content = Array.from({ length: 4 }, (_, index) => `<div class="event-skeleton" style="--row:${index + 1}"></div>`).join('');
  } else if (state.feed.error) {
    content = `<div class="state-message error-state"><strong>Feed unavailable</strong><span>${escapeHTML(state.feed.error)}</span></div>`;
  } else if (!state.feed.events.length) {
    content = `
      <div class="state-message empty-state">
        <strong>No events in this feed yet</strong>
        <span>Try another city or publish the first event for this demo run.</span>
        <button type="button" class="secondary-button" data-action="focus-create">Publish an event</button>
      </div>
    `;
  } else {
    content = state.feed.events.map(renderEventRow).join('');
  }

  return `
    <section class="feed-panel" aria-labelledby="feed-title">
      <div class="panel-header">
        <div>
          <h2 id="feed-title">Published events</h2>
          <p>${state.feed.events.length} result${state.feed.events.length === 1 ? '' : 's'}${state.feed.city ? ` in ${escapeHTML(state.feed.city)}` : ''}</p>
        </div>
        <span class="count-chip">${state.feed.events.length}</span>
      </div>
      <div class="event-list">${content}</div>
    </section>
  `;
}

function renderEventRow(item) {
  const event = item.event || item;
  const eventID = event.id || event.eventId || '';
  const selected = state.selectedEvent === eventID;
  return `
    <button type="button" class="event-row ${selected ? 'selected' : ''}" data-event-id="${escapeHTML(eventID)}">
      <span class="date-tile">
        <strong>${escapeHTML(monthLabel(event.startsAt))}</strong>
        <small>${escapeHTML(dayLabel(event.startsAt))}</small>
      </span>
      <span class="event-main">
        <strong>${escapeHTML(event.title || 'Untitled event')}</strong>
        <small>${escapeHTML([event.city, event.venue].filter(Boolean).join(' / ') || 'Location pending')}</small>
      </span>
      <span class="event-meta">
        <span>${escapeHTML(timeLabel(event.startsAt))}</span>
        <span class="pill ${toneClass(event.status)}">${escapeHTML(statusLabel(event.status || 'PUBLISHED'))}</span>
      </span>
    </button>
  `;
}

function renderDetail() {
  if (!state.eventDetail) {
    return `
      <section class="detail-panel empty-detail" aria-labelledby="detail-title">
        <div>
          <p class="eyebrow">Event detail</p>
          <h2 id="detail-title">Pick an event to see the current status.</h2>
          <p>Feed rows are for discovery. The detail panel shows your spot, capacity, and join actions.</p>
        </div>
        <div class="detail-placeholder" aria-hidden="true">
          <span></span><span></span><span></span>
        </div>
      </section>
    `;
  }

  const detail = state.eventDetail;
  const event = detail.event;
  const joinStatus = detail.viewerJoinStatus || 'NOT_JOINED';
  const signedIn = Boolean(state.auth.user?.id);
  const joinDisabled = !signedIn || !canJoin(joinStatus) || isPending('join');
  const cancelDisabled = !signedIn || !canCancel(joinStatus) || isPending('cancel');

  return `
    <section class="detail-panel" aria-labelledby="detail-title">
      <div class="detail-hero">
        <div>
          <p class="eyebrow">Event detail</p>
          <h2 id="detail-title">${escapeHTML(event.title)}</h2>
          <p>${escapeHTML(event.description || 'No description provided yet.')}</p>
        </div>
        <span class="pill large ${toneClass(joinStatus)}">${escapeHTML(statusLabel(joinStatus))}</span>
      </div>

      <dl class="detail-metrics">
        <div>
          <dt>Where</dt>
          <dd>${escapeHTML(event.city)} / ${escapeHTML(event.venue)}</dd>
        </div>
        <div>
          <dt>When</dt>
          <dd>${escapeHTML(formatDateTime(event.startsAt))}</dd>
        </div>
        <div>
          <dt>Capacity</dt>
          <dd>${escapeHTML(String(detail.confirmedCount ?? 0))} / ${escapeHTML(String(event.capacity ?? '-'))}</dd>
        </div>
      </dl>

      <div class="status-copy ${toneClass(joinStatus)}">
        <strong>${escapeHTML(joinStatusHeadline(joinStatus, signedIn))}</strong>
        <span>${escapeHTML(joinStatusDetail(joinStatus, signedIn))}</span>
      </div>

      <div class="action-row">
        <button type="button" class="primary-button" data-action="join" ${joinDisabled ? 'disabled' : ''}>${isPending('join') ? 'Joining' : 'Join event'}</button>
        <button type="button" class="secondary-button" data-action="cancel-join" ${cancelDisabled ? 'disabled' : ''}>${isPending('cancel') ? 'Canceling' : 'Cancel join'}</button>
      </div>
    </section>
  `;
}

function renderCreateEvent() {
  const disabled = !state.auth.user?.id || isPending('create');
  return `
    <section class="action-panel" id="create-panel">
      <div class="panel-title-row">
        <div>
          <p class="eyebrow">Organizer</p>
          <h2>Publish event</h2>
        </div>
        <span class="mini-badge">new listing</span>
      </div>
      <form id="create-event-form" class="stacked-form">
        <label>
          <span>Title</span>
          <input name="title" placeholder="Rooftop board game night" required />
        </label>
        <div class="form-pair">
          <label>
            <span>City</span>
            <input name="city" placeholder="Sydney" required />
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
            <input name="capacity" type="number" min="1" value="12" required />
          </label>
        </div>
        <label>
          <span>Description</span>
          <textarea name="description" placeholder="What should people expect?"></textarea>
        </label>
        <button type="submit" class="primary-button" ${disabled ? 'disabled' : ''}>${isPending('create') ? 'Publishing' : 'Publish event'}</button>
        ${!state.auth.user ? '<p class="form-hint">Sign in before publishing.</p>' : ''}
      </form>
    </section>
  `;
}

function renderMedia() {
  const selected = state.selectedEvent || '';
  const disabled = !state.auth.user?.id || !selected || isPending('media');
  return `
    <section class="action-panel" id="media-panel">
      <div class="panel-title-row">
        <div>
          <p class="eyebrow">Media</p>
          <h2>Upload intent</h2>
        </div>
        <span class="mini-badge">upload state</span>
      </div>
      <form id="media-form" class="stacked-form">
        <label>
          <span>Event ID</span>
          <input name="eventId" value="${escapeHTML(selected)}" placeholder="Select an event first" required />
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
        <button type="submit" class="secondary-button" ${disabled ? 'disabled' : ''}>${isPending('media') ? 'Creating' : 'Create upload intent'}</button>
        ${state.media ? renderMediaResult() : '<p class="form-hint">Select an event to attach a local media intent.</p>'}
      </form>
    </section>
  `;
}

function renderMediaResult() {
  const asset = state.media.asset || {};
  return `
    <div class="media-result">
      <span class="pill ${toneClass(asset.status)}">${escapeHTML(statusLabel(asset.status))}</span>
      <code>${escapeHTML(asset.id || 'media-id-pending')}</code>
    </div>
  `;
}

function bind() {
  document.querySelector('#auth-form')?.addEventListener('submit', handleAuth);
  document.querySelector('[data-action="logout"]')?.addEventListener('click', handleLogout);
  document.querySelector('#feed-filter')?.addEventListener('submit', handleFeedFilter);
  document.querySelector('#create-event-form')?.addEventListener('submit', handleCreateEvent);
  document.querySelector('#media-form')?.addEventListener('submit', handleCreateUpload);
  document.querySelector('[data-action="join"]')?.addEventListener('click', handleJoin);
  document.querySelector('[data-action="cancel-join"]')?.addEventListener('click', handleCancelJoin);
  document.querySelectorAll('[data-event-id]').forEach((row) => {
    row.addEventListener('click', () => selectEvent(row.dataset.eventId));
  });
  document.querySelectorAll('[data-city]').forEach((button) => {
    button.addEventListener('click', () => setCityFilter(button.dataset.city));
  });
  document.querySelectorAll('[data-action="refresh-feed"]').forEach((button) => {
    button.addEventListener('click', () => refreshFeed());
  });
  document.querySelectorAll('[data-action="focus-create"]').forEach((button) => {
    button.addEventListener('click', () => document.querySelector('#create-panel')?.scrollIntoView({ behavior: scrollBehavior(), block: 'start' }));
  });
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
    setNotice(button.dataset.mode === 'register' ? 'Account created. You can publish and join now.' : 'Signed in. You can publish and join now.', 'good');
    await refreshFeed({ renderAfter: false });
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
  });
}

async function handleFeedFilter(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  state.feed.city = String(form.get('city') || '');
  await refreshFeed();
}

async function setCityFilter(city) {
  state.feed.city = String(city || '');
  await refreshFeed();
}

async function refreshFeed(options = {}) {
  const renderAfter = options.renderAfter !== false;
  state.feed.loading = true;
  state.feed.error = '';
  state.pendingAction = 'feed';
  if (renderAfter) render();
  try {
    const payload = await api.listFeed(state.feed.city);
    state.feed.events = payload.events || [];
    if (renderAfter) setNotice('Feed refreshed. Open an event to verify current join status.', 'neutral');
  } catch (error) {
    state.feed.error = error.message;
    if (renderAfter) setNotice(error.message, 'bad');
  } finally {
    state.feed.loading = false;
    state.pendingAction = '';
    if (renderAfter) render();
  }
}

async function selectEvent(eventID) {
  if (!eventID) return;
  state.selectedEvent = eventID;
  await withPending('detail', async () => {
    state.eventDetail = await api.getEvent(eventID);
    state.media = null;
    setNotice('Event details loaded. Join status is ready to inspect.', 'neutral');
  });
}

async function handleCreateEvent(event) {
  event.preventDefault();
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
    setNotice('Event published. Refresh the feed if it has not appeared yet.', 'good');
    await refreshFeed({ renderAfter: false });
  });
}

async function handleJoin() {
  if (!state.selectedEvent) return;
  await withPending('join', async () => {
    await api.joinEvent(state.selectedEvent);
    state.eventDetail = await api.getEvent(state.selectedEvent);
    setNotice('Join request applied. Status is refreshed in the detail panel.', 'good');
  });
}

async function handleCancelJoin() {
  if (!state.selectedEvent) return;
  await withPending('cancel', async () => {
    await api.cancelJoin(state.selectedEvent);
    state.eventDetail = await api.getEvent(state.selectedEvent);
    setNotice('Join canceled. A waitlisted attendee can move up when a spot opens.', 'neutral');
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
    setNotice('Media upload intent created. The returned state is visible below.', 'good');
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

function isPending(action) {
  return state.pendingAction === action;
}

function setNotice(message, tone = 'neutral') {
  state.notice = message;
  state.noticeTone = tone;
}

function joinStatusHeadline(status, signedIn) {
  if (!signedIn) return 'Sign in to reserve a spot';
  switch (status) {
    case 'CONFIRMED': return 'You have a confirmed spot';
    case 'WAITLISTED': return 'You are on the waitlist';
    case 'CANCELED': return 'Your previous join was canceled';
    default: return 'Spot available if capacity allows';
  }
}

function joinStatusDetail(status, signedIn) {
  if (!signedIn) return 'Sign in so CityEvents can apply join or cancel actions for you.';
  switch (status) {
    case 'CONFIRMED': return 'Cancel if plans change; the next waitlisted attendee can move up.';
    case 'WAITLISTED': return 'If capacity opens, your updated spot appears here.';
    case 'CANCELED': return 'You can join again if the event is still open.';
    default: return 'Join once; the detail panel will refresh your visible status.';
  }
}

function defaultStartAt() {
  const date = new Date(Date.now() + 24 * 60 * 60 * 1000);
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60 * 1000);
  return local.toISOString().slice(0, 16);
}

function monthLabel(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'TBD';
  return date.toLocaleString([], { month: 'short' }).toUpperCase();
}

function dayLabel(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--';
  return date.toLocaleString([], { day: '2-digit' });
}

function timeLabel(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'Time pending';
  return date.toLocaleString([], { hour: '2-digit', minute: '2-digit' });
}

function toneClass(status) {
  return `tone-${statusTone[status] || 'neutral'}`;
}

function scrollBehavior() {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth';
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

render();
refreshFeed();
