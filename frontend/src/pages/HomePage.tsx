import type { ViewModel } from '../appView';
import { RouteLink } from '../components/RouteLink';
import {
  categories,
  categoryAccent,
  demoEvents,
  formatCompactDate,
  normalizeEvent,
  spotsLeft,
} from '../domain/events';
import { CategoryTile, EventImage, MiniEvent, SignalEvent } from '../features/events/EventCards';
import { SearchForm } from '../features/events/SearchControls';

export function HomePage({ view }: { view: ViewModel }) {
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
