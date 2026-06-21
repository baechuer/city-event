import type { ViewModel } from '../appView';
import { EmptyEvents, EventCard } from '../features/events/EventCards';
import { FilterChips, SearchForm } from '../features/events/SearchControls';

export function EventsPage({ view }: { view: ViewModel }) {
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
