export const defaultConfig: RuntimeConfig = {
  apiBase: '',
  authBase: '',
  eventBase: '',
  feedBase: '',
  mediaBase: '',
};

export const statusTone: Record<string, Tone> = {
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

export type UserRole = 'USER' | 'ORGANIZER' | 'ADMIN';
export type RegistrationStatus = 'NOT_JOINED' | 'CONFIRMED' | 'WAITLISTED' | 'CANCELED';
export type Tone = 'neutral' | 'good' | 'warn' | 'bad';

export interface RuntimeConfig {
  apiBase: string;
  authBase: string;
  eventBase: string;
  feedBase: string;
  mediaBase: string;
}

export interface User {
  id: string;
  email?: string;
  displayName?: string;
  role?: UserRole | string;
}

export interface AuthResult {
  user: User | null;
  accessToken: string;
}

export interface AppState {
  auth: AuthResult;
  feed: { city: string; events: unknown[]; loading: boolean; error: string };
  selectedEvent: string;
  eventDetail: unknown;
  media: unknown;
  notice: string;
  noticeTone: Tone;
  pendingAction: string;
}

export function createInitialState(): AppState {
  return {
    auth: { user: null, accessToken: '' },
    feed: { city: '', events: [], loading: false, error: '' },
    selectedEvent: '',
    eventDetail: null,
    media: null,
    notice: '',
    noticeTone: 'neutral',
    pendingAction: '',
  };
}

export function normalizeAuthResult(result: unknown): AuthResult {
  const value = result as { user?: User | null; accessToken?: string } | null | undefined;
  return {
    user: value?.user ?? null,
    accessToken: value?.accessToken ?? '',
  };
}

export function formatDateTime(value: string | undefined): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleString('en-AU', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function statusLabel(status: string | undefined): string {
  return String(status || 'UNKNOWN').replaceAll('_', ' ');
}

export function canJoin(status: string | undefined): boolean {
  return !status || status === 'NOT_JOINED' || status === 'CANCELED';
}

export function canCancel(status: string | undefined): boolean {
  return status === 'CONFIRMED' || status === 'WAITLISTED';
}

export function canPublish(user: User | null | undefined): boolean {
  return user?.role === 'ORGANIZER' || user?.role === 'ADMIN';
}
