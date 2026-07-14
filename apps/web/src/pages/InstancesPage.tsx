import {
  InstanceStatus,
  type InstanceView,
  type PaginatedResponse,
} from "@gpu-rental/contracts";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

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
import { instanceStatusLabel, statusTone } from "../labels";

const EMPTY_INSTANCES: PaginatedResponse<InstanceView> = {
  items: [],
  page: 1,
  pageSize: 20,
  total: 0,
};

export function InstancesPage() {
  const { gateway } = useApp();
  const { locale, tr } = useLocale();
  const [instances, setInstances] = useState(EMPTY_INSTANCES);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    void gateway
      .listMyInstances({ pageSize: 100 })
      .then((result) => {
        if (active) setInstances(result);
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

  async function changeState(
    instance: InstanceView,
    action: "start" | "stop" | "terminate",
  ): Promise<void> {
    setPending(instance.id);
    setError(null);
    try {
      if (action === "start") await gateway.startInstance(instance.id);
      if (action === "stop") await gateway.stopInstance(instance.id);
      if (action === "terminate") await gateway.terminateInstance(instance.id);
      setRevision((value) => value + 1);
    } catch (reason) {
      setError(readErrorMessage(reason));
    } finally {
      setPending(null);
    }
  }

  const running = instances.items.filter(
    (instance) => instance.status === InstanceStatus.Running,
  ).length;
  const accrued = instances.items.reduce(
    (sum, instance) => sum + instance.accruedCostCents,
    0,
  );

  return (
    <div className="page-frame instances-page">
      <header className="page-title-row">
        <div>
          <span className="serial-label">INSTANCE CONTROL / PANEL 06</span>
          <h1>{tr("我的实例", "My instances")}</h1>
          <p>
            {tr(
              "管理模拟实例的运行、停止和终止状态，并查看累计费用。",
              "Manage simulated instance state and inspect accrued usage cost.",
            )}
          </p>
        </div>
        <Link className="button button--orange" to="/">
          {tr("部署新实例", "Deploy another instance")}
        </Link>
      </header>

      <div className="order-metrics">
        <MetricTile
          label={tr("实例总数", "INSTANCES")}
          value={instances.total}
        />
        <MetricTile label={tr("运行中", "RUNNING")} value={running} />
        <MetricTile
          label={tr("本页累计费用", "ACCRUED HERE")}
          value={formatMoney(accrued, locale)}
        />
      </div>

      {loading ? <LoadState /> : null}
      {!loading && error ? (
        <ErrorState
          message={error}
          retry={() => setRevision((value) => value + 1)}
        />
      ) : null}
      {!loading && !error && instances.items.length === 0 ? (
        <EmptyState
          action={
            <Link className="button button--orange" to="/">
              {tr("浏览 GPU", "Browse GPUs")}
            </Link>
          }
          message={tr(
            "完成一次预订后，实例会出现在这里。",
            "Instances appear here after a reservation.",
          )}
          title={tr("尚未部署实例", "No instances deployed")}
        />
      ) : null}

      <div className="instance-grid">
        {instances.items.map((instance) => (
          <MechanicalPanel
            className="instance-card"
            eyebrow={`INSTANCE / ${instance.id.slice(-8).toUpperCase()}`}
            key={instance.id}
          >
            <div className="instance-card__heading">
              <div>
                <span>{instance.gpuName}</span>
                <h2>{instance.name}</h2>
              </div>
              <StatusLamp
                label={instanceStatusLabel(instance.status, tr)}
                tone={statusTone(instance.status)}
              />
            </div>
            <dl className="order-fields">
              <div>
                <dt>{tr("计算资源", "COMPUTE")}</dt>
                <dd>
                  {instance.gpuCount} × {instance.gpuModel} ·{" "}
                  {instance.gpuMemoryGb} GB
                </dd>
              </div>
              <div>
                <dt>{tr("运行环境", "ENVIRONMENT")}</dt>
                <dd>{instance.environmentTemplateName}</dd>
              </div>
              <div>
                <dt>{tr("临时磁盘", "TEMPORARY DISK")}</dt>
                <dd>{instance.temporaryStorageGb} GB</dd>
              </div>
              <div>
                <dt>{tr("累计运行", "BILLABLE TIME")}</dt>
                <dd>{formatDuration(instance.billableSeconds, tr)}</dd>
              </div>
              <div>
                <dt>{tr("累计费用", "ACCRUED COST")}</dt>
                <dd>{formatMoney(instance.accruedCostCents, locale)}</dd>
              </div>
              <div>
                <dt>{tr("租期结束", "LEASE END")}</dt>
                <dd>{formatDate(instance.endsAt, locale)}</dd>
              </div>
            </dl>
            <div className="instance-access">
              <strong>{tr("模拟连接信息", "SIMULATED ACCESS")}</strong>
              {instance.access.sshCommand ? (
                <code>{instance.access.sshCommand}</code>
              ) : null}
              {instance.access.jupyterUrl ? (
                <code>{instance.access.jupyterUrl}</code>
              ) : null}
              {instance.access.webTerminalUrl ? (
                <code>{instance.access.webTerminalUrl}</code>
              ) : null}
              <small>{instance.access.notice}</small>
            </div>
            <div className="instance-actions">
              {instance.status === InstanceStatus.Running ? (
                <button
                  disabled={pending === instance.id}
                  onClick={() => void changeState(instance, "stop")}
                  type="button"
                >
                  {tr("停止", "Stop")}
                </button>
              ) : null}
              {instance.status === InstanceStatus.Stopped ? (
                <button
                  disabled={pending === instance.id}
                  onClick={() => void changeState(instance, "start")}
                  type="button"
                >
                  {tr("启动", "Start")}
                </button>
              ) : null}
              {instance.status !== InstanceStatus.Terminated &&
              instance.status !== InstanceStatus.Failed ? (
                <button
                  className="button button--orange"
                  disabled={pending === instance.id}
                  onClick={() => void changeState(instance, "terminate")}
                  type="button"
                >
                  {tr("终止并退租", "Terminate & return")}
                </button>
              ) : null}
            </div>
          </MechanicalPanel>
        ))}
      </div>
    </div>
  );
}

function formatDuration(
  seconds: number,
  tr: (zh: string, en: string) => string,
): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return hours > 0
    ? `${hours} h ${minutes} min`
    : `${Math.max(1, minutes)} ${tr("分钟", "min")}`;
}
