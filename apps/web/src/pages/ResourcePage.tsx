import {
  GpuAvailability,
  type EnvironmentTemplateView,
  type GpuResourceView,
  type OrderView,
  type TeamView,
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
  const [templates, setTemplates] = useState<EnvironmentTemplateView[]>([]);
  const [teams, setTeams] = useState<TeamView[]>([]);
  const [templateId, setTemplateId] = useState("pytorch-jupyter");
  const [projectId, setProjectId] = useState("");
  const [instanceName, setInstanceName] = useState("");
  const [order, setOrder] = useState<OrderView | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    void Promise.all([
      gateway.getResource(resourceId),
      gateway.listEnvironmentTemplates(),
      user ? gateway.listTeams() : Promise.resolve([]),
    ])
      .then(([nextResource, nextTemplates, nextTeams]) => {
        if (!active) return;
        setResource(nextResource);
        setTemplates(nextTemplates);
        setTeams(nextTeams);
        setInstanceName(
          (current) => current || `${nextResource.name} workload`,
        );
        setTemplateId((current) =>
          nextTemplates.some((template) => template.id === current)
            ? current
            : (nextTemplates[0]?.id ?? ""),
        );
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
  }, [gateway, resourceId, revision, user]);

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
        environmentTemplateId: templateId,
        instanceName,
        projectId: projectId || undefined,
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
              <dd>
                {resource.gpuCount} × {resource.memoryGb} GB
              </dd>
            </div>
            <div>
              <dt>{tr("主机配置", "Host allocation")}</dt>
              <dd>
                {resource.cpuCores} vCPU · {resource.systemMemoryGb} GB RAM
              </dd>
            </div>
            <div>
              <dt>{tr("本地磁盘", "Local storage")}</dt>
              <dd>{resource.storageGb} GB</dd>
            </div>
            <div>
              <dt>CUDA / DRIVER</dt>
              <dd>
                {resource.cudaVersion} / {resource.driverVersion}
              </dd>
            </div>
            <div>
              <dt>{tr("网络带宽", "Network bandwidth")}</dt>
              <dd>{resource.bandwidthMbps} Mbps</dd>
            </div>
            <div>
              <dt>{tr("演示可靠性", "Demo reliability")}</dt>
              <dd>{resource.reliabilityPercent}%</dd>
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
              "该条目用于验证完整租用流程。实例连接入口使用 .invalid 保留域名，仅展示交付界面，不会连接实体 GPU。",
              "This item validates the complete rental workflow. Instance access uses reserved .invalid domains to demonstrate delivery without connecting to physical GPUs.",
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
          <label className="stack-field">
            <span>{tr("实例名称", "Instance name")}</span>
            <input
              disabled={!available || Boolean(order)}
              maxLength={80}
              minLength={2}
              onChange={(event) => setInstanceName(event.target.value)}
              required
              value={instanceName}
            />
          </label>
          <label className="stack-field">
            <span>{tr("运行环境", "Environment template")}</span>
            <select
              disabled={!available || Boolean(order)}
              onChange={(event) => setTemplateId(event.target.value)}
              value={templateId}
            >
              {templates.map((template) => (
                <option key={template.id} value={template.id}>
                  {template.name}
                </option>
              ))}
            </select>
          </label>
          {templates.find((template) => template.id === templateId) ? (
            <div className="template-summary">
              <strong>
                {
                  templates.find((template) => template.id === templateId)!
                    .image
                }
              </strong>
              <span>
                {
                  templates.find((template) => template.id === templateId)!
                    .description
                }
              </span>
            </div>
          ) : null}
          {teams.some((team) => team.projects.length > 0) ? (
            <label className="stack-field">
              <span>{tr("成本归属项目", "Cost attribution project")}</span>
              <select
                disabled={!available || Boolean(order)}
                onChange={(event) => setProjectId(event.target.value)}
                value={projectId}
              >
                <option value="">{tr("个人账户", "Personal account")}</option>
                {teams.flatMap((team) =>
                  team.projects.map((project) => (
                    <option key={project.id} value={project.id}>
                      {team.name} / {project.name}
                    </option>
                  )),
                )}
              </select>
            </label>
          ) : null}
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
                  "预订已写入订单列表，模拟实例已开始运行。",
                  "Reservation recorded and the simulated instance is running.",
                )}
              </p>
              <Link className="button button--orange" to="/instances">
                {tr("管理实例", "Manage instance")}
              </Link>
            </div>
          ) : (
            <button
              className="button button--orange button--wide"
              disabled={
                submitting ||
                !available ||
                !templateId ||
                instanceName.trim().length < 2
              }
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
