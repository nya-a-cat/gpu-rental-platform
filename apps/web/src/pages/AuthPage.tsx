import { useState, type FormEvent } from "react";
import { Link, Navigate, useLocation, useNavigate } from "react-router-dom";

import { readErrorMessage, useApp, useLocale } from "../app-context";
import {
  MechanicalPanel,
  StatusLamp,
  VentGrille,
} from "../components/mechanical";

interface LocationState {
  from?: string;
}

export function AuthPage({ kind }: { kind: "login" | "register" }) {
  const { gateway, login, register, user } = useApp();
  const { tr } = useLocale();
  const location = useLocation();
  const navigate = useNavigate();
  const destination = (location.state as LocationState | null)?.from || "/";
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [postAuthDestination, setPostAuthDestination] = useState(destination);

  if (user) return <Navigate replace to={postAuthDestination} />;

  async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const username = String(data.get("username") || "").trim();
    const password = String(data.get("password") || "");
    setPostAuthDestination(destination);
    setSubmitting(true);
    setError(null);
    try {
      if (kind === "register") await register({ username, password });
      else await login({ username, password });
      navigate(destination, { replace: true });
    } catch (reason) {
      setError(readErrorMessage(reason));
    } finally {
      setSubmitting(false);
    }
  }

  async function enterDemo(username: "dispatcher" | "operator"): Promise<void> {
    const target = username === "dispatcher" ? "/admin" : destination;
    setPostAuthDestination(target);
    setSubmitting(true);
    setError(null);
    try {
      await login({ username, password: "demo-password" });
      navigate(target, { replace: true });
    } catch (reason) {
      setError(readErrorMessage(reason));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="page-frame auth-page">
      <MechanicalPanel className="auth-story" eyebrow="ACCESS RELAY / 03">
        <div className="auth-story__content">
          <span className="serial-label">SESSION CONTROL</span>
          <h1>
            {tr("把身份接入", "Connect identity to")}
            <span>{tr("正确的操作台。", "the right console.")}</span>
          </h1>
          <p>
            {tr(
              "普通用户可预订和退租，管理员可管理资源上下架与全部订单。API 模式使用 HttpOnly 服务端会话。",
              "Operators reserve and return resources. Administrators manage listings and all orders. API mode uses HttpOnly server-side sessions.",
            )}
          </p>
          <div className="archive-strip">
            <figure>
              <img
                alt={tr(
                  "1976 年 NASA 控制室仪表墙",
                  "1976 NASA control-room instrument wall",
                )}
                decoding="async"
                loading="lazy"
                referrerPolicy="no-referrer"
                src="https://upload.wikimedia.org/wikipedia/commons/thumb/8/8d/INSTRUMENT_PANELS_IN_CONTROL_ROOM_-_NARA_-_17447770.jpg/1280px-INSTRUMENT_PANELS_IN_CONTROL_ROOM_-_NARA_-_17447770.jpg"
              />
              <figcaption>
                <a
                  href="https://commons.wikimedia.org/wiki/File:INSTRUMENT_PANELS_IN_CONTROL_ROOM_-_NARA_-_17447770.jpg"
                  rel="noreferrer"
                  target="_blank"
                >
                  Martin Brown / NASA / NARA
                </a>
                <span>PUBLIC DOMAIN (US)</span>
              </figcaption>
            </figure>
          </div>
          <div className="relay-diagram" aria-hidden="true">
            <span>IDENTITY</span>
            <i />
            <span>SESSION</span>
            <i />
            <span>ROLE</span>
          </div>
          <VentGrille
            label={tr("装饰性接入格栅", "Decorative access grille")}
          />
        </div>
      </MechanicalPanel>

      <MechanicalPanel
        className="auth-form-panel"
        eyebrow={kind === "login" ? "LOGIN / A" : "REGISTER / B"}
        title={
          kind === "login"
            ? tr("身份验证", "Identity check")
            : tr("创建操作员", "Create operator")
        }
      >
        <StatusLamp
          label={
            gateway.mode === "demo"
              ? tr("浏览器演示身份", "BROWSER DEMO IDENTITY")
              : tr("服务端会话", "SERVER SESSION")
          }
          tone={gateway.mode === "demo" ? "warn" : "good"}
        />
        <form className="stack-form" onSubmit={(event) => void submit(event)}>
          <label>
            <span>{tr("用户名", "Username")}</span>
            <input
              autoComplete="username"
              maxLength={32}
              minLength={3}
              name="username"
              pattern="[A-Za-z0-9_-]+"
              required
            />
          </label>
          <label>
            <span>{tr("密码", "Password")}</span>
            <input
              autoComplete={
                kind === "login" ? "current-password" : "new-password"
              }
              maxLength={72}
              minLength={kind === "login" ? 1 : 8}
              name="password"
              required
              type="password"
            />
          </label>
          {error ? (
            <div className="inline-error" role="alert">
              {error}
            </div>
          ) : null}
          <button
            className="button button--orange button--wide"
            disabled={submitting}
            type="submit"
          >
            {submitting
              ? tr("正在校验…", "Checking…")
              : kind === "login"
                ? tr("接入控制台", "Connect console")
                : tr("注册并接入", "Register & connect")}
          </button>
        </form>
        <p className="auth-switch">
          {kind === "login"
            ? tr("还没有身份？", "Need an identity?")
            : tr("已经注册？", "Already registered?")}{" "}
          <Link to={kind === "login" ? "/register" : "/login"}>
            {kind === "login"
              ? tr("创建操作员", "Create operator")
              : tr("返回登录", "Back to login")}
          </Link>
        </p>

        {gateway.mode === "demo" && kind === "login" ? (
          <div className="demo-identity-bank">
            <div>
              <span>{tr("快速演示身份", "QUICK DEMO IDENTITY")}</span>
              <small>
                {tr(
                  "固定密码 demo-password，仅用于当前浏览器演示。",
                  "Fixed password demo-password, only for this browser demo.",
                )}
              </small>
            </div>
            <button
              disabled={submitting}
              onClick={() => void enterDemo("operator")}
              type="button"
            >
              <strong>OPERATOR</strong>
              <span>{tr("普通用户流程", "User workflow")}</span>
            </button>
            <button
              disabled={submitting}
              onClick={() => void enterDemo("dispatcher")}
              type="button"
            >
              <strong>DISPATCHER</strong>
              <span>{tr("管理员流程", "Admin workflow")}</span>
            </button>
          </div>
        ) : null}
      </MechanicalPanel>
    </div>
  );
}
