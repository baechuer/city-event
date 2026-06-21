import type { CSSProperties } from 'react';
import type { ViewModel } from '../appView';
import { RouteLink } from '../components/RouteLink';
import { StateMessage } from '../components/StateMessage';
import {
  asRawEvent,
  categoryAccent,
  categoryName,
  rsvpDetail,
  rsvpHeadline,
  spotFill,
  type CityEvent,
} from '../domain/events';
import { RegistrationModerationPanel } from '../features/account/AccountPanels';
import { EventImage } from '../features/events/EventCards';
import { canCancel, canJoin, formatDateTime } from '../state';

export function EventDetailPage({ view, eventID }: { view: ViewModel; eventID: string }) {
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

  return <LoadedEventDetail event={event} view={view} />;
}

function LoadedEventDetail({ event, view }: { event: CityEvent; view: ViewModel }) {
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
