import { FormEvent, useCallback, useMemo, useState } from "react";
import { getApiBaseUrl, loginAdmin, PanelApiError } from "../lib/api";
import { clearStoredSession, loadStoredSession, saveStoredSession, type StoredSession } from "../lib/session";
import { useI18n } from "../lib/i18n";
import { NodeProfilesPage } from "../pages/NodeProfilesPage";
import { NodesPage } from "../pages/NodesPage";
import { PlansPage } from "../pages/PlansPage";
import { RoutingRulesPage } from "../pages/RoutingRulesPage";
import { SettingsPage } from "../pages/SettingsPage";
import { SubscriptionsPage } from "../pages/SubscriptionsPage";
import { SubscriptionTemplatesPage } from "../pages/SubscriptionTemplatesPage";
import { UsersPage } from "../pages/UsersPage";
import { WarpConfigurationPage } from "../pages/WarpConfigurationPage";

interface LoginFormState {
  email: string;
  password: string;
}

type PanelPage = "dashboard" | "users" | "plans" | "subscriptions" | "templates" | "nodes" | "profiles" | "warp" | "routing" | "settings";

const initialLoginFormState: LoginFormState = {
  email: "owner@example.com",
  password: "",
};

const NAV_KEYS: { page: PanelPage; labelKey: "nav.dashboard" | "nav.users" | "nav.plans" | "nav.subscriptions" | "nav.templates" | "nav.nodes" | "nav.profiles" | "nav.warp" | "nav.routing" | "nav.settings" }[] = [
  { page: "dashboard", labelKey: "nav.dashboard" },
  { page: "users", labelKey: "nav.users" },
  { page: "plans", labelKey: "nav.plans" },
  { page: "subscriptions", labelKey: "nav.subscriptions" },
  { page: "templates", labelKey: "nav.templates" },
  { page: "nodes", labelKey: "nav.nodes" },
  { page: "profiles", labelKey: "nav.profiles" },
  { page: "warp", labelKey: "nav.warp" },
  { page: "routing", labelKey: "nav.routing" },
  { page: "settings", labelKey: "nav.settings" },
];

function LanguageSwitcher() {
  const { locale, setLocale } = useI18n();
  return (
    <div className="language-switcher">
      <button
        type="button"
        className={locale === "en" ? "lang-btn active" : "lang-btn"}
        onClick={() => setLocale("en")}
      >
        EN
      </button>
      <button
        type="button"
        className={locale === "ru" ? "lang-btn active" : "lang-btn"}
        onClick={() => setLocale("ru")}
      >
        RU
      </button>
    </div>
  );
}

export function App() {
  const { t } = useI18n();
  const [storedSession, setStoredSession] = useState<StoredSession | null>(() => loadStoredSession());
  const [activePage, setActivePage] = useState<PanelPage>("dashboard");
  const [formState, setFormState] = useState<LoginFormState>(initialLoginFormState);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const expiresAtLabel = useMemo(() => {
    if (!storedSession?.session.expires_at) {
      return t("session.none");
    }
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(storedSession.session.expires_at));
  }, [storedSession, t]);

  const logout = useCallback((message?: string) => {
    clearStoredSession();
    setStoredSession(null);
    setActivePage("dashboard");
    setFormState(initialLoginFormState);
    setErrorMessage(message ?? null);
  }, []);

  const handleUnauthorized = useCallback(() => {
    logout(t("session.expired"));
  }, [logout, t]);

  function updateFormField(fieldName: keyof LoginFormState, value: string) {
    setFormState((currentValue) => ({ ...currentValue, [fieldName]: value }));
  }

  async function submitLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const email = formState.email.trim();
    const password = formState.password;

    if (!email || !password) {
      setErrorMessage(t("login.error.required"));
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
        setErrorMessage(t("login.error.connection"));
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  if (!storedSession) {
    return (
      <main className="auth-layout">
        <form className="auth-card" onSubmit={submitLogin}>
          <LanguageSwitcher />
          <p className="eyebrow">{t("login.eyebrow")}</p>
          <h1>{t("login.title")}</h1>
          <p className="muted-text">
            {t("login.subtitle")}
          </p>

          <label className="field-label" htmlFor="email">
            {t("login.email")}
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
            {t("login.password")}
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
            {isSubmitting ? t("login.submitting") : t("login.submit")}
          </button>

          <p className="helper-text">{t("login.api_target")}: {getApiBaseUrl()}</p>
        </form>
      </main>
    );
  }

  return (
    <main className="panel-layout">
      <aside className="sidebar">
        <div>
          <p className="eyebrow">Lenker</p>
          <h1>{t("sidebar.title")}</h1>
        </div>
        <nav className="nav-list" aria-label="Primary navigation">
          {NAV_KEYS.map((item) => (
            <button
              key={item.page}
              className={item.page === activePage ? "nav-link active" : "nav-link"}
              type="button"
              onClick={() => setActivePage(item.page)}
            >
              {t(item.labelKey)}
            </button>
          ))}
        </nav>
        <LanguageSwitcher />
      </aside>

      <section className="content-shell">
        <header className="topbar">
          <div>
            <p className="muted-text">{t("topbar.signed_in_as")}</p>
            <strong>{storedSession.admin.email}</strong>
          </div>
          <button className="secondary-button" type="button" onClick={() => logout()}>
            {t("topbar.sign_out")}
          </button>
        </header>

        {activePage === "dashboard" ? <Dashboard expiresAtLabel={expiresAtLabel} /> : null}
        {activePage === "users" ? <UsersPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "plans" ? <PlansPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "subscriptions" ? <SubscriptionsPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "templates" ? <SubscriptionTemplatesPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "nodes" ? <NodesPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "profiles" ? <NodeProfilesPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "warp" ? <WarpConfigurationPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "routing" ? <RoutingRulesPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
        {activePage === "settings" ? <SettingsPage session={storedSession} onUnauthorized={handleUnauthorized} /> : null}
      </section>
    </main>
  );
}

interface DashboardProps {
  expiresAtLabel: string;
}

function Dashboard({ expiresAtLabel }: DashboardProps) {
  const { t } = useI18n();
  return (
    <>
      <section className="hero-card" id="dashboard">
        <p className="eyebrow">{t("dashboard.eyebrow")}</p>
        <h2>{t("dashboard.title")}</h2>
        <p>{t("dashboard.description")}</p>
        <dl className="details-grid">
          <div>
            <dt>{t("dashboard.session_expires")}</dt>
            <dd>{expiresAtLabel}</dd>
          </div>
          <div>
            <dt>{t("dashboard.backend_target")}</dt>
            <dd>{getApiBaseUrl()}</dd>
          </div>
        </dl>
      </section>

      <section className="cards-grid">
        <StatusCard title={t("dashboard.users")} value={t("dashboard.live")} description={t("dashboard.users_desc")} />
        <StatusCard title={t("dashboard.plans")} value={t("dashboard.live")} description={t("dashboard.plans_desc")} />
        <StatusCard title={t("dashboard.subscriptions")} value={t("dashboard.live")} description={t("dashboard.subscriptions_desc")} />
        <StatusCard title={t("dashboard.nodes")} value={t("dashboard.live")} description={t("dashboard.nodes_desc")} />
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
