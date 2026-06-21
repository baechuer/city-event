import type { ViewModel } from '../appView';
import { categories } from '../domain/events';
import { CategoryFeature } from '../features/events/EventCards';

export function CategoriesPage({ view }: { view: ViewModel }) {
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
