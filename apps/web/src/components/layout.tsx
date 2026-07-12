import { UserRole } from "@gpu-rental/contracts";
import { NavLink, Outlet, useNavigate } from "react-router-dom";

import { useApp, useLocale } from "../app-context";
import { StatusLamp, VentGrille } from "./mechanical";

export function AppLayout() {
  const { gateway, logout, resetDemo, sessionError, sessionLoading, user } =
    useApp();
  const { locale, setLocale, tr } = useLocale();
  const navigate = useNavigate();

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
            <NavLink to="/orders">{tr("我的订单", "Orders")}</NavLink>
          ) : null}
          {user?.role === UserRole.Admin ? (
            <NavLink to="/admin">{tr("调度后台", "Admin")}</NavLink>
          ) : null}
        </nav>
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

      <div className={`mode-banner mode-banner--${gateway.mode}`} role="status">
        <StatusLamp
          label={
            gateway.mode === "demo"
              ? tr("演示数据 · 模拟库存", "Demo data · Simulated inventory")
              : tr("API 模式 · 服务端会话", "API mode · Server session")
          }
          tone={gateway.mode === "demo" ? "warn" : "good"}
        />
        <span>
          {gateway.mode === "demo"
            ? tr(
                "操作仅保存在当前浏览器，不会分配实体 GPU 或产生费用。",
                "Changes stay in this browser. No physical GPU is allocated or billed.",
              )
            : tr(
                "资源仍为模拟资产；订单、并发锁和会话由真实后端处理。",
                "Assets remain simulated; orders, locks and sessions use the real backend.",
              )}
        </span>
        {gateway.mode === "demo" ? (
          <button onClick={() => void handleReset()} type="button">
            {tr("重置演示", "Reset demo")}
          </button>
        ) : null}
      </div>

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
