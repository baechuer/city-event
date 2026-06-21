import { useCallback, useEffect, useMemo, useState, type FormEvent, type MouseEvent } from 'react';
import { createApiClient, type EventDetailPayload, type MediaUploadPayload } from './api';
import type { Filters, ViewModel } from './appView';
import { Notice } from './components/Notice';
import { Topbar } from './components/Topbar';
import { asRawEvent, categoryName, demoEvents, normalizeEvent, type CityEvent } from './domain/events';
import { PageRouter } from './pages/PageRouter';
import { parseRoute } from './routing';
import { canPublish, createInitialState, type AuthResult, type Tone } from './state';

declare global {
  interface Window {
    CITYEVENTS_CONFIG?: Record<string, string>;
  }
}

const api = createApiClient(window.CITYEVENTS_CONFIG || {});

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

  const view: ViewModel = {
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
        <PageRouter view={view} />
      </main>
      {notice ? <Notice message={notice} tone={noticeTone} /> : null}
    </>
  );
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Request failed.';
}
