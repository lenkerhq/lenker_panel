import { FormEvent, useCallback, useMemo, useState } from "react";
import { getApiBaseUrl, loginAdmin, PanelApiError } from "../lib/api";
import { clearStoredSession, loadStoredSession, saveStoredSession, type StoredSession } from "../lib/session";
import { NodesPage } from "../pages/NodesPage";
import { PlansPage } from "../pages/PlansPage";
import { SubscriptionsPage } from "../pages/SubscriptionsPage";
import { SubscriptionTemplatesPage } from "../pages/SubscriptionTemplatesPage";
import { UsersPage } from "../pages/UsersPage";

interface LoginFormState {
  email: string;
  password: string;
}

type PanelPage = "dashboard" | "users" | "plans" | "subscriptions" | "templates" | "nodes";

interface NavigationItem {
  page: PanelPage;
  label: string;
}

const initialLoginFormState: LoginFormState = {
  email: "owner@example.com",
  password: "",
};

const navigationItems: NavigationItem[] = [
  { page: "dashboard", label: "Dashboard" },
  { page: "users", label: "Users" },
  { page: "plans", label: "Plans" },
  { page: "subscriptions", label: "Subscriptions" },
  { page: "templates", label: "Templates" },
  { page: "nodes", label: "Nodes" },
];

export function App() {
  const [storedSession, setStoredSession] = useState<StoredSession | null>(() => loadStoredSession());
  const [activePage, setActivePage] = useState<PanelPage>("dashboard");
  const [formState, setFormState] = useState<LoginFormState>(initialLoginFormState);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const expiresAtLabel = useMemo(() => {
    if (!storedSession?.session.expires_at) {
      return "No active session";
    }

    return new Intl.DateTimeFormat(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(storedSession.session.expires_at));
  }, [storedSession]);

  const logout = useCallback((message?: string) => {
    clearStoredSession();
    setStoredSession(null);
    setActivePage("dashboard");
    setFormState(initialLoginFormState);
    setErrorMessage(message ?? null);
  }, []);

  const handleUnauthorized = useCallback(() => {
    logout("Session expired. Sign in again.");
  }, [logout]);

  function updateFormField(fieldName: keyof LoginFormState, value: string) {
    setFormState((currentValue) => ({ ...currentValue, [fieldName]: value }));
  }

  async function submitLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const email = formState.email.trim();
    const password = formState.password;

    if (!email || !password) {
      setErrorMessage("Email and password are required.");
      return;
    }

    setIsSubmitting(true);
    setErrorMessage(null);

    try {
      const session = await loginAdmin(email, password);
      saveStoredSession(session);
      setStoredSession(session);
      setFormState(initialLoginFormState);
    } catch (error) {
      if (error instanceof PanelApiError) {
        setErrorMessage(`${error.message} (${error.code})`);
      } else {
        setErrorMessage("Unable to connect to panel-api.");
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  if (!storedSession) {
    return (
      <main className="auth-layout">
        <form className="auth-card" onSubmit={submitLogin}>
          <p className="eyebrow">Lenker Provider Panel</p>
          <h1>Admin access</h1>
          <p className="muted-text">
            Sign in with the local admin created by <code>make docker-bootstrap-admin</code>.
          </p>

          <label className="field-label" htmlFor="email">
            Admin email
          </label>
          <input
            id="email"
            className="text-field"
            type="email"
            autoComplete="username"
            value={formState.email}
            onChange={(event) => updateFormField("email", event.target.value)}
          />

          <label className="field-label" htmlFor="password">
            Password
          </label>
          <input
            id="password"
            className="text-field"
            type="password"
            autoComplete="current-password"
            value={formState.password}
            onChange={(event) => updateFormField("password", event.target.value)}
          />

          {errorMessage ? <p className="error-text">{errorMessage}</p> : null}

          <button className="primary-button" type="submit" disabled={isSubmitting}>
            {isSubmitting ? "Signing in..." : "Sign in"}
          </button>

          <p className="helper-text">API target: {getApiBaseUrl()}</p>
        </form>
      </main>
    );
  }

  return (
    <main className="panel-layout">
      <aside className="sidebar">
        <div>
          <p className="eyebrow">Lenker</p>
          <h1>Provider Panel</h1>
        </div>
        <nav className="nav-list" aria-label="Primary navigation">
          {navigationItems.map((item) => (
            <button
              key={item.page}
              className={item.page === activePage ? "nav-link active" : "nav-link"}
              type="button"
              onClick={() => setActivePage(item.page)}
            >
              {item.label}
            </button>
          ))}
        </nav>
      </aside>

      <section className="content-shell">
        <header className="topbar">
          <div>
            <p className="muted-text">Signed in as</p>
            <strong>{storedSession.admin.email}</strong>
          </div>
          <button className="secondary-button" type="button" onClick={() => logout()}>
            Sign out
          </button>
        </header>

        {activePage === "dashboard" ? <Dashboard expiresAtLabel={expiresAtLabel} /> : null}
        {activePage === "users" ? <UsersPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "plans" ? <PlansPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "subscriptions" ? <SubscriptionsPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "templates" ? <SubscriptionTemplatesPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "nodes" ? <NodesPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
      </section>
    </main>
  );
}

interface DashboardProps {
  expiresAtLabel: string;
}

function Dashboard({ expiresAtLabel }: DashboardProps) {
  return (
    <>
      <section className="hero-card" id="dashboard">
        <p className="eyebrow">MVP v0.1</p>
        <h2>Dashboard shell is ready</h2>
        <p>
          The React app authenticates against panel-api, stores the admin session for the current browser tab,
          and renders the first provider dashboard shell.
        </p>
        <dl className="details-grid">
          <div>
            <dt>Session expires</dt>
            <dd>{expiresAtLabel}</dd>
          </div>
          <div>
            <dt>Backend target</dt>
            <dd>{getApiBaseUrl()}</dd>
          </div>
        </dl>
      </section>

      <section className="cards-grid">
        <StatusCard title="Users" value="Live" description="Create, edit, suspend, and activate users." />
        <StatusCard title="Plans" value="Live" description="Create, edit, and archive subscription plans." />
        <StatusCard title="Subscriptions" value="Live" description="Create, update, and renew subscriptions." />
        <StatusCard title="Nodes" value="Live" description="Bootstrap, inspect, drain, disable, and enable nodes." />
      </section>
    </>
  );
}

interface StatusCardProps {
  title: string;
  value: string;
  description: string;
}

function StatusCard({ title, value, description }: StatusCardProps) {
  return (
    <article className="status-card">
      <p className="muted-text">{title}</p>
      <strong>{value}</strong>
      <span>{description}</span>
    </article>
  );
}
