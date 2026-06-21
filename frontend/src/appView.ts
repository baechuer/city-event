import type { FormEvent, MouseEvent, SetStateAction } from 'react';
import type { EventDetailPayload, MediaUploadPayload } from './api';
import type { CityEvent } from './domain/events';
import type { Route } from './routing';
import type { AppState, AuthResult, Tone } from './state';

export interface Filters {
  keyword: string;
  city: string;
  category: string;
  date: string;
}

export interface ViewModel {
  auth: AuthResult;
  feed: AppState['feed'];
  filters: Filters;
  selectedEvent: string;
  eventDetail: EventDetailPayload | null;
  media: MediaUploadPayload | null;
  pendingAction: string;
  route: Route;
  catalogEvents(): CityEvent[];
  filteredEvents(overrides?: Partial<Filters>): CityEvent[];
  getEventForDisplay(eventID: string | undefined): CityEvent | null;
  isPending(action: string): boolean;
  navigate(path: string): void;
  onRouteClick(event: MouseEvent<HTMLAnchorElement>): void;
  refreshFeed(renderNotice?: boolean, cityOverride?: string): Promise<void>;
  setFilters(update: SetStateAction<Filters>): void;
  setNotice(message: string, tone?: Tone): void;
  handleSearch(event: FormEvent<HTMLFormElement>): Promise<void>;
  handleAuth(event: FormEvent<HTMLFormElement>): Promise<void>;
  handleLogout(): Promise<void>;
  handleNeedAuth(): void;
  handleCreateEvent(event: FormEvent<HTMLFormElement>): Promise<void>;
  handleAdminRoleUpdate(event: FormEvent<HTMLFormElement>): Promise<void>;
  handleJoin(): Promise<void>;
  handleCancelJoin(): Promise<void>;
  handleCancelRegistration(event: FormEvent<HTMLFormElement>): Promise<void>;
  handleCreateUpload(event: FormEvent<HTMLFormElement>): Promise<void>;
}
