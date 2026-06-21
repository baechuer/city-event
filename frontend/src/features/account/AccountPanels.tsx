import { RouteLink } from '../../components/RouteLink';
import { StateMessage } from '../../components/StateMessage';
import type { MediaUploadPayload } from '../../api';
import type { ViewModel } from '../../appView';
import { canManageEvent, defaultStartAt, formatCompactDate, type CityEvent } from '../../domain/events';
import { statusLabel, statusTone, type User } from '../../state';

export function CreateEventForm({ view }: { view: ViewModel }) {
  return (
    <form className="stacked-form publish-form" onSubmit={(event) => void view.handleCreateEvent(event)}>
      <label>
        <span>Title</span>
        <input name="title" placeholder="Rooftop board game night" required />
      </label>
      <div className="form-pair">
        <label>
          <span>City</span>
          <input name="city" placeholder="Sydney" defaultValue={view.filters.city} required />
        </label>
        <label>
          <span>Venue</span>
          <input name="venue" placeholder="Surry Hills" required />
        </label>
      </div>
      <div className="form-pair">
        <label>
          <span>Start time</span>
          <input name="startsAt" type="datetime-local" defaultValue={defaultStartAt()} required />
        </label>
        <label>
          <span>Capacity</span>
          <input name="capacity" type="number" min="1" defaultValue="24" required />
        </label>
      </div>
      <label>
        <span>Description</span>
        <textarea name="description" placeholder="What should people expect?"></textarea>
      </label>
      <button type="submit" className="primary-button" disabled={view.isPending('create')}>{view.isPending('create') ? 'Publishing' : 'Publish event'}</button>
    </form>
  );
}

export function RoleBlockedPanel({ user, view }: { user: User | null; view: ViewModel }) {
  return (
    <StateMessage title="Organizer role required" detail={`Your current role is ${user?.role || 'USER'}. Use the seeded admin account to promote this account before publishing.`}>
      <RouteLink className="primary-button" href="/me" onRouteClick={view.onRouteClick}>Account</RouteLink>
    </StateMessage>
  );
}

export function MediaPanel({ view }: { view: ViewModel }) {
  const event = view.getEventForDisplay(view.selectedEvent);
  const liveSelected = Boolean(event && event.source !== 'demo');
  return (
    <section className="compact-panel">
      <div className="section-heading">
        <span><strong>Media upload</strong></span>
        <span className="label-pill soft">Intent</span>
      </div>
      <form className="stacked-form" onSubmit={(event) => void view.handleCreateUpload(event)}>
        <label>
          <span>Event ID</span>
          <input name="eventId" defaultValue={liveSelected ? view.selectedEvent : ''} placeholder="Select a live event first" required />
        </label>
        <div className="form-pair">
          <label>
            <span>Filename</span>
            <input name="filename" placeholder="banner.jpg" defaultValue="banner.jpg" required />
          </label>
          <label>
            <span>Type</span>
            <select name="contentType" defaultValue="image/jpeg">
              <option value="image/jpeg">image/jpeg</option>
              <option value="image/png">image/png</option>
              <option value="image/webp">image/webp</option>
            </select>
          </label>
        </div>
        <label>
          <span>Size bytes</span>
          <input name="sizeBytes" type="number" min="1" defaultValue="1024" required />
        </label>
        <button type="submit" className="secondary-button" disabled={!liveSelected || view.isPending('media')}>{view.isPending('media') ? 'Creating' : 'Create upload intent'}</button>
        {view.media ? <MediaResult media={view.media} /> : <p className="form-hint">Media is available for live events created in CityEvents.</p>}
      </form>
    </section>
  );
}

export function AdminRolePanel({ view }: { view: ViewModel }) {
  return (
    <section className="compact-panel">
      <div className="section-heading">
        <span><strong>Role admin</strong></span>
        <span className="label-pill soft">ADMIN</span>
      </div>
      <form className="stacked-form" onSubmit={(event) => void view.handleAdminRoleUpdate(event)}>
        <label>
          <span>User ID</span>
          <input name="userId" placeholder="Paste target user id" required />
        </label>
        <label>
          <span>Role</span>
          <select name="role" defaultValue="USER">
            <option value="USER">USER</option>
            <option value="ORGANIZER">ORGANIZER</option>
            <option value="ADMIN">ADMIN</option>
          </select>
        </label>
        <button type="submit" className="secondary-button" disabled={view.isPending('role')}>{view.isPending('role') ? 'Updating' : 'Update role'}</button>
      </form>
    </section>
  );
}

export function RegistrationModerationPanel({ event, isLive, view }: { event: CityEvent; isLive: boolean; view: ViewModel }) {
  if (!isLive || !canManageEvent(event, view.auth.user)) return null;
  return (
    <section className="compact-panel moderation-panel">
      <div className="section-heading">
        <span><strong>Attendee moderation</strong></span>
        <span className="label-pill soft">{view.auth.user?.role || ''}</span>
      </div>
      <form className="stacked-form" onSubmit={(event) => void view.handleCancelRegistration(event)}>
        <label>
          <span>User ID</span>
          <input name="userId" placeholder="Attendee user id" required />
        </label>
        <button type="submit" className="secondary-button" disabled={view.isPending('moderation')}>{view.isPending('moderation') ? 'Canceling' : 'Cancel attendee RSVP'}</button>
      </form>
    </section>
  );
}

function MediaResult({ media }: { media: MediaUploadPayload }) {
  const asset = media.asset || {};
  return (
    <div className="media-result">
      <span className={`label-pill ${toneClass(asset.status)}`}>{statusLabel(asset.status)}</span>
      <code>{asset.id || 'media-id-pending'}</code>
    </div>
  );
}

function toneClass(status: string | undefined): string {
  return `tone-${statusTone[status || ''] || 'neutral'}`;
}
