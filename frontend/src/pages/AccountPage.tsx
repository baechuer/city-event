import type { ViewModel } from '../appView';
import { RouteLink } from '../components/RouteLink';
import { AuthPanel } from '../features/account/AuthPanel';
import { AdminRolePanel, MediaPanel } from '../features/account/AccountPanels';
import { SelectedEventSummary } from '../features/events/EventCards';

export function AccountPage({ view }: { view: ViewModel }) {
  const user = view.auth.user;
  if (!user) {
    return (
      <section className="account-layout">
        <div className="page-intro">
          <p className="eyebrow">Account</p>
          <h1>Sign in only when you are ready to act.</h1>
          <p>Visitors can browse first. Joining, canceling, publishing, and media upload intents require identity.</p>
        </div>
        <AuthPanel title="Enter CityEvents" detail="Use a 12+ character password with mixed case and a number." view={view} />
      </section>
    );
  }

  return (
    <section className="account-layout signed-in">
      <div className="profile-card">
        <p className="eyebrow">Signed in</p>
        <h1>{user.displayName || 'CityEvents user'}</h1>
        <p>{user.email || user.id}</p>
        <span className="label-pill soft">{user.role || 'USER'}</span>
        <button type="button" className="secondary-button" onClick={() => void view.handleLogout()}>Log out</button>
      </div>
      <div className="account-actions">
        <section className="compact-panel">
          <div className="section-heading">
            <span><strong>Selected event</strong></span>
            <RouteLink href="/events" onRouteClick={view.onRouteClick}>Choose event</RouteLink>
          </div>
          <SelectedEventSummary view={view} />
        </section>
        {user.role === 'ADMIN' ? <AdminRolePanel view={view} /> : null}
        <MediaPanel view={view} />
      </div>
    </section>
  );
}
