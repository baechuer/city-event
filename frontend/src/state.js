export const defaultConfig = {
  authBase: 'http://127.0.0.1:8081',
  eventBase: 'http://127.0.0.1:8082',
  feedBase: 'http://127.0.0.1:8083',
  mediaBase: 'http://127.0.0.1:8085',
};

export const statusTone = {
  NOT_JOINED: 'neutral',
  CONFIRMED: 'good',
  WAITLISTED: 'warn',
  CANCELED: 'bad',
  PUBLISHED: 'good',
  UPLOADING: 'warn',
  UPLOADED: 'warn',
  PROCESSING: 'warn',
  READY: 'good',
  FAILED: 'bad',
};

export function createInitialState() {
  return {
    auth: { user: null, accessToken: '' },
    feed: { city: '', events: [], loading: false, error: '' },
    selectedEvent: null,
    eventDetail: null,
    media: null,
    notice: '',
    noticeTone: 'neutral',
    pendingAction: '',
  };
}

export function normalizeAuthResult(result) {
  return {
    user: result?.user ?? null,
    accessToken: result?.accessToken ?? '',
  };
}

export function formatDateTime(value) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleString([], {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function statusLabel(status) {
  return String(status || 'UNKNOWN').replaceAll('_', ' ');
}

export function canJoin(status) {
  return !status || status === 'NOT_JOINED' || status === 'CANCELED';
}

export function canCancel(status) {
  return status === 'CONFIRMED' || status === 'WAITLISTED';
}
