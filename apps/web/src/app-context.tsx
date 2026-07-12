import type {
  LoginInput,
  RegisterInput,
  UserView,
} from "@gpu-rental/contracts";
import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type PropsWithChildren,
} from "react";

import { createGateway, type DataGateway } from "./data/gateway";

type Locale = "en" | "zh";

interface LocaleContextValue {
  locale: Locale;
  setLocale(locale: Locale): void;
  tr(zh: string, en: string): string;
}

interface AppContextValue {
  gateway: DataGateway;
  sessionError: string | null;
  sessionLoading: boolean;
  user: UserView | null;
  login(input: LoginInput): Promise<UserView>;
  logout(all?: boolean): Promise<void>;
  refreshSession(): Promise<void>;
  register(input: RegisterInput): Promise<UserView>;
  resetDemo(): Promise<void>;
}

const LocaleContext = createContext<LocaleContextValue | null>(null);
const AppContext = createContext<AppContextValue | null>(null);

export function AppProviders({
  children,
  gateway: gatewayOverride,
}: PropsWithChildren<{ gateway?: DataGateway }>) {
  const gateway = useMemo(
    () => gatewayOverride ?? createGateway(),
    [gatewayOverride],
  );
  const [locale, setLocaleState] = useState<Locale>(() => {
    const saved = window.localStorage.getItem("gpu-rental-locale");
    return saved === "en" ? "en" : "zh";
  });
  const [user, setUser] = useState<UserView | null>(null);
  const [sessionLoading, setSessionLoading] = useState(true);
  const [sessionError, setSessionError] = useState<string | null>(null);

  async function refreshSession(): Promise<void> {
    setSessionLoading(true);
    setSessionError(null);
    try {
      setUser(await gateway.getSession());
    } catch (error) {
      setSessionError(readErrorMessage(error));
    } finally {
      setSessionLoading(false);
    }
  }

  useEffect(() => {
    void refreshSession();
  }, [gateway]);

  function setLocale(nextLocale: Locale): void {
    setLocaleState(nextLocale);
    window.localStorage.setItem("gpu-rental-locale", nextLocale);
    document.documentElement.lang = nextLocale === "zh" ? "zh-CN" : "en";
  }

  const localeValue = useMemo<LocaleContextValue>(
    () => ({
      locale,
      setLocale,
      tr: (zh, en) => (locale === "zh" ? zh : en),
    }),
    [locale],
  );

  const appValue = useMemo<AppContextValue>(
    () => ({
      gateway,
      sessionError,
      sessionLoading,
      user,
      async login(input) {
        const response = await gateway.login(input);
        setUser(response.user);
        return response.user;
      },
      async register(input) {
        const response = await gateway.register(input);
        setUser(response.user);
        return response.user;
      },
      async logout(all = false) {
        if (all) await gateway.logoutAll();
        else await gateway.logout();
        setUser(null);
      },
      refreshSession,
      async resetDemo() {
        await gateway.resetDemo();
        setUser(null);
      },
    }),
    [gateway, sessionError, sessionLoading, user],
  );

  return (
    <LocaleContext.Provider value={localeValue}>
      <AppContext.Provider value={appValue}>{children}</AppContext.Provider>
    </LocaleContext.Provider>
  );
}

export function useApp(): AppContextValue {
  const value = useContext(AppContext);
  if (!value) throw new Error("useApp must be used inside AppProviders");
  return value;
}

export function useLocale(): LocaleContextValue {
  const value = useContext(LocaleContext);
  if (!value) throw new Error("useLocale must be used inside AppProviders");
  return value;
}

export function readErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Unknown request error";
}
