import { UserRole } from "@gpu-rental/contracts";
import { useEffect } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";

import { useApp, useLocale } from "../app-context";
import { StatusLamp, VentGrille } from "./mechanical";

export function AppLayout() {
  const { gateway, logout, resetDemo, sessionError, sessionLoading, user } =
    useApp();
  const { locale, setLocale, tr } = useLocale();
  const location = useLocation();
  const navigate = useNavigate();

  useEffect(() => {
    window.scrollTo({ left: 0, top: 0 });
  }, [location.pathname]);

  async function handleLogout(all = false): Promise<void> {
    await logout(all);
    navigate("/");
  }

  async function handleReset(): Promise<void> {
    const confirmed = window.confirm(
      tr(
        "重置将清除当前浏览器中的演示订单和身份，是否继续？",
        "Reset clears demo orders and identity in this browser. Continue?",
      ),
    );
    if (!confirmed) return;
    await resetDemo();
    navigate("/");
  }

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        {tr("跳到主要内容", "Skip to main content")}
      </a>
      <header className="topbar">
        <NavLink className="brand" to="/" aria-label="Kiloworks home">
          <span className="brand-mark" aria-hidden="true">
            KW
          </span>
          <span>
            <strong>KILOWORKS</strong>
            <small>GPU CONTROL EXCHANGE</small>
          </span>
        </NavLink>
        <nav
          className="primary-nav"
          aria-label={tr("主导航", "Main navigation")}
        >
          <NavLink to="/" end>
            {tr("算力市场", "Market")}
          </NavLink>
          {user ? (
            <>
              <NavLink to="/instances">{tr("我的实例", "Instances")}</NavLink>
              <NavLink to="/orders">{tr("我的订单", "Orders")}</NavLink>
              <NavLink to="/cloud-account">
                {tr("云账户", "Cloud account")}
              </NavLink>
            </>
          ) : null}
          {user?.role === UserRole.Admin ? (
            <NavLink to="/admin">{tr("调度后台", "Admin")}</NavLink>
          ) : null}
        </nav>
        <div
          className={`runtime-status runtime-status--${gateway.mode}`}
          role="status"
        >
          <StatusLamp
            label={
              gateway.mode === "demo"
                ? tr("沙盒库存", "SANDBOX INVENTORY")
                : tr("服务端在线", "SERVER LINK")
            }
            tone={gateway.mode === "demo" ? "warn" : "good"}
          />
          <span>
            {gateway.mode === "demo"
              ? tr("浏览器本地流程", "BROWSER-LOCAL WORKFLOW")
              : tr("真实会话与订单", "LIVE SESSION & ORDERS")}
          </span>
          {gateway.mode === "demo" ? (
            <button onClick={() => void handleReset()} type="button">
              {tr("归零", "RESET")}
            </button>
          ) : null}
        </div>
        <div className="topbar-actions">
          <div className="locale-toggle" aria-label={tr("语言", "Language")}>
            <button
              aria-pressed={locale === "zh"}
              onClick={() => setLocale("zh")}
              type="button"
            >
              中
            </button>
            <button
              aria-pressed={locale === "en"}
              onClick={() => setLocale("en")}
              type="button"
            >
              EN
            </button>
          </div>
          {sessionLoading ? (
            <span className="identity-chip">...</span>
          ) : user ? (
            <details className="identity-menu">
              <summary>
                <span className="identity-light" aria-hidden="true" />
                {user.username}
              </summary>
              <div>
                <span>
                  {user.role === UserRole.Admin ? "ADMIN" : "OPERATOR"}
                </span>
                <button onClick={() => void handleLogout(false)} type="button">
                  {tr("退出当前会话", "Sign out")}
                </button>
                <button onClick={() => void handleLogout(true)} type="button">
                  {tr("撤销全部会话", "Revoke all sessions")}
                </button>
              </div>
            </details>
          ) : (
            <NavLink
              className="button button--orange button--small"
              to="/login"
            >
              {tr("接入控制台", "Sign in")}
            </NavLink>
          )}
        </div>
      </header>

      {sessionError ? (
        <div className="session-warning" role="alert">
          {tr("无法确认当前会话：", "Could not verify session: ")}
          {sessionError}
        </div>
      ) : null}

      <main id="main-content" tabIndex={-1}>
        <Outlet />
      </main>

      <footer className="footer">
        <div>
          <strong>KILOWORKS / 2026</strong>
          <span>
            {tr(
              "公开演示与真实后端严格分离",
              "Public demo and real backend are explicitly separated",
            )}
          </span>
        </div>
        <VentGrille label={tr("装饰性散热格栅", "Decorative vent grille")} />
        <span>REACT · NESTJS · MONGODB · REDIS</span>
      </footer>
    </div>
  );
}
