import { RouteLink } from '../../components/RouteLink';
import { StateMessage } from '../../components/StateMessage';
import {
  categories,
  categoryAccent,
  categoryBySlug,
  categoryName,
  dayLabel,
  formatCompactDate,
  spotsLeft,
  type Category,
  type CityEvent,
} from '../../domain/events';
import type { ViewModel } from '../../appView';

export function EventImage({ event, className }: { event: CityEvent; className: string }) {
  const category = categoryBySlug.get(event.category) || categories[0];
  return <img className={className} src={category.image} alt="" />;
}

export function EventCard({ event, view }: { event: CityEvent; view: ViewModel }) {
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

export function SignalEvent({ event, view }: { event: CityEvent; view: ViewModel }) {
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

export function MiniEvent({ event, view }: { event: CityEvent; view: ViewModel }) {
  return (
    <RouteLink className="mini-event" href={`/events/${encodeURIComponent(event.id)}`} onRouteClick={view.onRouteClick}>
      <strong>{event.title}</strong>
      <span>{formatCompactDate(event.startsAt)} / {spotsLeft(event)} spots</span>
    </RouteLink>
  );
}

export function EmptyEvents({ view, categoryNameValue = 'this lane' }: { view: ViewModel; categoryNameValue?: string }) {
  return (
    <StateMessage title={`No live events in ${categoryNameValue} yet`} detail="Publish one to test the live RSVP and feed projection flow.">
      <RouteLink className="primary-button" href="/publish" onRouteClick={view.onRouteClick}>Publish event</RouteLink>
    </StateMessage>
  );
}

export function SelectedEventSummary({ view }: { view: ViewModel }) {
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

export function CategoryTile({ category, view }: { category: Category; view: ViewModel }) {
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

export function CategoryFeature({ category, view }: { category: Category; view: ViewModel }) {
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
