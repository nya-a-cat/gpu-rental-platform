import {
  BillingEntryType,
  InstanceStatus,
  NetworkProtocol,
  TeamRole,
  VolumeStatus,
  type CloudAccountView,
  type InstanceView,
  type TeamView,
} from "@gpu-rental/contracts";
import { useCallback, useEffect, useState, type FormEvent } from "react";

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

export function CloudAccountPage() {
  const { gateway } = useApp();
  const { locale, tr } = useLocale();
  const [account, setAccount] = useState<CloudAccountView | null>(null);
  const [teams, setTeams] = useState<TeamView[]>([]);
  const [instances, setInstances] = useState<InstanceView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState<string | null>(null);
  const [apiToken, setApiToken] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    const [nextAccount, nextTeams, instancePage] = await Promise.all([
      gateway.getCloudAccount(),
      gateway.listTeams(),
      gateway.listMyInstances({ pageSize: 100 }),
    ]);
    setAccount(nextAccount);
    setTeams(nextTeams);
    setInstances(
      instancePage.items.filter(
        (instance) =>
          instance.status === InstanceStatus.Running ||
          instance.status === InstanceStatus.Stopped,
      ),
    );
  }, [gateway]);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    void refresh()
      .catch((reason: unknown) => {
        if (active) setError(readErrorMessage(reason));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [refresh]);

  async function run(key: string, operation: () => Promise<unknown>) {
    setPending(key);
    setError(null);
    try {
      await operation();
      await refresh();
    } catch (reason) {
      setError(readErrorMessage(reason));
    } finally {
      setPending(null);
    }
  }

  async function submitTopUp(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    await run("top-up", () =>
      gateway.topUp({
        amountCents: Math.round(Number(data.get("amount")) * 100),
      }),
    );
    form.reset();
  }

  async function submitSshKey(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    await run("ssh-key", () =>
      gateway.createSshKey({
        name: String(data.get("name") || ""),
        publicKey: String(data.get("publicKey") || ""),
      }),
    );
    form.reset();
  }

  async function submitApiKey(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    await run("api-key", async () => {
      const key = await gateway.createApiKey({
        name: String(data.get("name") || ""),
      });
      setApiToken(key.token);
    });
    form.reset();
  }

  async function submitNetworkRule(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    await run("network-rule", () =>
      gateway.createNetworkRule({
        instanceId: String(data.get("instanceId") || ""),
        name: String(data.get("name") || ""),
        protocol: String(data.get("protocol")) as NetworkProtocol,
        port: Number(data.get("port")),
        sourceCidr: String(data.get("sourceCidr") || ""),
      }),
    );
    form.reset();
  }

  async function submitVolume(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    await run("volume", () =>
      gateway.createVolume({
        name: String(data.get("name") || ""),
        sizeGb: Number(data.get("sizeGb")),
      }),
    );
    form.reset();
  }

  async function submitTeam(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    await run("team", () =>
      gateway.createTeam({ name: String(data.get("name") || "") }),
    );
    form.reset();
  }

  if (loading) {
    return (
      <div className="page-frame page-frame--narrow">
        <LoadState label={tr("正在读取云账户", "Reading cloud account")} />
      </div>
    );
  }

  if (!account) {
    return (
      <div className="page-frame page-frame--narrow">
        <ErrorState
          message={error ?? "Cloud account unavailable"}
          retry={() => void refresh()}
        />
      </div>
    );
  }

  const unread = account.notifications.filter(
    (notification) => !notification.readAt,
  ).length;
  const activeVolumes = account.volumes.filter(
    (volume) => volume.status !== VolumeStatus.Deleted,
  );

  return (
    <div className="page-frame cloud-account-page">
      <header className="page-title-row">
        <div>
          <span className="serial-label">CLOUD ACCOUNT / PANEL 07</span>
          <h1>{tr("云账户与协作", "Cloud account & collaboration")}</h1>
          <p>
            {tr(
              "管理模拟钱包、访问凭据、网络规则、持久卷、团队项目和通知。",
              "Manage the simulated wallet, access credentials, network rules, persistent volumes, team projects and notifications.",
            )}
          </p>
        </div>
        <StatusLamp label="SIMULATED CONTROL PLANE" tone="warn" />
      </header>

      <div className="order-metrics">
        <MetricTile
          label={tr("钱包余额", "WALLET BALANCE")}
          value={formatMoney(account.wallet.balanceCents, locale)}
        />
        <MetricTile
          label={tr("有效持久卷", "ACTIVE VOLUMES")}
          value={activeVolumes.length}
        />
        <MetricTile label={tr("未读通知", "UNREAD ALERTS")} value={unread} />
      </div>

      {error ? (
        <ErrorState message={error} retry={() => void refresh()} />
      ) : null}

      <section className="cloud-account-grid">
        <MechanicalPanel
          eyebrow="WALLET / LEDGER"
          title={tr("钱包与账单", "Wallet & billing")}
        >
          <form
            className="inline-business-form"
            onSubmit={(event) => void submitTopUp(event)}
          >
            <label>
              <span>{tr("模拟充值金额", "Simulated top-up")}</span>
              <input
                min="1"
                name="amount"
                placeholder="100.00"
                required
                step="0.01"
                type="number"
              />
            </label>
            <button disabled={pending === "top-up"} type="submit">
              {tr("充值", "Top up")}
            </button>
          </form>
          <div className="business-record-list">
            {account.billingEntries.slice(0, 8).map((entry) => (
              <article key={entry.id}>
                <div>
                  <strong>{entry.description}</strong>
                  <small>{formatDate(entry.createdAt, locale)}</small>
                </div>
                <span
                  className={
                    entry.type === BillingEntryType.OrderCharge
                      ? "is-debit"
                      : "is-credit"
                  }
                >
                  {entry.type === BillingEntryType.OrderCharge ? "−" : "+"}
                  {formatMoney(entry.amountCents, locale)}
                </span>
              </article>
            ))}
          </div>
        </MechanicalPanel>

        <MechanicalPanel
          eyebrow="ACCESS / KEYS"
          title={tr("访问凭据", "Access credentials")}
        >
          <form
            className="stack-form compact-form"
            onSubmit={(event) => void submitSshKey(event)}
          >
            <label>
              <span>SSH key name</span>
              <input minLength={2} name="name" required />
            </label>
            <label>
              <span>Public key</span>
              <textarea
                name="publicKey"
                placeholder="ssh-ed25519 AAAA..."
                required
                rows={2}
              />
            </label>
            <button disabled={pending === "ssh-key"} type="submit">
              {tr("登记 SSH 密钥", "Register SSH key")}
            </button>
          </form>
          <form
            className="inline-business-form"
            onSubmit={(event) => void submitApiKey(event)}
          >
            <label>
              <span>API key name</span>
              <input minLength={2} name="name" required />
            </label>
            <button disabled={pending === "api-key"} type="submit">
              {tr("创建 API 密钥", "Create API key")}
            </button>
          </form>
          {apiToken ? (
            <div className="one-time-secret" role="status">
              <strong>{tr("仅显示一次", "Shown once")}</strong>
              <code>{apiToken}</code>
              <button onClick={() => setApiToken(null)} type="button">
                {tr("已保存", "Saved")}
              </button>
            </div>
          ) : null}
          <div className="business-record-list">
            {account.sshKeys.map((key) => (
              <article key={key.id}>
                <div>
                  <strong>{key.name}</strong>
                  <small>{key.fingerprint}</small>
                </div>
                <button
                  onClick={() =>
                    void run(key.id, () => gateway.deleteSshKey(key.id))
                  }
                  type="button"
                >
                  {tr("删除", "Delete")}
                </button>
              </article>
            ))}
            {account.apiKeys.map((key) => (
              <article key={key.id}>
                <div>
                  <strong>{key.name}</strong>
                  <small>{key.prefix}…</small>
                </div>
                <button
                  onClick={() =>
                    void run(key.id, () => gateway.deleteApiKey(key.id))
                  }
                  type="button"
                >
                  {tr("撤销", "Revoke")}
                </button>
              </article>
            ))}
          </div>
        </MechanicalPanel>

        <MechanicalPanel
          eyebrow="FIREWALL / PORTS"
          title={tr("端口与防火墙", "Ports & firewall")}
        >
          {instances.length ? (
            <form
              className="stack-form compact-form"
              onSubmit={(event) => void submitNetworkRule(event)}
            >
              <label>
                <span>{tr("实例", "Instance")}</span>
                <select name="instanceId" required>
                  {instances.map((instance) => (
                    <option key={instance.id} value={instance.id}>
                      {instance.name}
                    </option>
                  ))}
                </select>
              </label>
              <div className="form-row">
                <label>
                  <span>{tr("规则名称", "Rule name")}</span>
                  <input minLength={2} name="name" required />
                </label>
                <label>
                  <span>{tr("协议", "Protocol")}</span>
                  <select name="protocol">
                    <option value={NetworkProtocol.Tcp}>TCP</option>
                    <option value={NetworkProtocol.Udp}>UDP</option>
                  </select>
                </label>
              </div>
              <div className="form-row">
                <label>
                  <span>{tr("端口", "Port")}</span>
                  <input
                    max="65535"
                    min="1"
                    name="port"
                    required
                    type="number"
                  />
                </label>
                <label>
                  <span>Source CIDR</span>
                  <input defaultValue="0.0.0.0/0" name="sourceCidr" required />
                </label>
              </div>
              <button disabled={pending === "network-rule"} type="submit">
                {tr("添加模拟规则", "Add simulated rule")}
              </button>
            </form>
          ) : (
            <EmptyState
              message={tr(
                "创建实例后可以配置端口规则。",
                "Create an instance before adding port rules.",
              )}
              title={tr("没有可用实例", "No mutable instances")}
            />
          )}
          <div className="business-record-list">
            {account.networkRules.map((rule) => (
              <article key={rule.id}>
                <div>
                  <strong>
                    {rule.name} · {rule.protocol.toUpperCase()} {rule.port}
                  </strong>
                  <small>{rule.sourceCidr} · SIMULATED</small>
                </div>
                <button
                  onClick={() =>
                    void run(rule.id, () => gateway.deleteNetworkRule(rule.id))
                  }
                  type="button"
                >
                  {tr("删除", "Delete")}
                </button>
              </article>
            ))}
          </div>
        </MechanicalPanel>

        <MechanicalPanel
          eyebrow="STORAGE / VOLUMES"
          title={tr("持久卷与快照", "Persistent volumes & snapshots")}
        >
          <form
            className="inline-business-form"
            onSubmit={(event) => void submitVolume(event)}
          >
            <label>
              <span>{tr("卷名称", "Volume name")}</span>
              <input minLength={2} name="name" required />
            </label>
            <label>
              <span>{tr("容量 GB", "Size GB")}</span>
              <input
                min="10"
                max="10240"
                name="sizeGb"
                required
                type="number"
              />
            </label>
            <button disabled={pending === "volume"} type="submit">
              {tr("创建卷", "Create volume")}
            </button>
          </form>
          <div className="volume-list">
            {account.volumes.map((volume) => (
              <article
                className={
                  volume.status === VolumeStatus.Deleted ? "is-deleted" : ""
                }
                key={volume.id}
              >
                <div className="volume-list__heading">
                  <strong>{volume.name}</strong>
                  <StatusLamp
                    label={volume.status.toUpperCase()}
                    tone={
                      volume.status === VolumeStatus.Attached
                        ? "good"
                        : volume.status === VolumeStatus.Deleted
                          ? "neutral"
                          : "warn"
                    }
                  />
                </div>
                <small>
                  {volume.sizeGb} GB ·{" "}
                  {formatMoney(volume.monthlyPriceCents, locale)}/
                  {tr("月", "month")} · {volume.snapshots.length} snapshots
                </small>
                {volume.status === VolumeStatus.Available &&
                instances.length ? (
                  <form
                    className="inline-business-form"
                    onSubmit={(event) => {
                      event.preventDefault();
                      const data = new FormData(event.currentTarget);
                      void run(volume.id, () =>
                        gateway.attachVolume(volume.id, {
                          instanceId: String(data.get("instanceId")),
                        }),
                      );
                    }}
                  >
                    <select name="instanceId">
                      {instances.map((instance) => (
                        <option key={instance.id} value={instance.id}>
                          {instance.name}
                        </option>
                      ))}
                    </select>
                    <button type="submit">{tr("挂载", "Attach")}</button>
                  </form>
                ) : null}
                {volume.status === VolumeStatus.Attached ? (
                  <button
                    onClick={() =>
                      void run(volume.id, () => gateway.detachVolume(volume.id))
                    }
                    type="button"
                  >
                    {tr("卸载", "Detach")}
                  </button>
                ) : null}
                {volume.status !== VolumeStatus.Deleted ? (
                  <form
                    className="inline-business-form"
                    onSubmit={(event) => {
                      event.preventDefault();
                      const form = event.currentTarget;
                      const data = new FormData(form);
                      void run(`snapshot-${volume.id}`, () =>
                        gateway.createSnapshot(volume.id, {
                          name: String(data.get("name")),
                        }),
                      ).then(() => form.reset());
                    }}
                  >
                    <input
                      minLength={2}
                      name="name"
                      placeholder={tr("快照名称", "Snapshot name")}
                      required
                    />
                    <button type="submit">{tr("创建快照", "Snapshot")}</button>
                  </form>
                ) : null}
                {volume.status === VolumeStatus.Available ? (
                  <button
                    onClick={() =>
                      void run(`delete-${volume.id}`, () =>
                        gateway.deleteVolume(volume.id),
                      )
                    }
                    type="button"
                  >
                    {tr("删除卷", "Delete volume")}
                  </button>
                ) : null}
              </article>
            ))}
          </div>
        </MechanicalPanel>

        <MechanicalPanel
          eyebrow="TEAMS / PROJECTS"
          title={tr("团队与成本项目", "Teams & cost projects")}
        >
          <form
            className="inline-business-form"
            onSubmit={(event) => void submitTeam(event)}
          >
            <label>
              <span>{tr("团队名称", "Team name")}</span>
              <input minLength={2} name="name" required />
            </label>
            <button disabled={pending === "team"} type="submit">
              {tr("创建团队", "Create team")}
            </button>
          </form>
          <div className="team-list">
            {teams.map((team) => (
              <article key={team.id}>
                <div className="volume-list__heading">
                  <strong>{team.name}</strong>
                  <span>{team.currentUserRole}</span>
                </div>
                <small>
                  {team.members.length} members · {team.projects.length}{" "}
                  projects
                </small>
                {team.projects.map((project) => (
                  <div className="project-line" key={project.id}>
                    <span>{project.name}</span>
                    <strong>
                      {formatMoney(project.bookedCostCents, locale)} /{" "}
                      {formatMoney(project.monthlyBudgetCents, locale)}
                    </strong>
                  </div>
                ))}
                {team.currentUserRole !== TeamRole.Member ? (
                  <>
                    <form
                      className="inline-business-form"
                      onSubmit={(event) => {
                        event.preventDefault();
                        const form = event.currentTarget;
                        const data = new FormData(form);
                        void run(`project-${team.id}`, () =>
                          gateway.createProject(team.id, {
                            name: String(data.get("name")),
                            monthlyBudgetCents: Math.round(
                              Number(data.get("budget")) * 100,
                            ),
                          }),
                        ).then(() => form.reset());
                      }}
                    >
                      <input
                        minLength={2}
                        name="name"
                        placeholder={tr("项目名称", "Project name")}
                        required
                      />
                      <input
                        min="0"
                        name="budget"
                        placeholder={tr("月预算", "Monthly budget")}
                        required
                        step="0.01"
                        type="number"
                      />
                      <button type="submit">
                        {tr("创建项目", "Create project")}
                      </button>
                    </form>
                    <form
                      className="inline-business-form"
                      onSubmit={(event) => {
                        event.preventDefault();
                        const form = event.currentTarget;
                        const data = new FormData(form);
                        void run(`member-${team.id}`, () =>
                          gateway.addTeamMember(team.id, {
                            username: String(data.get("username")),
                            role: String(data.get("role")) as
                              TeamRole.Admin | TeamRole.Member,
                          }),
                        ).then(() => form.reset());
                      }}
                    >
                      <input
                        minLength={3}
                        name="username"
                        placeholder={tr("用户名", "Username")}
                        required
                      />
                      <select name="role">
                        <option value={TeamRole.Member}>Member</option>
                        <option value={TeamRole.Admin}>Admin</option>
                      </select>
                      <button type="submit">
                        {tr("添加成员", "Add member")}
                      </button>
                    </form>
                  </>
                ) : null}
              </article>
            ))}
          </div>
        </MechanicalPanel>

        <MechanicalPanel
          eyebrow="EVENTS / NOTIFICATIONS"
          title={tr("通知中心", "Notification center")}
        >
          <button
            disabled={!unread}
            onClick={() =>
              void run("read-all", () => gateway.markAllNotificationsRead())
            }
            type="button"
          >
            {tr("全部已读", "Mark all read")}
          </button>
          <div className="notification-list">
            {account.notifications.slice(0, 12).map((notification) => (
              <article
                className={notification.readAt ? "is-read" : ""}
                key={notification.id}
              >
                <div>
                  <strong>{notification.title}</strong>
                  <p>{notification.message}</p>
                  <small>{formatDate(notification.createdAt, locale)}</small>
                </div>
                {!notification.readAt ? (
                  <button
                    onClick={() =>
                      void run(notification.id, () =>
                        gateway.markNotificationRead(notification.id),
                      )
                    }
                    type="button"
                  >
                    {tr("已读", "Read")}
                  </button>
                ) : null}
              </article>
            ))}
          </div>
        </MechanicalPanel>
      </section>
    </div>
  );
}
