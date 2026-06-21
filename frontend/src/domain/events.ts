import type { User } from '../state';

export type EventSource = 'live' | 'demo';

export interface Category {
  slug: string;
  name: string;
  short: string;
  line: string;
  image: string;
  accent: string;
}

export interface CityEvent {
  id: string;
  organizerID: string;
  source: EventSource;
  category: string;
  rank: number;
  title: string;
  description: string;
  city: string;
  venue: string;
  startsAt: string;
  capacity: number;
  confirmedCount: number;
  status: string;
  label: string;
}

export interface RawEvent {
  id?: string;
  eventId?: string;
  organizerId?: string;
  organizerID?: string;
  category?: string;
  title?: string;
  description?: string;
  city?: string;
  venue?: string;
  startsAt?: string;
  capacity?: number;
  confirmedCount?: number;
  status?: string;
  label?: string;
}

interface FeedItem {
  event?: RawEvent;
  confirmedCount?: number;
}

export const quickCities = ['Sydney', 'Melbourne', 'Brisbane'];

export const categories: Category[] = [
  {
    slug: 'networking',
    name: 'Networking',
    short: 'Career rooms',
    line: 'Founder nights, industry mixers, job leads, and serious conversations.',
    image: '/assets/categories/networking.jpg',
    accent: 'coral',
  },
  {
    slug: 'new-friends',
    name: 'Meet New Friends',
    short: 'Low-pressure socials',
    line: 'Brunch, board games, walking groups, and new-in-town tables.',
    image: '/assets/categories/friends.jpg',
    accent: 'teal',
  },
  {
    slug: 'sports-outdoors',
    name: 'Sports',
    short: 'Move together',
    line: 'Pickleball, running, hikes, social leagues, and weekend outdoors.',
    image: '/assets/categories/sports.jpg',
    accent: 'green',
  },
  {
    slug: 'hobbies',
    name: 'Hobbies',
    short: 'Do the thing',
    line: 'Books, photography, games, crafts, anime, cooking, and music circles.',
    image: '/assets/categories/hobbies.jpg',
    accent: 'amber',
  },
  {
    slug: 'learning-tech',
    name: 'Learning & Tech',
    short: 'Build skills',
    line: 'AI workshops, coding nights, study rooms, product and design meetups.',
    image: '/assets/categories/tech.jpg',
    accent: 'blue',
  },
  {
    slug: 'food-nightlife',
    name: 'Food & Nightlife',
    short: 'After-hours plans',
    line: 'Supper clubs, markets, tastings, music bars, and late city plans.',
    image: '/assets/categories/food.jpg',
    accent: 'rose',
  },
];

export const categoryBySlug = new Map(categories.map((category) => [category.slug, category]));

export const demoEvents: CityEvent[] = [
  {
    id: 'demo-sydney-founder-signal',
    source: 'demo',
    category: 'networking',
    rank: 99,
    title: 'Founder Signal Night',
    description: 'A focused room for students, builders, and early founders to trade ideas and meet useful people.',
    city: 'Sydney',
    venue: 'Haymarket Studio',
    startsAt: relativeDate(2, 18, 30),
    capacity: 80,
    confirmedCount: 62,
    status: 'PUBLISHED',
    label: 'Major this week',
    organizerID: '',
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
    organizerID: '',
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
    organizerID: '',
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
    organizerID: '',
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
    organizerID: '',
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
    organizerID: '',
  },
];

export function normalizeEvent(
  item: unknown,
  source: EventSource = 'live',
  rank = 0,
  fallbackCity = 'Sydney',
  confirmedOverride?: number,
): CityEvent {
  const feedItem = (item || {}) as FeedItem;
  const event = (feedItem.event || feedItem || {}) as RawEvent;
  const category = event.category || inferCategory(event);
  const confirmedCount = Number(confirmedOverride ?? feedItem.confirmedCount ?? event.confirmedCount ?? estimateConfirmed(event, rank));
  return {
    id: event.id || event.eventId || '',
    organizerID: event.organizerId || event.organizerID || '',
    source,
    category,
    rank,
    title: event.title || 'Untitled event',
    description: event.description || categoryBySlug.get(category)?.line || 'A local event worth checking out.',
    city: event.city || fallbackCity || 'Sydney',
    venue: event.venue || 'Venue pending',
    startsAt: event.startsAt || relativeDate(2, 18, 0),
    capacity: Number(event.capacity || 24),
    confirmedCount,
    status: event.status || 'PUBLISHED',
    label: event.label || (source === 'live' ? 'Live' : categoryName(category)),
  };
}

export function canManageEvent(event: CityEvent, user: User | null): boolean {
  if (!user?.id) return false;
  if (user.role === 'ADMIN') return true;
  return user.role === 'ORGANIZER' && event.organizerID === user.id;
}

export function asRawEvent(value: unknown): RawEvent | null {
  if (typeof value !== 'object' || value === null) return null;
  return value as RawEvent;
}

export function categoryName(slug: string): string {
  return categoryBySlug.get(slug)?.name || 'Event';
}

export function categoryAccent(event: CityEvent): string {
  return categoryBySlug.get(event.category)?.accent || 'teal';
}

export function spotsLeft(event: CityEvent): number {
  return Math.max(0, Number(event.capacity || 0) - Number(event.confirmedCount || 0));
}

export function spotFill(event: CityEvent, confirmedCount: number): number {
  const capacity = Number(event.capacity || 1);
  return Math.max(6, Math.min(100, Math.round((Number(confirmedCount || 0) / capacity) * 100)));
}

export function rsvpHeadline(event: CityEvent, joinStatus: string, isLive: boolean, user: User | null): string {
  if (!isLive) return 'Preview listing';
  if (!user?.id) return 'Sign in to reserve a spot';
  switch (joinStatus) {
    case 'CONFIRMED': return 'You are going';
    case 'WAITLISTED': return 'You are waitlisted';
    case 'CANCELED': return 'You canceled this RSVP';
    default: return `${spotsLeft(event)} spots left`;
  }
}

export function rsvpDetail(joinStatus: string, isLive: boolean, user: User | null): string {
  if (!isLive) return 'This card shows the discovery experience. Live RSVP is available for events created in CityEvents.';
  if (!user?.id) return 'Browsing stays open. Sign in only when you want to join, publish, or manage media.';
  switch (joinStatus) {
    case 'CONFIRMED': return 'Your spot is confirmed. Cancel if plans change.';
    case 'WAITLISTED': return 'You are in line if another attendee cancels.';
    case 'CANCELED': return 'You can join again if the event is still open.';
    default: return 'Join once and your status updates here.';
  }
}

export function defaultStartAt(): string {
  const date = new Date(Date.now() + 24 * 60 * 60 * 1000);
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60 * 1000);
  return local.toISOString().slice(0, 16);
}

export function formatCompactDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'Time pending';
  return date.toLocaleString('en-AU', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

export function dayLabel(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--';
  return date.toLocaleString('en-AU', { day: '2-digit' });
}

function inferCategory(event: RawEvent): string {
  const text = [event.title, event.description, event.venue].join(' ').toLowerCase();
  if (/founder|career|business|network|startup|professional/.test(text)) return 'networking';
  if (/friend|social|brunch|board|new in town|hangout/.test(text)) return 'new-friends';
  if (/sport|run|hike|pickleball|football|basketball|outdoor|gym/.test(text)) return 'sports-outdoors';
  if (/book|craft|game|photo|anime|cook|music|art/.test(text)) return 'hobbies';
  if (/tech|ai|code|study|workshop|design|product/.test(text)) return 'learning-tech';
  if (/food|dinner|supper|bar|night|market|taste/.test(text)) return 'food-nightlife';
  return 'new-friends';
}

function estimateConfirmed(event: RawEvent, rank: number): number {
  const capacity = Number(event.capacity || 24);
  return Math.max(0, Math.min(capacity - 1, Math.round(capacity * (0.42 + ((rank || 0) % 30) / 100))));
}

function relativeDate(days: number, hour: number, minute: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  date.setHours(hour, minute, 0, 0);
  return date.toISOString();
}
