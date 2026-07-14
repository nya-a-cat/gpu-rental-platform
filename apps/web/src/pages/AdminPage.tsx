import {
  GpuListingStatus,
  OrderStatus,
  type AdminOverview,
  type GpuResourceView,
  type OrderView,
} from "@gpu-rental/contracts";
import { useEffect, useState, type FormEvent } from "react";

import { readErrorMessage, useApp, useLocale } from "../app-context";
import {
  EmptyState,
  ErrorState,
  LoadState,
  MechanicalPanel,
  MetricTile,
  StatusLamp,
} from "../components/mechanical";
import { formatDate, formatMoney } from "../format";
import { listingLabel, orderStatusLabel, statusTone } from "../labels";

type AdminTab = "inventory" | "orders" | "overview";

export function AdminPage() {
  const { gateway } = useApp();
  const { locale, tr } = useLocale();
  const [tab, setTab] = useState<AdminTab>("overview");
  const [overview, setOverview] = useState<AdminOverview | null>(null);
  const [resources, setResources] = useState<GpuResourceView[]>([]);
  const [orders, setOrders] = useState<OrderView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);

  const onlineRatio = overview?.resourcesTotal
    ? Math.round((overview.resourcesOnline / overview.resourcesTotal) * 100)
    : 0;

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    void Promise.all([
      gateway.getAdminOverview(),
      gateway.listAdminResources({ pageSize: 100 }),
      gateway.listAdminOrders({ pageSize: 100 }),
    ])
      .then(([nextOverview, resourcePage, orderPage]) => {
        if (!active) return;
        setOverview(nextOverview);
        setResources(resourcePage.items);
        setOrders(orderPage.items);
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
  }, [gateway, revision]);

  async function changeListing(
    resource: GpuResourceView,
    listingStatus: GpuListingStatus,
  ): Promise<void> {
    setPending(resource.id);
    setError(null);
    try {
      await gateway.setListingStatus(resource.id, { listingStatus });
      setRevision((value) => value + 1);
    } catch (reason) {
      setError(readErrorMessage(reason));
    } finally {
      setPending(null);
    }
  }

  async function createResource(
    event: FormEvent<HTMLFormElement>,
  ): Promise<void> {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    setPending("create");
    setError(null);
    try {
      await gateway.createResource({
        name: String(data.get("name") || "").trim(),
        model: String(data.get("model") || "").trim(),
        memoryGb: Number(data.get("memoryGb")),
        gpuCount: Number(data.get("gpuCount")),
        cpuCores: Number(data.get("cpuCores")),
        systemMemoryGb: Number(data.get("systemMemoryGb")),
        storageGb: Number(data.get("storageGb")),
        cudaVersion: String(data.get("cudaVersion") || "").trim(),
        driverVersion: String(data.get("driverVersion") || "").trim(),
        bandwidthMbps: Number(data.get("bandwidthMbps")),
        reliabilityPercent: Number(data.get("reliabilityPercent")),
        region: String(data.get("region") || "").trim(),
        hourlyPriceCents: Math.round(Number(data.get("hourlyPrice")) * 100),
        tags: String(data.get("tags") || "")
          .split(",")
          .map((tag) => tag.trim())
          .filter(Boolean),
        listingStatus: GpuListingStatus.Offline,
      });
      form.reset();
      setRevision((value) => value + 1);
    } catch (reason) {
      setError(readErrorMessage(reason));
    } finally {
      setPending(null);
    }
  }

  async function cancelOrder(orderId: string): Promise<void> {
    setPending(orderId);
    setError(null);
    try {
      await gateway.cancelOrder(orderId);
      setRevision((value) => value + 1);
    } catch (reason) {
      setError(readErrorMessage(reason));
    } finally {
      setPending(null);
    }
  }

  return (
    <div className="page-frame admin-page">
      <header className="page-title-row">
        <div>
          <span className="serial-label">DISPATCH OFFICE / PANEL 05</span>
          <h1>{tr("资源调度后台", "Dispatch administration")}</h1>
          <p>
            {tr(
              "管理模拟资产上下架、订单状态与平台业务汇总。",
              "Manage simulated listings, order states and business totals.",
            )}
          </p>
        </div>
        <StatusLamp label="ADMIN ACCESS" tone="good" />
      </header>
      <nav className="tab-bank" aria-label={tr("后台页面", "Admin sections")}>
        {(["overview", "inventory", "orders"] as const).map((value) => (
          <button
            aria-current={tab === value ? "page" : undefined}
            key={value}
            onClick={() => setTab(value)}
            type="button"
          >
            {value === "overview"
              ? tr("业务概览", "Overview")
              : value === "inventory"
                ? tr("资源管理", "Inventory")
                : tr("订单管理", "Orders")}
          </button>
        ))}
      </nav>
      {loading ? <LoadState /> : null}
      {!loading && error ? (
        <ErrorState
          message={error}
          retry={() => setRevision((value) => value + 1)}
        />
      ) : null}
      {!loading && overview && tab === "overview" ? (
        <section className="admin-overview">
          <div className="admin-metrics">
            <MetricTile
              label={tr("用户", "USERS")}
              value={overview.usersTotal}
            />
            <MetricTile
              label={tr("资源总数", "RESOURCES")}
              value={overview.resourcesTotal}
            />
            <MetricTile
              label={tr("已上架", "ONLINE")}
              value={overview.resourcesOnline}
            />
            <MetricTile
              label={tr("生效订单", "ACTIVE ORDERS")}
              value={overview.activeOrders}
            />
            <MetricTile
              label={tr("终态订单", "TERMINAL")}
              value={overview.terminalOrders}
            />
            <MetricTile
              label={tr("预订金额", "BOOKED VALUE")}
              value={formatMoney(overview.bookedRevenueCents, locale)}
            />
          </div>
          <MechanicalPanel
            className="dispatch-board"
            eyebrow="LIVE DISPATCH READOUT"
            title={tr("运行状态", "Operating state")}
          >
            <div className="dispatch-readout">
              <div className="dispatch-readout__primary">
                <span>{tr("资源在线率", "RESOURCE ONLINE")}</span>
                <strong>{onlineRatio}%</strong>
                <div aria-label={`${onlineRatio}%`} className="ratio-track">
                  <span style={{ width: `${onlineRatio}%` }} />
                </div>
              </div>
              <div className="dispatch-readout__matrix" aria-hidden="true">
                {resources.map((resource) => (
                  <i
                    className={
                      resource.listingStatus === GpuListingStatus.Online
                        ? "is-online"
                        : ""
                    }
                    key={resource.id}
                  />
                ))}
              </div>
              <dl className="dispatch-protocol">
                <div>
                  <dt>{tr("占用保护", "ALLOCATION GUARD")}</dt>
                  <dd>{tr("已启用", "ACTIVE")}</dd>
                </div>
                <div>
                  <dt>{tr("订单终态", "TERMINAL ORDERS")}</dt>
                  <dd>{overview.terminalOrders}</dd>
                </div>
                <div>
                  <dt>{tr("资产模式", "ASSET MODE")}</dt>
                  <dd>SIMULATED</dd>
                </div>
              </dl>
            </div>
          </MechanicalPanel>
        </section>
      ) : null}
      {!loading && tab === "inventory" ? (
        <div className="admin-split">
          <MechanicalPanel
            eyebrow="NEW UNIT / FORM"
            title={tr("登记模拟资源", "Register simulated resource")}
          >
            <form
              className="stack-form"
              onSubmit={(event) => void createResource(event)}
            >
              <label>
                <span>{tr("资源名称", "Resource name")}</span>
                <input name="name" minLength={2} required />
              </label>
              <label>
                <span>{tr("GPU 型号", "GPU model")}</span>
                <input name="model" minLength={2} required />
              </label>
              <div className="form-row">
                <label>
                  <span>{tr("显存 GB", "Memory GB")}</span>
                  <input
                    name="memoryGb"
                    min="1"
                    max="1024"
                    required
                    type="number"
                  />
                </label>
                <label>
                  <span>{tr("小时价格 ¥", "Hourly ¥")}</span>
                  <input
                    name="hourlyPrice"
                    min="0"
                    required
                    step="0.01"
                    type="number"
                  />
                </label>
              </div>
              <label>
                <span>{tr("区域", "Region")}</span>
                <input name="region" minLength={2} required />
              </label>
              <label>
                <span>{tr("标签（逗号分隔）", "Tags (comma-separated)")}</span>
                <input name="tags" />
              </label>
              <div className="form-row">
                <label>
                  <span>{tr("GPU 数量", "GPU count")}</span>
                  <input
                    defaultValue="1"
                    max="8"
                    min="1"
                    name="gpuCount"
                    required
                    type="number"
                  />
                </label>
                <label>
                  <span>{tr("CPU 核心", "vCPU cores")}</span>
                  <input
                    defaultValue="16"
                    max="512"
                    min="1"
                    name="cpuCores"
                    required
                    type="number"
                  />
                </label>
              </div>
              <div className="form-row">
                <label>
                  <span>{tr("系统内存 GB", "System RAM GB")}</span>
                  <input
                    defaultValue="64"
                    max="4096"
                    min="1"
                    name="systemMemoryGb"
                    required
                    type="number"
                  />
                </label>
                <label>
                  <span>{tr("本地磁盘 GB", "Local storage GB")}</span>
                  <input
                    defaultValue="100"
                    max="16384"
                    min="1"
                    name="storageGb"
                    required
                    type="number"
                  />
                </label>
              </div>
              <div className="form-row">
                <label>
                  <span>CUDA</span>
                  <input defaultValue="12.4" name="cudaVersion" required />
                </label>
                <label>
                  <span>{tr("驱动版本", "Driver version")}</span>
                  <input defaultValue="550" name="driverVersion" required />
                </label>
              </div>
              <div className="form-row">
                <label>
                  <span>{tr("网络 Mbps", "Network Mbps")}</span>
                  <input
                    defaultValue="1000"
                    max="1000000"
                    min="1"
                    name="bandwidthMbps"
                    required
                    type="number"
                  />
                </label>
                <label>
                  <span>{tr("可靠性 %", "Reliability %")}</span>
                  <input
                    defaultValue="99.9"
                    max="100"
                    min="0"
                    name="reliabilityPercent"
                    required
                    step="0.001"
                    type="number"
                  />
                </label>
              </div>
              <button
                className="button button--orange"
                disabled={pending === "create"}
                type="submit"
              >
                {tr("登记为下架状态", "Register offline")}
              </button>
            </form>
          </MechanicalPanel>
          <MechanicalPanel
            className="admin-register"
            eyebrow="INVENTORY REGISTER"
            title={tr("资源清单", "Inventory register")}
          >
            {resources.length === 0 ? (
              <EmptyState
                message={tr(
                  "当前没有资源记录，请使用左侧表单登记首个资源。",
                  "No resources are registered. Use the form to add the first unit.",
                )}
                title={tr("资源寄存器为空", "Inventory register empty")}
              />
            ) : (
              <div className="admin-list">
                {resources.map((resource) => (
                  <article key={resource.id}>
                    <div>
                      <span>{resource.name}</span>
                      <strong>
                        {resource.model} · {resource.memoryGb} GB
                      </strong>
                      <small>
                        {resource.region} ·{" "}
                        {formatMoney(resource.hourlyPriceCents, locale)}/h
                      </small>
                    </div>
                    <StatusLamp
                      label={listingLabel(resource.listingStatus, tr)}
                      tone={statusTone(resource.listingStatus)}
                    />
                    <select
                      aria-label={tr(
                        `设置 ${resource.name} 上架状态`,
                        `Set listing state for ${resource.name}`,
                      )}
                      disabled={pending === resource.id}
                      onChange={(event) =>
                        void changeListing(
                          resource,
                          event.target.value as GpuListingStatus,
                        )
                      }
                      value={resource.listingStatus}
                    >
                      {Object.values(GpuListingStatus).map((value) => (
                        <option key={value} value={value}>
                          {listingLabel(value, tr)}
                        </option>
                      ))}
                    </select>
                  </article>
                ))}
              </div>
            )}
          </MechanicalPanel>
        </div>
      ) : null}
      {!loading && tab === "orders" ? (
        <MechanicalPanel
          eyebrow="GLOBAL ORDER REGISTER"
          title={tr("全部订单", "All orders")}
        >
          {orders.length === 0 ? (
            <EmptyState
              message={tr(
                "当前还没有订单，用户完成预订后会进入全局寄存器。",
                "No orders yet. Reservations appear in the global register.",
              )}
              title={tr("订单寄存器为空", "Order register empty")}
            />
          ) : (
            <div className="admin-list admin-list--orders">
              {orders.map((order) => (
                <article key={order.id}>
                  <div>
                    <span>{order.id}</span>
                    <strong>
                      {order.gpuModel} · {order.gpuName}
                    </strong>
                    <small>
                      {formatDate(order.createdAt, locale)} ·{" "}
                      {formatMoney(order.totalPriceCents, locale)}
                    </small>
                  </div>
                  <StatusLamp
                    label={orderStatusLabel(order.status, tr)}
                    tone={statusTone(order.status)}
                  />
                  {order.status === OrderStatus.Active ? (
                    <button
                      className="button button--dark button--small"
                      disabled={pending === order.id}
                      onClick={() => void cancelOrder(order.id)}
                      type="button"
                    >
                      {tr("取消订单", "Cancel order")}
                    </button>
                  ) : (
                    <span className="terminal-stamp">TERMINAL</span>
                  )}
                </article>
              ))}
            </div>
          )}
        </MechanicalPanel>
      ) : null}
    </div>
  );
}
