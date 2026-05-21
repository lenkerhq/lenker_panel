import { createContext, useContext, useState, useCallback, ReactNode } from "react";

export type Locale = "en" | "ru";

const translations = {
  en: {
    // Login
    "login.eyebrow": "Lenker Provider Panel",
    "login.title": "Admin access",
    "login.subtitle": "Sign in with the local admin created by make docker-bootstrap-admin.",
    "login.email": "Admin email",
    "login.password": "Password",
    "login.submit": "Sign in",
    "login.submitting": "Signing in...",
    "login.error.required": "Email and password are required.",
    "login.error.connection": "Unable to connect to panel-api.",
    "login.api_target": "API target",
    // Sidebar
    "sidebar.title": "Provider Panel",
    "nav.dashboard": "Dashboard",
    "nav.users": "Users",
    "nav.plans": "Plans",
    "nav.subscriptions": "Subscriptions",
    "nav.templates": "Templates",
    "nav.nodes": "Nodes",
    "nav.profiles": "Profiles",
    "nav.warp": "WARP",
    "nav.routing": "Routing",
    "nav.settings": "Settings",
    // Topbar
    "topbar.signed_in_as": "Signed in as",
    "topbar.sign_out": "Sign out",
    // Dashboard
    "dashboard.eyebrow": "MVP v0.1",
    "dashboard.title": "Dashboard shell is ready",
    "dashboard.description": "The React app authenticates against panel-api, stores the admin session for the current browser tab, and renders the first provider dashboard shell.",
    "dashboard.session_expires": "Session expires",
    "dashboard.backend_target": "Backend target",
    "dashboard.users": "Users",
    "dashboard.users_desc": "Create, edit, suspend, and activate users.",
    "dashboard.plans": "Plans",
    "dashboard.plans_desc": "Create, edit, and archive subscription plans.",
    "dashboard.subscriptions": "Subscriptions",
    "dashboard.subscriptions_desc": "Create, update, and renew subscriptions.",
    "dashboard.nodes": "Nodes",
    "dashboard.nodes_desc": "Bootstrap, inspect, drain, disable, and enable nodes.",
    "dashboard.live": "Live",
    // Session
    "session.expired": "Session expired. Sign in again.",
    "session.none": "No active session",
  },
  ru: {
    // Login
    "login.eyebrow": "Панель провайдера Lenker",
    "login.title": "Вход администратора",
    "login.subtitle": "Войдите с учётной записью администратора, созданной через make docker-bootstrap-admin.",
    "login.email": "Email администратора",
    "login.password": "Пароль",
    "login.submit": "Войти",
    "login.submitting": "Вход...",
    "login.error.required": "Email и пароль обязательны.",
    "login.error.connection": "Не удалось подключиться к panel-api.",
    "login.api_target": "API сервер",
    // Sidebar
    "sidebar.title": "Панель провайдера",
    "nav.dashboard": "Главная",
    "nav.users": "Пользователи",
    "nav.plans": "Тарифы",
    "nav.subscriptions": "Подписки",
    "nav.templates": "Шаблоны",
    "nav.nodes": "Ноды",
    "nav.profiles": "Профили",
    "nav.warp": "WARP",
    "nav.routing": "Маршрутизация",
    "nav.settings": "Настройки",
    // Topbar
    "topbar.signed_in_as": "Вы вошли как",
    "topbar.sign_out": "Выйти",
    // Dashboard
    "dashboard.eyebrow": "MVP v0.1",
    "dashboard.title": "Панель управления готова",
    "dashboard.description": "React-приложение аутентифицируется через panel-api, хранит сессию администратора и отображает панель провайдера.",
    "dashboard.session_expires": "Сессия истекает",
    "dashboard.backend_target": "Бэкенд",
    "dashboard.users": "Пользователи",
    "dashboard.users_desc": "Создание, редактирование, блокировка и активация пользователей.",
    "dashboard.plans": "Тарифы",
    "dashboard.plans_desc": "Создание, редактирование и архивация тарифных планов.",
    "dashboard.subscriptions": "Подписки",
    "dashboard.subscriptions_desc": "Создание, обновление и продление подписок.",
    "dashboard.nodes": "Ноды",
    "dashboard.nodes_desc": "Инициализация, просмотр, отключение и управление нодами.",
    "dashboard.live": "Активно",
    // Session
    "session.expired": "Сессия истекла. Войдите снова.",
    "session.none": "Нет активной сессии",
  },
} as const;

type TranslationKey = keyof typeof translations.en;

interface I18nContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: TranslationKey) => string;
}

const I18nContext = createContext<I18nContextValue | null>(null);

const LOCALE_KEY = "lenker_locale";

function getInitialLocale(): Locale {
  try {
    const stored = localStorage.getItem(LOCALE_KEY);
    if (stored === "ru" || stored === "en") return stored;
  } catch {}
  return navigator.language.startsWith("ru") ? "ru" : "en";
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(getInitialLocale);

  const setLocale = useCallback((l: Locale) => {
    setLocaleState(l);
    try { localStorage.setItem(LOCALE_KEY, l); } catch {}
  }, []);

  const t = useCallback((key: TranslationKey): string => {
    return translations[locale][key] ?? translations.en[key] ?? key;
  }, [locale]);

  return (
    <I18nContext.Provider value={{ locale, setLocale, t }}>
      {children}
    </I18nContext.Provider>
  );
}

export function useI18n() {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used within I18nProvider");
  return ctx;
}
