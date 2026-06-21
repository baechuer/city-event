import { categories, quickCities } from '../../domain/events';
import type { ViewModel } from '../../appView';

export function SearchForm({ id, placeholder, view }: { id: string; placeholder: string; view: ViewModel }) {
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

export function FilterChips({ view }: { view: ViewModel }) {
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
