import { createApiClient } from './api.js';
import { canCancel, canJoin, createInitialState, formatDateTime, statusLabel, statusTone } from './state.js';

const api = createApiClient(window.CITYEVENTS_CONFIG || {});
const state = createInitialState();
state.auth = api.loadAuth();

const app = document.querySelector('#app');

function render() {
  app.innerHTML = `
    <aside class="sidebar">
      <div class="brand">CityEvents</div>
      <nav>
        <button class="nav-button active" data-action="refresh-feed">Events</button>
        <button class="nav-button" data-action="focus-create">Create</button>
        <button class="nav-button" data-action="focus-media">Media</button>
      </nav>
      ${renderAuthPanel()}
    </aside>
    <main class="workspace">
      <section class="toolbar">
        <form id="feed-filter" class="inline-form">
          <label>
            <span>City</span>
            <input name="city" value="${escapeHTML(state.feed.city)}" placeholder="Sydney" />
          </label>
          <button type="submit">Refresh</button>
        </form>
        <div class="notice">${escapeHTML(state.notice)}</div>
      </section>
      <section class="grid-layout">
        ${renderFeed()}
        ${renderDetail()}
      </section>
      <section class="form-band" id="create-panel">
        ${renderCreateEvent()}
      </section>
      <section class="form-band" id="media-panel">
        ${renderMedia()}
      </section>
    </main>
  `;
  bind();
}

function renderAuthPanel() {
  if (state.auth.user) {
    return `
      <section class="auth-panel">
        <div class="user-name">${escapeHTML(state.auth.user.displayName || state.auth.user.email)}</div>
        <div class="muted">${escapeHTML(state.auth.user.email || state.auth.user.id)}</div>
        <button data-action="logout">Logout</button>
      </section>
    `;
  }
  return `
    <section class="auth-panel">
      <form id="auth-form">
        <input name="displayName" placeholder="Display name" />
        <input name="email" type="email" placeholder="Email" required />
        <input name="password" type="password" placeholder="Password" required />
        <div class="split">
          <button type="submit" data-mode="login">Login</button>
          <button type="submit" data-mode="register">Register</button>
        </div>
      </form>
    </section>
  `;
}

function renderFeed() {
  const rows = state.feed.events.map((item) => {
    const event = item.event || item;
    return `
      <article class="event-row ${state.selectedEvent === event.id || state.selectedEvent === event.eventId ? 'selected' : ''}" data-event-id="${escapeHTML(event.id || event.eventId)}">
        <div>
          <h3>${escapeHTML(event.title || 'Untitled event')}</h3>
          <div class="muted">${escapeHTML(event.city || '')} · ${escapeHTML(event.venue || '')}</div>
        </div>
        <div class="right">
          <div>${escapeHTML(formatDateTime(event.startsAt))}</div>
          <span class="pill ${toneClass(event.status)}">${escapeHTML(statusLabel(event.status))}</span>
        </div>
      </article>
    `;
  }).join('');
  return `
    <section class="panel feed-panel">
      <div class="panel-header">
        <h2>Event Feed</h2>
        <span>${state.feed.events.length}</span>
      </div>
      ${state.feed.error ? `<div class="error">${escapeHTML(state.feed.error)}</div>` : ''}
      <div class="event-list">${rows || '<div class="empty">No events</div>'}</div>
    </section>
  `;
}

function renderDetail() {
  if (!state.eventDetail) {
    return `
      <section class="panel detail-panel">
        <div class="empty">Select an event</div>
      </section>
    `;
  }
  const detail = state.eventDetail;
  const event = detail.event;
  const joinStatus = detail.viewerJoinStatus || 'NOT_JOINED';
  return `
    <section class="panel detail-panel">
      <div class="panel-header">
        <h2>${escapeHTML(event.title)}</h2>
        <span class="pill ${toneClass(joinStatus)}">${escapeHTML(statusLabel(joinStatus))}</span>
      </div>
      <div class="detail-grid">
        <span>City</span><strong>${escapeHTML(event.city)}</strong>
        <span>Venue</span><strong>${escapeHTML(event.venue)}</strong>
        <span>Starts</span><strong>${escapeHTML(formatDateTime(event.startsAt))}</strong>
        <span>Capacity</span><strong>${detail.confirmedCount} / ${event.capacity}</strong>
      </div>
      <p>${escapeHTML(event.description || '')}</p>
      <div class="actions">
        <button data-action="join" ${canJoin(joinStatus) ? '' : 'disabled'}>Join</button>
        <button data-action="cancel-join" ${canCancel(joinStatus) ? '' : 'disabled'}>Cancel Join</button>
      </div>
    </section>
  `;
}

function renderCreateEvent() {
  return `
    <form id="create-event-form" class="wide-form">
      <h2>Create Event</h2>
      <div class="form-grid">
        <input name="title" placeholder="Title" required />
        <input name="city" placeholder="City" required />
        <input name="venue" placeholder="Venue" required />
        <input name="startsAt" type="datetime-local" required />
        <input name="capacity" type="number" min="1" value="10" required />
        <textarea name="description" placeholder="Description"></textarea>
      </div>
      <button type="submit">Publish</button>
    </form>
  `;
}

function renderMedia() {
  return `
    <form id="media-form" class="wide-form">
      <h2>Media Upload</h2>
      <div class="form-grid">
        <input name="eventId" placeholder="Event ID" value="${escapeHTML(state.selectedEvent || '')}" required />
        <input name="filename" placeholder="Filename" value="banner.jpg" required />
        <select name="contentType">
          <option value="image/jpeg">image/jpeg</option>
          <option value="image/png">image/png</option>
          <option value="image/webp">image/webp</option>
        </select>
        <input name="sizeBytes" type="number" min="1" value="1024" required />
      </div>
      <button type="submit">Create Upload</button>
      ${state.media ? `<div class="media-result"><span class="pill ${toneClass(state.media.asset.status)}">${escapeHTML(statusLabel(state.media.asset.status))}</span><code>${escapeHTML(state.media.asset.id)}</code></div>` : ''}
    </form>
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
  document.querySelectorAll('.event-row').forEach((row) => {
    row.addEventListener('click', () => selectEvent(row.dataset.eventId));
  });
  document.querySelector('[data-action="refresh-feed"]')?.addEventListener('click', refreshFeed);
  document.querySelector('[data-action="focus-create"]')?.addEventListener('click', () => document.querySelector('#create-panel')?.scrollIntoView());
  document.querySelector('[data-action="focus-media"]')?.addEventListener('click', () => document.querySelector('#media-panel')?.scrollIntoView());
}

async function handleAuth(event) {
  event.preventDefault();
  const button = event.submitter;
  const form = new FormData(event.currentTarget);
  const data = Object.fromEntries(form.entries());
  try {
    state.auth = button.dataset.mode === 'register'
      ? await api.register(data)
      : await api.login({ email: data.email, password: data.password });
    state.notice = 'Signed in';
    await refreshFeed();
  } catch (error) {
    state.notice = error.message;
    render();
  }
}

async function handleLogout() {
  try {
    await api.logout();
  } catch {
    api.clearAuth();
  }
  state.auth = api.loadAuth();
  state.eventDetail = null;
  state.notice = 'Signed out';
  render();
}

async function handleFeedFilter(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  state.feed.city = String(form.get('city') || '');
  await refreshFeed();
}

async function refreshFeed() {
  state.feed.loading = true;
  state.feed.error = '';
  try {
    const payload = await api.listFeed(state.feed.city);
    state.feed.events = payload.events || [];
  } catch (error) {
    state.feed.error = error.message;
  } finally {
    state.feed.loading = false;
    render();
  }
}

async function selectEvent(eventID) {
  if (!eventID) return;
  state.selectedEvent = eventID;
  try {
    state.eventDetail = await api.getEvent(eventID);
    state.notice = '';
  } catch (error) {
    state.notice = error.message;
  }
  render();
}

async function handleCreateEvent(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  const startsAt = new Date(String(form.get('startsAt'))).toISOString();
  try {
    const detail = await api.createEvent({
      title: String(form.get('title') || ''),
      description: String(form.get('description') || ''),
      city: String(form.get('city') || ''),
      venue: String(form.get('venue') || ''),
      startsAt,
      capacity: Number(form.get('capacity') || 1),
    });
    state.selectedEvent = detail.event.id;
    state.eventDetail = detail;
    state.notice = 'Event created';
    await refreshFeed();
  } catch (error) {
    state.notice = error.message;
    render();
  }
}

async function handleJoin() {
  if (!state.selectedEvent) return;
  try {
    await api.joinEvent(state.selectedEvent);
    await selectEvent(state.selectedEvent);
    state.notice = 'Join updated';
  } catch (error) {
    state.notice = error.message;
    render();
  }
}

async function handleCancelJoin() {
  if (!state.selectedEvent) return;
  try {
    await api.cancelJoin(state.selectedEvent);
    await selectEvent(state.selectedEvent);
    state.notice = 'Join canceled';
  } catch (error) {
    state.notice = error.message;
    render();
  }
}

async function handleCreateUpload(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  try {
    state.media = await api.createUpload({
      eventId: String(form.get('eventId') || ''),
      filename: String(form.get('filename') || ''),
      contentType: String(form.get('contentType') || ''),
      sizeBytes: Number(form.get('sizeBytes') || 0),
    });
    state.notice = 'Upload created';
  } catch (error) {
    state.notice = error.message;
  }
  render();
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

render();
refreshFeed();
