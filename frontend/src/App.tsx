import { useCallback, useEffect, useMemo, useState, type CSSProperties, type FormEvent, type MouseEvent, type ReactNode } from 'react';
import { createApiClient, type EventDetailPayload, type MediaUploadPayload } from './api';
import { canCancel, canJoin, canPublish, createInitialState, formatDateTime, statusLabel, statusTone, type AuthResult, type Tone, type User } from './state';

type EventSource = 'live' | 'demo';

interface Category {
  slug: string;
  name: string;
  short: string;
  line: string;
  image: string;
  accent: string;
}

interface CityEvent {
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

interface RawEvent {
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

interface Filters {
  keyword: string;
  city: string;
  category: string;
  date: string;
}

interface Route {
  name: 'home' | 'events' | 'eventDetail' | 'categories' | 'category' | 'publish' | 'me';
  path: string;
  eventID?: string;
  slug?: string;
}

const api = createApiClient(window.CITYEVENTS_CONFIG || {});
const quickCities = ['Sydney', 'Melbourne', 'Brisbane'];

const categories: Category[] = [
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

const categoryBySlug = new Map(categories.map((category) => [category.slug, category]));

const demoEvents: CityEvent[] = [
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

declare global {
  interface Window {
    CITYEVENTS_CONFIG?: Record<string, string>;
  }
}

export function App() {
  const initialState = useMemo(() => createInitialState(), []);
  const [path, setPath] = useState(window.location.pathname || '/');
  const [auth, setAuth] = useState<AuthResult>(initialState.auth);
  const [feed, setFeed] = useState(initialState.feed);
  const [filters, setFilters] = useState<Filters>({ keyword: '', city: 'Sydney', category: '', date: 'any' });
  const [selectedEvent, setSelectedEvent] = useState('');
  const [eventDetail, setEventDetail] = useState<EventDetailPayload | null>(null);
  const [media, setMedia] = useState<MediaUploadPayload | null>(null);
  const [notice, setNoticeText] = useState('');
  const [noticeTone, setNoticeTone] = useState<Tone>('neutral');
  const [pendingAction, setPendingAction] = useState('');
  const [returnTo, setReturnTo] = useState('');

  const route = parseRoute(path);

  const setNotice = useCallback((message: string, tone: Tone = 'neutral') => {
    setNoticeText(message);
    setNoticeTone(tone);
  }, []);

  const navigate = useCallback((nextPath: string) => {
    if (window.location.pathname !== nextPath) {
      window.history.pushState({}, '', nextPath);
    }
    setPath(nextPath);
    window.scrollTo({ top: 0, behavior: 'auto' });
  }, []);

  const onRouteClick = useCallback((event: MouseEvent<HTMLAnchorElement>) => {
    if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    const href = event.currentTarget.getAttribute('href');
    if (!href || href.startsWith('http') || href.startsWith('mailto:')) return;
    event.preventDefault();
    navigate(href);
  }, [navigate]);

  const catalogEvents = useCallback((): CityEvent[] => {
    const live = (feed.events || []).map((item, index) => normalizeEvent(item, 'live', 120 - index, filters.city));
    const demos = demoEvents.map((item) => normalizeEvent(item, 'demo', item.rank, filters.city));
    const seen = new Set<string>();
    return [...live, ...demos]
      .filter((event) => {
        if (seen.has(event.id)) return false;
        seen.add(event.id);
        return true;
      })
      .sort((a, b) => Number(b.rank || 0) - Number(a.rank || 0));
  }, [feed.events, filters.city]);

  const getEventForDisplay = useCallback((eventID: string | undefined): CityEvent | null => {
    if (!eventID) return null;
    const detailEvent = asRawEvent(eventDetail?.event);
    if (detailEvent?.id === eventID) {
      return normalizeEvent(eventDetail?.event, 'live', 130, filters.city, eventDetail?.confirmedCount);
    }
    return catalogEvents().find((event) => event.id === eventID) || null;
  }, [catalogEvents, eventDetail, filters.city]);

  const filteredEvents = useCallback((overrides: Partial<Filters> = {}) => {
    const activeFilters = { ...filters, ...overrides };
    const keyword = activeFilters.keyword.trim().toLowerCase();
    return catalogEvents().filter((event) => {
      if (activeFilters.city && event.city !== activeFilters.city) return false;
      if (activeFilters.category && event.category !== activeFilters.category) return false;
      if (!keyword) return true;
      return [event.title, event.description, event.venue, event.city, categoryName(event.category)]
        .join(' ')
        .toLowerCase()
        .includes(keyword);
    });
  }, [catalogEvents, filters]);

  const isPending = useCallback((action: string) => pendingAction === action, [pendingAction]);

  const runPending = useCallback(async (action: string, fn: () => Promise<void>) => {
    setPendingAction(action);
    try {
      await fn();
    } catch (error) {
      setNotice(errorMessage(error), 'bad');
    } finally {
      setPendingAction('');
    }
  }, [setNotice]);

  const refreshFeed = useCallback(async (renderNotice = true, cityOverride?: string) => {
    const city = cityOverride ?? (feed.city || filters.city || '');
    setFeed((current) => ({ ...current, loading: true, error: '' }));
    setPendingAction('feed');
    try {
      const payload = await api.listFeed(city);
      setFeed({ city, events: payload.events || [], loading: false, error: '' });
      if (renderNotice) setNotice('Live feed refreshed.', 'neutral');
    } catch (error) {
      const message = errorMessage(error);
      setFeed((current) => ({ ...current, loading: false, error: message }));
      if (renderNotice) setNotice(message, 'bad');
    } finally {
      setPendingAction('');
    }
  }, [feed.city, filters.city, setNotice]);

  const loadEventDetail = useCallback(async (eventID: string) => {
    setSelectedEvent(eventID);
    setPendingAction('detail');
    try {
      const detail = await api.getEvent(eventID);
      setEventDetail(detail);
      setMedia(null);
      setNotice('Event status loaded.', 'neutral');
    } catch (error) {
      setNotice(errorMessage(error), 'bad');
    } finally {
      setPendingAction('');
    }
  }, [setNotice]);

  useEffect(() => {
    const onPopState = () => setPath(window.location.pathname || '/');
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  useEffect(() => {
    let active = true;
    async function bootstrap() {
      try {
        const refreshed = await api.refresh();
        if (active) setAuth(refreshed);
      } catch {
        api.clearAuth();
        if (active) setAuth(api.loadAuth());
      }
      if (active) await refreshFeed(false, 'Sydney');
    }
    void bootstrap();
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (route.name !== 'eventDetail' || !route.eventID) return;
    setSelectedEvent(route.eventID);
    const event = getEventForDisplay(route.eventID);
    if (event?.source === 'demo') {
      setEventDetail(null);
      setMedia(null);
      return;
    }
    if (eventDetail?.event && asRawEvent(eventDetail.event)?.id === route.eventID) return;
    if (pendingAction === 'detail') return;
    void loadEventDetail(route.eventID);
  }, [route.name, route.eventID, getEventForDisplay, eventDetail, pendingAction, loadEventDetail]);

  const handleSearch = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const nextFilters = {
      keyword: String(form.get('keyword') || '').trim(),
      city: String(form.get('city') || filters.city || 'Sydney'),
      category: String(form.get('category') || ''),
      date: 'any',
    };
    setFilters(nextFilters);
    await refreshFeed(false, nextFilters.city);
    navigate('/events');
  };

  const handleAuth = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const submitter = (event.nativeEvent as SubmitEvent).submitter as HTMLButtonElement | null;
    const form = new FormData(event.currentTarget);
    const data = Object.fromEntries(form.entries());
    await runPending('auth', async () => {
      const nextAuth = submitter?.dataset.mode === 'register'
        ? await api.register(data)
        : await api.login({ email: data.email || '', password: data.password || '' });
      setAuth(nextAuth);
      setNotice(submitter?.dataset.mode === 'register' ? 'Account created.' : 'Signed in.', 'good');
      await refreshFeed(false, filters.city);
      if (returnTo) {
        const destination = returnTo;
        setReturnTo('');
        navigate(destination);
      }
    });
  };

  const handleLogout = async () => {
    await runPending('auth', async () => {
      try {
        await api.logout();
      } catch {
        api.clearAuth();
      }
      setAuth(api.loadAuth());
      setEventDetail(null);
      setMedia(null);
      setNotice('Signed out.', 'neutral');
      navigate('/');
    });
  };

  const handleNeedAuth = () => {
    setReturnTo(window.location.pathname);
    setNotice('Sign in to reserve a spot.', 'neutral');
    navigate('/me');
  };

  const handleCreateEvent = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!auth.user?.id) {
      setReturnTo('/publish');
      setNotice('Sign in before publishing.', 'neutral');
      navigate('/me');
      return;
    }
    if (!canPublish(auth.user)) {
      setNotice('Organizer role required before publishing.', 'bad');
      return;
    }
    const form = new FormData(event.currentTarget);
    await runPending('create', async () => {
      const detail = await api.createEvent({
        title: String(form.get('title') || ''),
        description: String(form.get('description') || ''),
        city: String(form.get('city') || ''),
        venue: String(form.get('venue') || ''),
        startsAt: new Date(String(form.get('startsAt'))).toISOString(),
        capacity: Number(form.get('capacity') || 1),
      });
      const created = asRawEvent(detail.event);
      if (!created?.id) throw new Error('Event was created without an id.');
      setSelectedEvent(created.id);
      setEventDetail(detail);
      const city = created.city || filters.city;
      setFilters((current) => ({ ...current, city }));
      setFeed((current) => ({ ...current, city }));
      setNotice('Event published.', 'good');
      await refreshFeed(false, city);
      navigate(`/events/${created.id}`);
    });
  };

  const handleAdminRoleUpdate = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await runPending('role', async () => {
      await api.updateUserRole(String(form.get('userId') || ''), String(form.get('role') || 'USER'));
      setNotice('Role updated.', 'good');
    });
  };

  const handleJoin = async () => {
    if (!selectedEvent) return;
    if (!auth.user?.id) {
      handleNeedAuth();
      return;
    }
    const event = getEventForDisplay(selectedEvent);
    if (!event || event.source === 'demo') {
      setNotice('Preview events cannot be joined. Publish or open a live event.', 'neutral');
      return;
    }
    await runPending('join', async () => {
      await api.joinEvent(selectedEvent);
      setEventDetail(await api.getEvent(selectedEvent));
      setNotice('You are going.', 'good');
    });
  };

  const handleCancelJoin = async () => {
    if (!selectedEvent) return;
    await runPending('cancel', async () => {
      await api.cancelJoin(selectedEvent);
      setEventDetail(await api.getEvent(selectedEvent));
      setNotice('RSVP canceled.', 'neutral');
    });
  };

  const handleCancelRegistration = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selectedEvent) return;
    const form = new FormData(event.currentTarget);
    await runPending('moderation', async () => {
      await api.cancelRegistration(selectedEvent, String(form.get('userId') || ''));
      setEventDetail(await api.getEvent(selectedEvent));
      setNotice('Attendee RSVP canceled.', 'good');
    });
  };

  const handleCreateUpload = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await runPending('media', async () => {
      setMedia(await api.createUpload({
        eventId: String(form.get('eventId') || ''),
        filename: String(form.get('filename') || ''),
        contentType: String(form.get('contentType') || ''),
        sizeBytes: Number(form.get('sizeBytes') || 0),
      }));
      setNotice('Media upload intent created.', 'good');
    });
  };

  const view = {
    auth,
    feed,
    filters,
    selectedEvent,
    eventDetail,
    media,
    pendingAction,
    route,
    catalogEvents,
    filteredEvents,
    getEventForDisplay,
    isPending,
    navigate,
    onRouteClick,
    refreshFeed,
    setFilters,
    setNotice,
    handleSearch,
    handleAuth,
    handleLogout,
    handleNeedAuth,
    handleCreateEvent,
    handleAdminRoleUpdate,
    handleJoin,
    handleCancelJoin,
    handleCancelRegistration,
    handleCreateUpload,
  };

  return (
    <>
      <a className="skip-link" href="#main-content">Skip to main content</a>
      <Topbar route={route} auth={auth} onRouteClick={onRouteClick} />
      <main id="main-content" className={`page-shell page-${route.name}`}>
        <Page view={view} />
      </main>
      {notice ? <Notice message={notice} tone={noticeTone} /> : null}
    </>
  );
}

interface ViewModel {
  auth: AuthResult;
  feed: ReturnType<typeof createInitialState>['feed'];
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
  setFilters(update: Filters | ((current: Filters) => Filters)): void;
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

function Topbar({ route, auth, onRouteClick }: { route: Route; auth: AuthResult; onRouteClick: (event: MouseEvent<HTMLAnchorElement>) => void }) {
  return (
    <header className="topbar">
      <RouteLink className="brand-lockup" href="/" onRouteClick={onRouteClick}>
        <span className="brand-mark" aria-hidden="true">CE</span>
        <span>
          <span className="brand-name">CityEvents</span>
          <span className="brand-subtitle">Nearby plans, clear spots</span>
        </span>
      </RouteLink>
      <nav className="site-nav" aria-label="Main">
        <NavLink label="Discover" href="/" route={route} onRouteClick={onRouteClick} />
        <NavLink label="Events" href="/events" route={route} onRouteClick={onRouteClick} />
        <NavLink label="Categories" href="/categories" route={route} onRouteClick={onRouteClick} />
        <NavLink label="Publish" href="/publish" route={route} onRouteClick={onRouteClick} />
        <NavLink label={auth.user ? 'Me' : 'Sign in'} href="/me" route={route} onRouteClick={onRouteClick} />
      </nav>
    </header>
  );
}

function NavLink({ label, href, route, onRouteClick }: { label: string; href: string; route: Route; onRouteClick: (event: MouseEvent<HTMLAnchorElement>) => void }) {
  const active = href === '/' ? route.name === 'home' : route.path === href || route.path.startsWith(`${href}/`);
  return (
    <RouteLink className={active ? 'active' : ''} href={href} onRouteClick={onRouteClick}>
      {label}
    </RouteLink>
  );
}

function Page({ view }: { view: ViewModel }) {
  if (view.route.name === 'eventDetail') return <EventDetailPage view={view} eventID={view.route.eventID || ''} />;
  if (view.route.name === 'events') return <EventsPage view={view} />;
  if (view.route.name === 'categories') return <CategoriesPage view={view} />;
  if (view.route.name === 'category') return <CategoryPage view={view} slug={view.route.slug || ''} />;
  if (view.route.name === 'publish') return <PublishPage view={view} />;
  if (view.route.name === 'me') return <AccountPage view={view} />;
  return <HomePage view={view} />;
}

function HomePage({ view }: { view: ViewModel }) {
  const events = view.catalogEvents();
  const featureEvents = demoEvents.map((item) => normalizeEvent(item, 'demo', item.rank, view.filters.city));
  const spotlight = featureEvents[0];
  const secondary = featureEvents.slice(1, 4);
  const cityEvents = events.filter((event) => event.city === view.filters.city).slice(0, 4);

  return (
    <section className="home-board" aria-label="CityEvents discovery">
      <div className="hero-panel">
        <div className="hero-copy">
          <p className="eyebrow">Tonight&apos;s picks</p>
          <h1>Pick a plan before the night disappears.</h1>
          <p>Fast discovery for social events, useful rooms, active weekends, and low-pressure ways to meet people.</p>
        </div>
        <SearchForm id="hero-search" placeholder="Find events, categories, or venues" view={view} />
      </div>

      <section className={`spotlight-card ${categoryAccent(spotlight)}`} aria-labelledby="spotlight-title">
        <EventImage event={spotlight} className="spotlight-image" />
        <div className="spotlight-copy">
          <span className="label-pill">{spotlight.label || 'Featured'}</span>
          <h2 id="spotlight-title">{spotlight.title}</h2>
          <p>{spotlight.description}</p>
          <div className="event-facts">
            <span>{formatCompactDate(spotlight.startsAt)}</span>
            <span>{spotlight.venue}</span>
            <span>{spotsLeft(spotlight)} spots left</span>
          </div>
          <RouteLink className="primary-button" href={`/events/${encodeURIComponent(spotlight.id)}`} onRouteClick={view.onRouteClick}>View event</RouteLink>
        </div>
      </section>

      <aside className="signal-stack" aria-label="Popular events">
        <div className="section-heading compact">
          <span className="eyebrow">Popular now</span>
          <RouteLink href="/events" onRouteClick={view.onRouteClick}>See all</RouteLink>
        </div>
        {secondary.map((event) => <SignalEvent key={event.id} event={event} view={view} />)}
      </aside>

      <section className="category-strip" aria-label="Explore by category">
        {categories.map((category) => <CategoryTile key={category.slug} category={category} view={view} />)}
      </section>

      <section className="city-strip" aria-label="Upcoming near you">
        <div className="section-heading compact">
          <span><strong>{view.filters.city}</strong> upcoming</span>
          <button type="button" className="text-button" onClick={() => void view.refreshFeed()}>{view.isPending('feed') ? 'Refreshing' : 'Refresh live feed'}</button>
        </div>
        <div className="city-event-row">
          {(cityEvents.length ? cityEvents : demoEvents.slice(0, 4)).map((event) => <MiniEvent key={event.id} event={event} view={view} />)}
        </div>
      </section>
    </section>
  );
}

function EventsPage({ view }: { view: ViewModel }) {
  const events = view.filteredEvents();
  return (
    <section className="browse-layout">
      <div className="browse-command">
        <p className="eyebrow">Browse events</p>
        <h1>Scan the city by category, date, and spots left.</h1>
        <SearchForm id="browse-filter" placeholder="Search events" view={view} />
        <FilterChips view={view} />
      </div>
      <div className="browse-results">
        <div className="section-heading">
          <span><strong>{events.length}</strong> results</span>
          <button type="button" className="secondary-button" onClick={() => void view.refreshFeed()}>{view.isPending('feed') ? 'Refreshing' : 'Refresh live feed'}</button>
        </div>
        <div className="event-grid">
          {events.length ? events.map((event) => <EventCard key={event.id} event={event} view={view} />) : <EmptyEvents view={view} />}
        </div>
      </div>
    </section>
  );
}

function CategoriesPage({ view }: { view: ViewModel }) {
  return (
    <section className="category-page">
      <div className="page-intro">
        <p className="eyebrow">Category lanes</p>
        <h1>Choose the reason you want to leave the house.</h1>
        <p>Each lane starts with strong visual cues and focused event cards so the user does not need to hunt.</p>
      </div>
      <div className="category-grid">
        {categories.map((category) => <CategoryFeature key={category.slug} category={category} view={view} />)}
      </div>
    </section>
  );
}

function CategoryPage({ view, slug }: { view: ViewModel; slug: string }) {
  const category = categoryBySlug.get(slug) || categories[0];
  const events = view.filteredEvents({ category: category.slug });
  return (
    <section className="category-detail">
      <div className={`category-hero ${category.accent}`}>
        <img src={category.image} alt="" />
        <div>
          <p className="eyebrow">Category</p>
          <h1>{category.name}</h1>
          <p>{category.line}</p>
          <RouteLink className="secondary-button" href="/categories" onRouteClick={view.onRouteClick}>All categories</RouteLink>
        </div>
      </div>
      <div className="event-grid">
        {events.length ? events.map((event) => <EventCard key={event.id} event={event} view={view} />) : <EmptyEvents view={view} categoryNameValue={category.name} />}
      </div>
    </section>
  );
}

function EventDetailPage({ view, eventID }: { view: ViewModel; eventID: string }) {
  const event = view.getEventForDisplay(eventID);
  if (!event) {
    if (view.isPending('detail')) {
      return <section className="detail-page"><StateMessage title="Loading event" detail="Fetching the live event details." /></section>;
    }
    return (
      <section className="detail-page">
        <StateMessage title="Event not found" detail="Go back to discovery and choose another event.">
          <RouteLink className="primary-button" href="/events" onRouteClick={view.onRouteClick}>Browse events</RouteLink>
        </StateMessage>
      </section>
    );
  }

  const isLive = event.source !== 'demo';
  const detail = isLive && asRawEvent(view.eventDetail?.event)?.id === event.id ? view.eventDetail : null;
  const joinStatus = detail?.viewerJoinStatus || 'NOT_JOINED';
  const confirmedCount = Number(detail?.confirmedCount ?? event.confirmedCount ?? 0);
  const joined = joinStatus === 'CONFIRMED' || joinStatus === 'WAITLISTED';
  const joinDisabled = isLive && view.auth.user?.id ? !canJoin(joinStatus) || view.isPending('join') : false;
  const cancelDisabled = !isLive || !view.auth.user?.id || !canCancel(joinStatus) || view.isPending('cancel');

  return (
    <section className="detail-page">
      <article className={`event-detail-card ${categoryAccent(event)}`}>
        <EventImage event={event} className="detail-image" />
        <div className="detail-copy">
          <div className="detail-topline">
            <span className="label-pill">{categoryName(event.category)}</span>
            <span className="label-pill soft">{isLive ? 'Live RSVP' : 'Preview'}</span>
          </div>
          <h1>{event.title}</h1>
          <p>{event.description || 'No description provided yet.'}</p>
          <dl className="fact-grid">
            <div><dt>When</dt><dd>{formatDateTime(event.startsAt)}</dd></div>
            <div><dt>Where</dt><dd>{event.city} / {event.venue}</dd></div>
            <div><dt>Capacity</dt><dd>{confirmedCount} / {event.capacity || '-'}</dd></div>
          </dl>
        </div>
      </article>

      <aside className="rsvp-panel">
        <div>
          <p className="eyebrow">Your spot</p>
          <h2>{rsvpHeadline(event, joinStatus, isLive, view.auth.user)}</h2>
          <p>{rsvpDetail(joinStatus, isLive, view.auth.user)}</p>
        </div>
        <div className="spot-meter" aria-label="Spots left">
          <span style={{ '--fill': `${spotFill(event, confirmedCount)}%` } as CSSProperties}></span>
        </div>
        <div className="rsvp-actions">
          <button
            type="button"
            className="primary-button"
            onClick={isLive && view.auth.user?.id ? view.handleJoin : isLive ? view.handleNeedAuth : () => view.setNotice('Preview events show the discovery experience. Publish a live event to test RSVP.', 'neutral')}
            disabled={joinDisabled}
          >
            {view.isPending('join') ? 'Joining' : isLive ? view.auth.user?.id ? joined ? 'Joined' : 'Join event' : 'Sign in to join' : 'Preview only'}
          </button>
          <button type="button" className="secondary-button" onClick={() => void view.handleCancelJoin()} disabled={cancelDisabled}>
            {view.isPending('cancel') ? 'Canceling' : 'Cancel RSVP'}
          </button>
        </div>
        <RegistrationModerationPanel event={event} isLive={isLive} view={view} />
        <RouteLink className="text-button" href="/events" onRouteClick={view.onRouteClick}>Back to events</RouteLink>
      </aside>
    </section>
  );
}

function PublishPage({ view }: { view: ViewModel }) {
  const signedIn = Boolean(view.auth.user?.id);
  const allowed = canPublish(view.auth.user);
  return (
    <section className="publish-layout">
      <div className="page-intro">
        <p className="eyebrow">Organizer</p>
        <h1>Publish a real event into the live feed.</h1>
        <p>Keep creation focused: title, place, time, capacity, and a short reason to show up.</p>
      </div>
      <div className="publish-card">
        {signedIn ? allowed ? <CreateEventForm view={view} /> : <RoleBlockedPanel user={view.auth.user} view={view} /> : <AuthPanel title="Sign in to publish" detail="Create your account first, then the publish form unlocks." view={view} />}
      </div>
    </section>
  );
}

function AccountPage({ view }: { view: ViewModel }) {
  const user = view.auth.user;
  if (!user) {
    return (
      <section className="account-layout">
        <div className="page-intro">
          <p className="eyebrow">Account</p>
          <h1>Sign in only when you are ready to act.</h1>
          <p>Visitors can browse first. Joining, canceling, publishing, and media upload intents require identity.</p>
        </div>
        <AuthPanel title="Enter CityEvents" detail="Use a 12+ character password with mixed case and a number." view={view} />
      </section>
    );
  }

  return (
    <section className="account-layout signed-in">
      <div className="profile-card">
        <p className="eyebrow">Signed in</p>
        <h1>{user.displayName || 'CityEvents user'}</h1>
        <p>{user.email || user.id}</p>
        <span className="label-pill soft">{user.role || 'USER'}</span>
        <button type="button" className="secondary-button" onClick={() => void view.handleLogout()}>Log out</button>
      </div>
      <div className="account-actions">
        <section className="compact-panel">
          <div className="section-heading">
            <span><strong>Selected event</strong></span>
            <RouteLink href="/events" onRouteClick={view.onRouteClick}>Choose event</RouteLink>
          </div>
          <SelectedEventSummary view={view} />
        </section>
        {user.role === 'ADMIN' ? <AdminRolePanel view={view} /> : null}
        <MediaPanel view={view} />
      </div>
    </section>
  );
}

function SearchForm({ id, placeholder, view }: { id: string; placeholder: string; view: ViewModel }) {
  return (
    <form id={id} className="search-form" onSubmit={(event) => void view.handleSearch(event)}>
      <label>
        <span>Search</span>
        <input name="keyword" defaultValue={view.filters.keyword} placeholder={placeholder} />
      </label>
      <label>
        <span>City</span>
        <select name="city" defaultValue={view.filters.city}>
          {quickCities.map((city) => <option key={city} value={city}>{city}</option>)}
        </select>
      </label>
      <label>
        <span>Category</span>
        <select name="category" defaultValue={view.filters.category}>
          <option value="">All</option>
          {categories.map((category) => <option key={category.slug} value={category.slug}>{category.name}</option>)}
        </select>
      </label>
      <button type="submit" className="primary-button">Explore</button>
    </form>
  );
}

function FilterChips({ view }: { view: ViewModel }) {
  return (
    <div className="filter-chips" aria-label="Quick filters">
      {categories.slice(0, 5).map((category) => (
        <button
          key={category.slug}
          type="button"
          className={`chip-button ${view.filters.category === category.slug ? 'selected' : ''}`}
          onClick={() => {
            view.setFilters((current) => ({ ...current, category: category.slug }));
            view.navigate('/events');
          }}
        >
          {category.name}
        </button>
      ))}
      <button
        type="button"
        className={`chip-button ${view.filters.category === '' ? 'selected' : ''}`}
        onClick={() => {
          view.setFilters((current) => ({ ...current, category: '' }));
          view.navigate('/events');
        }}
      >
        All
      </button>
    </div>
  );
}

function SignalEvent({ event, view }: { event: CityEvent; view: ViewModel }) {
  return (
    <RouteLink className="signal-event" href={`/events/${encodeURIComponent(event.id)}`} onRouteClick={view.onRouteClick}>
      <span className="signal-date">{dayLabel(event.startsAt)}</span>
      <span>
        <strong>{event.title}</strong>
        <small>{categoryName(event.category)} / {event.city}</small>
      </span>
      <em>{spotsLeft(event)}</em>
    </RouteLink>
  );
}

function CategoryTile({ category, view }: { category: Category; view: ViewModel }) {
  return (
    <RouteLink className={`category-tile ${category.accent}`} href={`/categories/${category.slug}`} onRouteClick={view.onRouteClick}>
      <img src={category.image} alt="" />
      <span>
        <strong>{category.name}</strong>
        <small>{category.short}</small>
      </span>
    </RouteLink>
  );
}

function CategoryFeature({ category, view }: { category: Category; view: ViewModel }) {
  const count = view.catalogEvents().filter((event) => event.category === category.slug).length;
  return (
    <RouteLink className={`category-feature ${category.accent}`} href={`/categories/${category.slug}`} onRouteClick={view.onRouteClick}>
      <img src={category.image} alt="" />
      <span className="label-pill">{count} events</span>
      <h2>{category.name}</h2>
      <p>{category.line}</p>
    </RouteLink>
  );
}

function MiniEvent({ event, view }: { event: CityEvent; view: ViewModel }) {
  return (
    <RouteLink className="mini-event" href={`/events/${encodeURIComponent(event.id)}`} onRouteClick={view.onRouteClick}>
      <strong>{event.title}</strong>
      <span>{formatCompactDate(event.startsAt)} / {spotsLeft(event)} spots</span>
    </RouteLink>
  );
}

function EventCard({ event, view }: { event: CityEvent; view: ViewModel }) {
  return (
    <RouteLink className={`event-card ${categoryAccent(event)}`} href={`/events/${encodeURIComponent(event.id)}`} onRouteClick={view.onRouteClick}>
      <EventImage event={event} className="card-image" />
      <span className="label-pill">{event.label || categoryName(event.category)}</span>
      <h2>{event.title}</h2>
      <p>{event.description}</p>
      <div className="event-card-meta">
        <span>{formatCompactDate(event.startsAt)}</span>
        <span>{event.city}</span>
        <span>{spotsLeft(event)} spots</span>
      </div>
    </RouteLink>
  );
}

function EventImage({ event, className }: { event: CityEvent; className: string }) {
  const category = categoryBySlug.get(event.category) || categories[0];
  return <img className={className} src={category.image} alt="" />;
}

function EmptyEvents({ view, categoryNameValue = 'this lane' }: { view: ViewModel; categoryNameValue?: string }) {
  return (
    <StateMessage title={`No live events in ${categoryNameValue} yet`} detail="Publish one to test the live RSVP and feed projection flow.">
      <RouteLink className="primary-button" href="/publish" onRouteClick={view.onRouteClick}>Publish event</RouteLink>
    </StateMessage>
  );
}

function AuthPanel({ title, detail, view }: { title: string; detail: string; view: ViewModel }) {
  return (
    <section className="auth-card">
      <div>
        <p className="eyebrow">Account</p>
        <h2>{title}</h2>
        <p>{detail}</p>
      </div>
      <form className="stacked-form" onSubmit={(event) => void view.handleAuth(event)}>
        <label>
          <span>Display name</span>
          <input name="displayName" placeholder="Avery" autoComplete="name" />
        </label>
        <label>
          <span>Email</span>
          <input name="email" type="email" placeholder="avery@example.com" autoComplete="email" required />
        </label>
        <label>
          <span>Password</span>
          <input name="password" type="password" placeholder="12+ chars, mixed case, number" autoComplete="current-password" required />
        </label>
        <div className="button-pair">
          <button type="submit" className="primary-button" data-mode="login" disabled={view.isPending('auth')}>Log in</button>
          <button type="submit" className="secondary-button" data-mode="register" disabled={view.isPending('auth')}>Register</button>
        </div>
      </form>
    </section>
  );
}

function CreateEventForm({ view }: { view: ViewModel }) {
  return (
    <form className="stacked-form publish-form" onSubmit={(event) => void view.handleCreateEvent(event)}>
      <label>
        <span>Title</span>
        <input name="title" placeholder="Rooftop board game night" required />
      </label>
      <div className="form-pair">
        <label>
          <span>City</span>
          <input name="city" placeholder="Sydney" defaultValue={view.filters.city} required />
        </label>
        <label>
          <span>Venue</span>
          <input name="venue" placeholder="Surry Hills" required />
        </label>
      </div>
      <div className="form-pair">
        <label>
          <span>Start time</span>
          <input name="startsAt" type="datetime-local" defaultValue={defaultStartAt()} required />
        </label>
        <label>
          <span>Capacity</span>
          <input name="capacity" type="number" min="1" defaultValue="24" required />
        </label>
      </div>
      <label>
        <span>Description</span>
        <textarea name="description" placeholder="What should people expect?"></textarea>
      </label>
      <button type="submit" className="primary-button" disabled={view.isPending('create')}>{view.isPending('create') ? 'Publishing' : 'Publish event'}</button>
    </form>
  );
}

function RoleBlockedPanel({ user, view }: { user: User | null; view: ViewModel }) {
  return (
    <StateMessage title="Organizer role required" detail={`Your current role is ${user?.role || 'USER'}. Use the seeded admin account to promote this account before publishing.`}>
      <RouteLink className="primary-button" href="/me" onRouteClick={view.onRouteClick}>Account</RouteLink>
    </StateMessage>
  );
}

function SelectedEventSummary({ view }: { view: ViewModel }) {
  const event = view.getEventForDisplay(view.selectedEvent);
  if (!event) {
    return <StateMessage title="No event selected" detail="Open an event before creating media upload intent." />;
  }
  return (
    <RouteLink className="selected-event" href={`/events/${encodeURIComponent(event.id)}`} onRouteClick={view.onRouteClick}>
      <EventImage event={event} className="selected-image" />
      <span>
        <strong>{event.title}</strong>
        <small>{formatCompactDate(event.startsAt)} / {event.venue}</small>
      </span>
    </RouteLink>
  );
}

function MediaPanel({ view }: { view: ViewModel }) {
  const event = view.getEventForDisplay(view.selectedEvent);
  const liveSelected = Boolean(event && event.source !== 'demo');
  return (
    <section className="compact-panel">
      <div className="section-heading">
        <span><strong>Media upload</strong></span>
        <span className="label-pill soft">Intent</span>
      </div>
      <form className="stacked-form" onSubmit={(event) => void view.handleCreateUpload(event)}>
        <label>
          <span>Event ID</span>
          <input name="eventId" defaultValue={liveSelected ? view.selectedEvent : ''} placeholder="Select a live event first" required />
        </label>
        <div className="form-pair">
          <label>
            <span>Filename</span>
            <input name="filename" placeholder="banner.jpg" defaultValue="banner.jpg" required />
          </label>
          <label>
            <span>Type</span>
            <select name="contentType" defaultValue="image/jpeg">
              <option value="image/jpeg">image/jpeg</option>
              <option value="image/png">image/png</option>
              <option value="image/webp">image/webp</option>
            </select>
          </label>
        </div>
        <label>
          <span>Size bytes</span>
          <input name="sizeBytes" type="number" min="1" defaultValue="1024" required />
        </label>
        <button type="submit" className="secondary-button" disabled={!liveSelected || view.isPending('media')}>{view.isPending('media') ? 'Creating' : 'Create upload intent'}</button>
        {view.media ? <MediaResult media={view.media} /> : <p className="form-hint">Media is available for live events created in CityEvents.</p>}
      </form>
    </section>
  );
}

function AdminRolePanel({ view }: { view: ViewModel }) {
  return (
    <section className="compact-panel">
      <div className="section-heading">
        <span><strong>Role admin</strong></span>
        <span className="label-pill soft">ADMIN</span>
      </div>
      <form className="stacked-form" onSubmit={(event) => void view.handleAdminRoleUpdate(event)}>
        <label>
          <span>User ID</span>
          <input name="userId" placeholder="Paste target user id" required />
        </label>
        <label>
          <span>Role</span>
          <select name="role" defaultValue="USER">
            <option value="USER">USER</option>
            <option value="ORGANIZER">ORGANIZER</option>
            <option value="ADMIN">ADMIN</option>
          </select>
        </label>
        <button type="submit" className="secondary-button" disabled={view.isPending('role')}>{view.isPending('role') ? 'Updating' : 'Update role'}</button>
      </form>
    </section>
  );
}

function RegistrationModerationPanel({ event, isLive, view }: { event: CityEvent; isLive: boolean; view: ViewModel }) {
  if (!isLive || !canManageEvent(event, view.auth.user)) return null;
  return (
    <section className="compact-panel moderation-panel">
      <div className="section-heading">
        <span><strong>Attendee moderation</strong></span>
        <span className="label-pill soft">{view.auth.user?.role || ''}</span>
      </div>
      <form className="stacked-form" onSubmit={(event) => void view.handleCancelRegistration(event)}>
        <label>
          <span>User ID</span>
          <input name="userId" placeholder="Attendee user id" required />
        </label>
        <button type="submit" className="secondary-button" disabled={view.isPending('moderation')}>{view.isPending('moderation') ? 'Canceling' : 'Cancel attendee RSVP'}</button>
      </form>
    </section>
  );
}

function MediaResult({ media }: { media: MediaUploadPayload }) {
  const asset = media.asset || {};
  return (
    <div className="media-result">
      <span className={`label-pill ${toneClass(asset.status)}`}>{statusLabel(asset.status)}</span>
      <code>{asset.id || 'media-id-pending'}</code>
    </div>
  );
}

function Notice({ message, tone }: { message: string; tone: Tone }) {
  return (
    <div className={`notice-toast tone-${tone}`} aria-live="polite">
      <span>{message}</span>
    </div>
  );
}

function StateMessage({ title, detail, children }: { title: string; detail: string; children?: ReactNode }) {
  return (
    <div className="state-message">
      <strong>{title}</strong>
      <span>{detail}</span>
      {children}
    </div>
  );
}

function RouteLink({ href, className, onRouteClick, children }: { href: string; className?: string; onRouteClick: (event: MouseEvent<HTMLAnchorElement>) => void; children: ReactNode }) {
  return <a className={className} href={href} onClick={onRouteClick}>{children}</a>;
}

function parseRoute(path: string): Route {
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

function normalizeEvent(item: unknown, source: EventSource = 'live', rank = 0, fallbackCity = 'Sydney', confirmedOverride?: number): CityEvent {
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

function canManageEvent(event: CityEvent, user: User | null): boolean {
  if (!user?.id) return false;
  if (user.role === 'ADMIN') return true;
  return user.role === 'ORGANIZER' && event.organizerID === user.id;
}

function asRawEvent(value: unknown): RawEvent | null {
  if (typeof value !== 'object' || value === null) return null;
  return value as RawEvent;
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

function categoryName(slug: string): string {
  return categoryBySlug.get(slug)?.name || 'Event';
}

function categoryAccent(event: CityEvent): string {
  return categoryBySlug.get(event.category)?.accent || 'teal';
}

function spotsLeft(event: CityEvent): number {
  return Math.max(0, Number(event.capacity || 0) - Number(event.confirmedCount || 0));
}

function spotFill(event: CityEvent, confirmedCount: number): number {
  const capacity = Number(event.capacity || 1);
  return Math.max(6, Math.min(100, Math.round((Number(confirmedCount || 0) / capacity) * 100)));
}

function rsvpHeadline(event: CityEvent, joinStatus: string, isLive: boolean, user: User | null): string {
  if (!isLive) return 'Preview listing';
  if (!user?.id) return 'Sign in to reserve a spot';
  switch (joinStatus) {
    case 'CONFIRMED': return 'You are going';
    case 'WAITLISTED': return 'You are waitlisted';
    case 'CANCELED': return 'You canceled this RSVP';
    default: return `${spotsLeft(event)} spots left`;
  }
}

function rsvpDetail(joinStatus: string, isLive: boolean, user: User | null): string {
  if (!isLive) return 'This card shows the discovery experience. Live RSVP is available for events created in CityEvents.';
  if (!user?.id) return 'Browsing stays open. Sign in only when you want to join, publish, or manage media.';
  switch (joinStatus) {
    case 'CONFIRMED': return 'Your spot is confirmed. Cancel if plans change.';
    case 'WAITLISTED': return 'You are in line if another attendee cancels.';
    case 'CANCELED': return 'You can join again if the event is still open.';
    default: return 'Join once and your status updates here.';
  }
}

function defaultStartAt(): string {
  const date = new Date(Date.now() + 24 * 60 * 60 * 1000);
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60 * 1000);
  return local.toISOString().slice(0, 16);
}

function relativeDate(days: number, hour: number, minute: number): string {
  const date = new Date();
  date.setDate(date.getDate() + days);
  date.setHours(hour, minute, 0, 0);
  return date.toISOString();
}

function formatCompactDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'Time pending';
  return date.toLocaleString('en-AU', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

function dayLabel(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--';
  return date.toLocaleString('en-AU', { day: '2-digit' });
}

function toneClass(status: string | undefined): string {
  return `tone-${statusTone[status || ''] || 'neutral'}`;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Request failed.';
}
