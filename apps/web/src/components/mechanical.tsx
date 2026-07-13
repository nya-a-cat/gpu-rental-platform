import type { PropsWithChildren, ReactNode } from "react";

import { useLocale } from "../app-context";
import { clampPercentage } from "../format";

export function MechanicalPanel({
  children,
  className = "",
  eyebrow,
  title,
}: PropsWithChildren<{
  className?: string;
  eyebrow?: string;
  title?: string;
}>) {
  return (
    <section className={`mechanical-panel ${className}`.trim()}>
      <span className="panel-screw panel-screw--tl" aria-hidden="true" />
      <span className="panel-screw panel-screw--tr" aria-hidden="true" />
      <span className="panel-screw panel-screw--bl" aria-hidden="true" />
      <span className="panel-screw panel-screw--br" aria-hidden="true" />
      {title ? (
        <header className="panel-header">
          {eyebrow ? <span className="panel-eyebrow">{eyebrow}</span> : null}
          <h2>{title}</h2>
        </header>
      ) : null}
      {children}
    </section>
  );
}

export function AnalogGauge({
  display,
  label,
  value,
}: {
  display: string;
  label: string;
  value: number;
}) {
  const percentage = clampPercentage(value);
  const angle = -58 + (percentage / 100) * 116;
  const ticks = Array.from({ length: 9 }, (_, index) => -60 + index * 15);

  return (
    <div
      className="analog-gauge"
      role="meter"
      aria-label={label}
      aria-valuemax={100}
      aria-valuemin={0}
      aria-valuenow={percentage}
      aria-valuetext={display}
    >
      <svg viewBox="0 0 140 92" aria-hidden="true">
        <path className="gauge-track" d="M 20 72 A 50 50 0 0 1 120 72" />
        {ticks.map((tick) => (
          <line
            className="gauge-tick"
            key={tick}
            x1="70"
            x2="70"
            y1="18"
            y2="25"
            transform={`rotate(${tick} 70 72)`}
          />
        ))}
        <line
          className="gauge-needle"
          x1="70"
          x2="70"
          y1="72"
          y2="31"
          transform={`rotate(${angle} 70 72)`}
        />
        <circle className="gauge-pin" cx="70" cy="72" r="6" />
      </svg>
      <span className="gauge-display">{display}</span>
      <span className="gauge-label">{label}</span>
    </div>
  );
}

export function StatusLamp({
  label,
  tone = "neutral",
}: {
  label: string;
  tone?: "danger" | "good" | "neutral" | "warn";
}) {
  return (
    <span className={`status-lamp status-lamp--${tone}`}>
      <span aria-hidden="true" />
      {label}
    </span>
  );
}

export function MetricTile({
  label,
  value,
}: {
  label: string;
  value: ReactNode;
}) {
  return (
    <div className="metric-tile">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export function VentGrille({ label }: { label: string }) {
  return (
    <div className="vent-grille" aria-label={label} role="img">
      {Array.from({ length: 7 }, (_, index) => (
        <span key={index} aria-hidden="true" />
      ))}
    </div>
  );
}

export function RotaryControl({
  disabled = false,
  label,
  onChange,
  position,
  value,
}: {
  disabled?: boolean;
  label: string;
  onChange(direction: 1 | -1): void;
  position: number;
  value: string;
}) {
  return (
    <button
      aria-label={`${label}: ${value}`}
      aria-keyshortcuts="ArrowLeft ArrowRight ArrowUp ArrowDown"
      className="rotary-control"
      data-position={position}
      disabled={disabled}
      onClick={() => onChange(1)}
      onKeyDown={(event) => {
        if (event.key === "ArrowRight" || event.key === "ArrowUp") {
          event.preventDefault();
          onChange(1);
        }
        if (event.key === "ArrowLeft" || event.key === "ArrowDown") {
          event.preventDefault();
          onChange(-1);
        }
      }}
      type="button"
    >
      <span className="rotary-control__knob" aria-hidden="true">
        <span />
      </span>
      <span className="rotary-control__label">{label}</span>
      <strong aria-live="polite">{value}</strong>
    </button>
  );
}

export function LoadState({ label }: { label?: string }) {
  const { tr } = useLocale();
  return (
    <div className="state-box" aria-live="polite" aria-busy="true">
      <span className="state-spinner" aria-hidden="true" />
      <strong>{label ?? tr("正在校准控制台", "Calibrating console")}</strong>
      <span>{tr("正在读取资源状态…", "Reading resource state…")}</span>
    </div>
  );
}

export function EmptyState({
  action,
  message,
  title,
}: {
  action?: ReactNode;
  message: string;
  title: string;
}) {
  return (
    <div className="state-box">
      <span className="state-code">000</span>
      <strong>{title}</strong>
      <span>{message}</span>
      {action}
    </div>
  );
}

export function ErrorState({
  message,
  retry,
}: {
  message: string;
  retry?: () => void;
}) {
  const { tr } = useLocale();
  return (
    <div className="state-box state-box--error" role="alert">
      <span className="state-code">ERR</span>
      <strong>{tr("线路响应异常", "Control line error")}</strong>
      <span>{message}</span>
      {retry ? (
        <button className="button button--dark" onClick={retry} type="button">
          {tr("重新连接", "Retry")}
        </button>
      ) : null}
    </div>
  );
}

export function Pagination({
  onChange,
  page,
  pageSize,
  total,
}: {
  onChange(nextPage: number): void;
  page: number;
  pageSize: number;
  total: number;
}) {
  const { tr } = useLocale();
  const pages = Math.max(1, Math.ceil(total / pageSize));
  if (pages <= 1) return null;
  return (
    <nav className="pagination" aria-label={tr("分页", "Pagination")}>
      <button
        className="button button--quiet"
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
        type="button"
      >
        {tr("上一页", "Previous")}
      </button>
      <span>
        {page.toString().padStart(2, "0")} / {pages.toString().padStart(2, "0")}
      </span>
      <button
        className="button button--quiet"
        disabled={page >= pages}
        onClick={() => onChange(page + 1)}
        type="button"
      >
        {tr("下一页", "Next")}
      </button>
    </nav>
  );
}
