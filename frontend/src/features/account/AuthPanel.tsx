import type { ViewModel } from '../../appView';

export function AuthPanel({ title, detail, view }: { title: string; detail: string; view: ViewModel }) {
  return (
    <section className="auth-card">
      <div>
        <p className="eyebrow">Account</p>
        <h2>{title}</h2>
        <p>{detail}</p>
      </div>
      <form className="stacked-form" onSubmit={(event) => void view.handleAuth(event)}>
        <label>
          <span>Display name</span>
          <input name="displayName" placeholder="Avery" autoComplete="name" />
        </label>
        <label>
          <span>Email</span>
          <input name="email" type="email" placeholder="avery@example.com" autoComplete="email" required />
        </label>
        <label>
          <span>Password</span>
          <input name="password" type="password" placeholder="12+ chars, mixed case, number" autoComplete="current-password" required />
        </label>
        <div className="button-pair">
          <button type="submit" className="primary-button" data-mode="login" disabled={view.isPending('auth')}>Log in</button>
          <button type="submit" className="secondary-button" data-mode="register" disabled={view.isPending('auth')}>Register</button>
        </div>
      </form>
    </section>
  );
}
