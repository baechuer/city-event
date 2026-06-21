import type { ViewModel } from '../appView';
import { RouteLink } from '../components/RouteLink';
import { categories, categoryBySlug } from '../domain/events';
import { EmptyEvents, EventCard } from '../features/events/EventCards';

export function CategoryPage({ view, slug }: { view: ViewModel; slug: string }) {
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
