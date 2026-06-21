import type { MouseEvent } from 'react';
import { RouteLink } from './RouteLink';
import type { Route } from '../routing';
import type { AuthResult } from '../state';

export function Topbar({
  route,
  auth,
  onRouteClick,
}: {
  route: Route;
  auth: AuthResult;
  onRouteClick(event: MouseEvent<HTMLAnchorElement>): void;
}) {
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

function NavLink({
  label,
  href,
  route,
  onRouteClick,
}: {
  label: string;
  href: string;
  route: Route;
  onRouteClick(event: MouseEvent<HTMLAnchorElement>): void;
}) {
  const active = href === '/' ? route.name === 'home' : route.path === href || route.path.startsWith(`${href}/`);
  return (
    <RouteLink className={active ? 'active' : ''} href={href} onRouteClick={onRouteClick}>
      {label}
    </RouteLink>
  );
}
