import assert from 'node:assert/strict';
import test from 'node:test';
import { canCancel, canJoin, createInitialState, formatDateTime, normalizeAuthResult, statusLabel } from './state';

test('initial state is empty and stable', () => {
  const state = createInitialState();
  assert.equal(state.auth.user, null);
  assert.deepEqual(state.feed.events, []);
});

test('auth result normalizes missing fields', () => {
  const auth = normalizeAuthResult({});
  assert.equal(auth.user, null);
  assert.equal(auth.accessToken, '');
});

test('status helpers render workflow states', () => {
  assert.equal(statusLabel('NOT_JOINED'), 'NOT JOINED');
  assert.equal(canJoin('NOT_JOINED'), true);
  assert.equal(canJoin('CONFIRMED'), false);
  assert.equal(canCancel('WAITLISTED'), true);
  assert.equal(canCancel('CANCELED'), false);
});

test('date formatter handles invalid input', () => {
  assert.equal(formatDateTime('not-a-date'), '');
});
