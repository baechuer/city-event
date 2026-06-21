import { expect, test } from '@playwright/test';

const apiBase = (process.env.CITYEVENTS_API_BASE || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const adminEmail = process.env.CITYEVENTS_E2E_ADMIN_EMAIL || 'admin@cityevents.local';
const adminPassword = process.env.CITYEVENTS_E2E_ADMIN_PASSWORD || 'AdminPass12345';
const metricsBearerToken = process.env.METRICS_BEARER_TOKEN || '';
const testPassword = 'StrongerPass123';

test('organizer publishes an event and an attendee joins it', async ({ page, request }) => {
  const suffix = `${Date.now()}-${test.info().workerIndex}`;
  const organizerEmail = `organizer-${suffix}@example.com`;
  const attendeeEmail = `attendee-${suffix}@example.com`;
  const eventTitle = `Playwright AI Builders ${suffix}`;

  await page.goto('/');
  await expect(page.getByRole('heading', { name: /Pick a plan/i })).toBeVisible();
  await expect(page.locator('body')).toContainText('CityEvents');
  expect(await page.evaluate(() => window.CITYEVENTS_CONFIG?.apiBase)).toBe(apiBase);

  const organizer = await registerUser(request, organizerEmail, 'Playwright Organizer');
  await registerUser(request, attendeeEmail, 'Playwright Attendee');
  await promoteUserToOrganizer(request, organizer.user.id);

  await loginThroughUI(page, organizerEmail, testPassword);
  await expect(page.getByText('ORGANIZER', { exact: true })).toBeVisible();

  await page.goto('/publish');
  await expect(page.getByRole('heading', { name: /Publish a real event/i })).toBeVisible();
  await page.getByLabel('Title', { exact: true }).fill(eventTitle);
  await page.getByLabel('City', { exact: true }).fill('Sydney');
  await page.getByLabel('Venue', { exact: true }).fill('Playwright Studio');
  await page.getByLabel('Start time', { exact: true }).fill(futureDateTimeLocal());
  await page.getByLabel('Capacity', { exact: true }).fill('3');
  await page.getByLabel('Description', { exact: true }).fill('Browser e2e verifies publish, feed, and RSVP behavior.');

  await Promise.all([
    page.waitForURL(/\/events\/[^/]+$/),
    page.getByRole('button', { name: 'Publish event' }).click(),
  ]);
  await expect(page.getByRole('heading', { name: eventTitle })).toBeVisible();
  await expect(page.getByText('Event published.')).toBeVisible();

  const eventURL = page.url();
  const eventID = eventURL.split('/events/')[1];

  await page.goto('/me');
  await page.getByRole('button', { name: 'Log out' }).click();
  await expect(page.getByRole('heading', { name: /Pick a plan/i })).toBeVisible();

  await loginThroughUI(page, attendeeEmail, testPassword);
  await page.goto(`/events/${eventID}`);
  await expect(page.getByRole('heading', { name: eventTitle })).toBeVisible();
  await page.getByRole('button', { name: 'Join event' }).click();
  await expect(page.getByText('You are going.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Joined' })).toBeVisible();

  const metrics = await request.get(`${apiBase}/metrics`, {
    headers: metricsBearerToken ? { Authorization: `Bearer ${metricsBearerToken}` } : {},
  });
  expect(metrics.ok()).toBeTruthy();
  await expect(metrics.text()).resolves.toContain('cityevents_http_request_duration_seconds_bucket');
});

async function registerUser(request, email, displayName) {
  const response = await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, displayName, password: testPassword },
  });
  return jsonOrThrow(response, `register ${email}`);
}

async function promoteUserToOrganizer(request, userID) {
  const login = await request.post(`${apiBase}/v1/auth/login`, {
    data: { email: adminEmail, password: adminPassword },
  });
  const admin = await jsonOrThrow(login, 'admin login');
  const response = await request.patch(`${apiBase}/v1/auth/users/${encodeURIComponent(userID)}/role`, {
    headers: { Authorization: `Bearer ${admin.accessToken}` },
    data: { role: 'ORGANIZER' },
  });
  await jsonOrThrow(response, 'promote organizer');
}

async function loginThroughUI(page, email, password) {
  await page.goto('/me');
  await page.getByLabel('Email', { exact: true }).fill(email);
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Log in' }).click();
  await expect(page.getByText('Signed in', { exact: true })).toBeVisible();
  await expect(page.getByText(email)).toBeVisible();
}

async function jsonOrThrow(response, label) {
  const text = await response.text();
  if (!response.ok()) {
    throw new Error(`${label} failed with ${response.status()}: ${text}`);
  }
  return text ? JSON.parse(text) : {};
}

function futureDateTimeLocal() {
  const date = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000);
  const pad = (value) => String(value).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}
