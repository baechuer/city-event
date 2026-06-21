import type { ViewModel } from '../appView';
import { AccountPage } from './AccountPage';
import { CategoriesPage } from './CategoriesPage';
import { CategoryPage } from './CategoryPage';
import { EventDetailPage } from './EventDetailPage';
import { EventsPage } from './EventsPage';
import { HomePage } from './HomePage';
import { PublishPage } from './PublishPage';

export function PageRouter({ view }: { view: ViewModel }) {
  if (view.route.name === 'eventDetail') return <EventDetailPage view={view} eventID={view.route.eventID || ''} />;
  if (view.route.name === 'events') return <EventsPage view={view} />;
  if (view.route.name === 'categories') return <CategoriesPage view={view} />;
  if (view.route.name === 'category') return <CategoryPage view={view} slug={view.route.slug || ''} />;
  if (view.route.name === 'publish') return <PublishPage view={view} />;
  if (view.route.name === 'me') return <AccountPage view={view} />;
  return <HomePage view={view} />;
}
