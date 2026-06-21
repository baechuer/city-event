export interface Route {
  name: 'home' | 'events' | 'eventDetail' | 'categories' | 'category' | 'publish' | 'me';
  path: string;
  eventID?: string;
  slug?: string;
}

export function parseRoute(path: string): Route {
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
