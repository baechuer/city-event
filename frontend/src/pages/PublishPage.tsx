import type { ViewModel } from '../appView';
import { AuthPanel } from '../features/account/AuthPanel';
import { CreateEventForm, RoleBlockedPanel } from '../features/account/AccountPanels';
import { canPublish } from '../state';

export function PublishPage({ view }: { view: ViewModel }) {
  const signedIn = Boolean(view.auth.user?.id);
  const allowed = canPublish(view.auth.user);
  return (
    <section className="publish-layout">
      <div className="page-intro">
        <p className="eyebrow">Organizer</p>
        <h1>Publish a real event into the live feed.</h1>
        <p>Keep creation focused: title, place, time, capacity, and a short reason to show up.</p>
      </div>
      <div className="publish-card">
        {signedIn ? allowed ? <CreateEventForm view={view} /> : <RoleBlockedPanel user={view.auth.user} view={view} /> : <AuthPanel title="Sign in to publish" detail="Create your account first, then the publish form unlocks." view={view} />}
      </div>
    </section>
  );
}
