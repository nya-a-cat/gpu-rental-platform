import {
  GpuAvailability,
  type GpuResourceView,
  type OrderView,
} from "@gpu-rental/contracts";
import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";

import { readErrorMessage, useApp, useLocale } from "../app-context";
import gpuModuleCutaway from "../assets/generated/gpu-module-cutaway.webp";
import {
  AnalogGauge,
  ErrorState,
  LoadState,
  MechanicalPanel,
  StatusLamp,
  VentGrille,
} from "../components/mechanical";
import { formatMoney } from "../format";
import { availabilityLabel, statusTone } from "../labels";

export function ResourcePage() {
  const { gateway, user } = useApp();
  const { locale, tr } = useLocale();
  const { resourceId = "" } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const [resource, setResource] = useState<GpuResourceView | null>(null);
  const [durationHours, setDurationHours] = useState(8);
  const [order, setOrder] = useState<OrderView | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    void gateway
      .getResource(resourceId)
      .then((result) => {
        if (active) setResource(result);
      })
      .catch((reason: unknown) => {
        if (active) setError(readErrorMessage(reason));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [gateway, resourceId, revision]);

  async function reserve(): Promise<void> {
    if (!resource) return;
    if (!user) {
      navigate("/login", { state: { from: location.pathname } });
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const created = await gateway.createOrder({
        gpuResourceId: resource.id,
        durationHours,
      });
      setOrder(created);
      setResource({ ...resource, availability: GpuAvailability.Rented });
    } catch (reason) {
      setError(readErrorMessage(reason));
    } finally {
      setSubmitting(false);
    }
  }

  if (loading) {
    return (
      <div className="page-frame page-frame--narrow">
        <LoadState label={tr("正在读取资源铭牌", "Reading resource plate")} />
      </div>
    );
  }
  if (error && !resource) {
    return (
      <div className="page-frame page-frame--narrow">
        <ErrorState
          message={error}
          retry={() => setRevision((value) => value + 1)}
        />
      </div>
    );
  }
  if (!resource) return null;

  const available = resource.availability === GpuAvailability.Available;
  const totalPrice = resource.hourlyPriceCents * durationHours;

  return (
    <div className="page-frame detail-page">
      <div className="breadcrumb-row">
        <Link to="/">{tr("算力市场", "Market")}</Link>
        <span>/</span>
        <span>{resource.name}</span>
      </div>
      <div className="detail-grid">
        <MechanicalPanel
          className="detail-identity"
          eyebrow="EQUIPMENT PLATE / 02"
        >
          <div className="detail-serial">
            <span>RESOURCE ID</span>
            <code>{resource.id}</code>
          </div>
          <div className="detail-heading">
            <div>
              <span>{resource.name}</span>
              <h1>{resource.model}</h1>
            </div>
            <StatusLamp
              label={availabilityLabel(resource.availability, tr)}
              tone={statusTone(resource.availability)}
            />
          </div>
          <div className="large-schematic">
            <div
              className="module-diagram"
              aria-label={tr("GPU 模块结构示意", "GPU module schematic")}
              role="img"
            >
              <div className="module-ports" aria-hidden="true">
                <span />
                <span />
                <span />
                <span />
              </div>
              <div className="module-observation">
                <img alt="" aria-hidden="true" src={gpuModuleCutaway} />
                <span>MODULE VIEW / 04</span>
              </div>
              <div className="module-memory-bank" aria-hidden="true">
                <span />
                <span />
                <span />
                <span />
                <span />
                <span />
              </div>
              <div className="module-bus" aria-hidden="true" />
            </div>
            <div className="schematic-sidecar">
              <div className="schematic-caption">
                <span>FICTIONAL MODULE / ORIGINAL ARTWORK</span>
                <strong>{resource.memoryGb} GB</strong>
              </div>
              <VentGrille
                label={tr("装饰性设备格栅", "Decorative equipment grille")}
              />
            </div>
          </div>
          <dl className="detail-specs">
            <div>
              <dt>{tr("显存容量", "Memory capacity")}</dt>
              <dd>{resource.memoryGb} GB</dd>
            </div>
            <div>
              <dt>{tr("资源区域", "Region")}</dt>
              <dd>{resource.region}</dd>
            </div>
            <div>
              <dt>{tr("小时单价", "Hourly rate")}</dt>
              <dd>{formatMoney(resource.hourlyPriceCents, locale)}</dd>
            </div>
            <div>
              <dt>{tr("资源性质", "Resource mode")}</dt>
              <dd>SIMULATED</dd>
            </div>
          </dl>
          <div className="tag-row">
            {resource.tags.map((tag) => (
              <span key={tag}>{tag}</span>
            ))}
          </div>
          <p className="truth-note">
            {tr(
              "该条目用于验证订单流程，不代表一台可远程连接的实体 GPU。平台不会生成 SSH、Notebook、IP、温度或利用率信息。",
              "This item validates the order workflow; it is not a remotely accessible physical GPU. The platform does not invent SSH, notebook, IP, temperature or utilization data.",
            )}
          </p>
        </MechanicalPanel>

        <MechanicalPanel
          className="reservation-panel"
          eyebrow="ORDER DIAL / B"
          title={tr("预订控制器", "Reservation controller")}
        >
          <AnalogGauge
            display={`${durationHours} h`}
            label={tr("租用时长", "DURATION")}
            value={(durationHours / 720) * 100}
          />
          <label className="range-control">
            <span>
              {tr("租用时长", "Duration")}
              <strong>{durationHours} h</strong>
            </span>
            <input
              disabled={!available || Boolean(order)}
              max="720"
              min="1"
              onChange={(event) => setDurationHours(Number(event.target.value))}
              type="range"
              value={durationHours}
            />
          </label>
          <div
            className="duration-presets"
            aria-label={tr("常用租期", "Common durations")}
          >
            {[1, 8, 24, 72, 168].map((hours) => (
              <button
                aria-pressed={durationHours === hours}
                disabled={!available || Boolean(order)}
                key={hours}
                onClick={() => setDurationHours(hours)}
                type="button"
              >
                {hours}h
              </button>
            ))}
          </div>
          <div className="order-calculation">
            <span>{tr("小时单价", "Hourly rate")}</span>
            <strong>{formatMoney(resource.hourlyPriceCents, locale)}</strong>
            <span>{tr("预计总价", "Estimated total")}</span>
            <strong>{formatMoney(totalPrice, locale)}</strong>
          </div>
          {error ? (
            <div className="inline-error" role="alert">
              {error}
            </div>
          ) : null}
          {order ? (
            <div className="success-ticket" role="status">
              <span>ORDER ACCEPTED</span>
              <strong>{order.id}</strong>
              <p>
                {tr(
                  "预订已写入订单列表。",
                  "Reservation added to your order list.",
                )}
              </p>
              <Link className="button button--orange" to="/orders">
                {tr("查看我的订单", "View my orders")}
              </Link>
            </div>
          ) : (
            <button
              className="button button--orange button--wide"
              disabled={submitting || !available}
              onClick={() => void reserve()}
              type="button"
            >
              {submitting
                ? tr("正在锁定资源…", "Locking resource…")
                : available
                  ? user
                    ? tr("确认预订", "Confirm reservation")
                    : tr("登录后预订", "Sign in to reserve")
                  : tr("资源正在租用", "Resource is rented")}
            </button>
          )}
          <p className="fine-print">
            {tr(
              "真实 API 模式使用 Redis 资源锁与数据库唯一约束防止重复分配。",
              "API mode uses a Redis resource lock plus a database uniqueness constraint to prevent double allocation.",
            )}
          </p>
        </MechanicalPanel>
      </div>
    </div>
  );
}
