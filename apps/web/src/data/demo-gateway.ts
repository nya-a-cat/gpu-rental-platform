import {
  ConnectionMode,
  BillingEntryType,
  GpuAvailability,
  GpuListingStatus,
  InstanceStatus,
  NetworkProtocol,
  NotificationType,
  OrderStatus,
  ResourceMode,
  TeamRole,
  UserRole,
  VolumeStatus,
  type AddTeamMemberInput,
  type AdminOverview,
  type ApiKeyView,
  type AttachVolumeInput,
  type AuthResponse,
  type CloudAccountView,
  type CreateApiKeyInput,
  type CreateGpuResourceInput,
  type CreateNetworkRuleInput,
  type CreateOrderInput,
  type CreateProjectInput,
  type CreateSnapshotInput,
  type CreateSshKeyInput,
  type CreateTeamInput,
  type CreateVolumeInput,
  type EnvironmentTemplateView,
  type GpuResourceFacets,
  type GpuResourceView,
  type InstanceView,
  type LoginInput,
  type NetworkRuleView,
  type NotificationView,
  type OrderView,
  type PaginatedResponse,
  type RegisterInput,
  type SetGpuListingStatusInput,
  type SshKeyView,
  type TeamView,
  type TopUpInput,
  type UpdateGpuResourceInput,
  type UserView,
  type VolumeView,
} from "@gpu-rental/contracts";

import type {
  AdminResourceQuery,
  DataGateway,
  InstanceQuery,
  OrderQuery,
  ResourceQuery,
} from "./gateway";
import { GatewayError } from "./gateway";

const STORAGE_KEY = "gpu-rental-demo-state-v2";
const DEMO_PASSWORD_HASH =
  "41bd876b085d6031cb0e04de35b88d77f83a4ba39f879fee40805ac19e356023";

export interface StorageLike {
  getItem(key: string): string | null;
  removeItem(key: string): void;
  setItem(key: string, value: string): void;
}

interface DemoState {
  accounts: Record<string, CloudAccountView>;
  credentials: Record<string, string>;
  currentUserId: string | null;
  instances: InstanceView[];
  orders: OrderView[];
  resources: GpuResourceView[];
  sequence: number;
  teams: TeamView[];
  users: UserView[];
  version: 3;
}

const DEMO_TEMPLATES: EnvironmentTemplateView[] = [
  {
    id: "pytorch-jupyter",
    name: "PyTorch + JupyterLab",
    description: "CUDA-enabled PyTorch workspace with JupyterLab access.",
    image: "pytorch/pytorch:2.7.1-cuda12.8-cudnn9-runtime",
    category: "training",
    connectionModes: [
      ConnectionMode.Jupyter,
      ConnectionMode.Ssh,
      ConnectionMode.WebTerminal,
    ],
  },
  {
    id: "cuda-development",
    name: "CUDA Development",
    description: "Minimal CUDA development image for custom workloads.",
    image: "nvidia/cuda:12.8.1-devel-ubuntu24.04",
    category: "development",
    connectionModes: [ConnectionMode.Ssh, ConnectionMode.WebTerminal],
  },
  {
    id: "vllm-inference",
    name: "vLLM Inference",
    description: "OpenAI-compatible model serving environment based on vLLM.",
    image: "vllm/vllm-openai:v0.10.0",
    category: "inference",
    connectionModes: [ConnectionMode.Ssh, ConnectionMode.WebTerminal],
  },
];

export class DemoGateway implements DataGateway {
  readonly mode = "demo" as const;

  constructor(
    private readonly storage: StorageLike = window.localStorage,
    private readonly now: () => Date = () => new Date(),
  ) {}

  async getSession(): Promise<UserView | null> {
    const state = this.read();
    return state.users.find((user) => user.id === state.currentUserId) ?? null;
  }

  async login(input: LoginInput): Promise<AuthResponse> {
    const state = this.read();
    const user = state.users.find(
      (candidate) =>
        candidate.username.toLowerCase() === input.username.toLowerCase(),
    );
    const passwordHash = await hashPassword(input.password);
    if (!user || state.credentials[user.id] !== passwordHash) {
      throw new GatewayError(
        "Demo account or password is incorrect",
        "INVALID_CREDENTIALS",
        401,
      );
    }
    state.currentUserId = user.id;
    this.write(state);
    return { user };
  }

  async register(input: RegisterInput): Promise<AuthResponse> {
    const state = this.read();
    if (
      state.users.some(
        (user) => user.username.toLowerCase() === input.username.toLowerCase(),
      )
    ) {
      throw new GatewayError(
        "The username is already in use",
        "USERNAME_TAKEN",
        409,
      );
    }
    const timestamp = this.now().toISOString();
    const user: UserView = {
      id: `demo-user-${state.sequence++}`,
      username: input.username,
      role: UserRole.User,
      createdAt: timestamp,
      updatedAt: timestamp,
    };
    state.users.push(user);
    state.accounts[user.id] = createDemoCloudAccount(timestamp);
    state.credentials[user.id] = await hashPassword(input.password);
    state.currentUserId = user.id;
    this.write(state);
    return { user };
  }

  async logout(): Promise<void> {
    const state = this.read();
    state.currentUserId = null;
    this.write(state);
  }

  logoutAll(): Promise<void> {
    return this.logout();
  }

  async getCloudAccount(): Promise<CloudAccountView> {
    const state = this.read();
    const user = requireUser(state);
    return cloneValue(requireDemoAccount(state, user.id));
  }

  async topUp(input: TopUpInput): Promise<CloudAccountView> {
    const state = this.read();
    const user = requireUser(state);
    const account = requireDemoAccount(state, user.id);
    const timestamp = this.now().toISOString();
    account.wallet.balanceCents += input.amountCents;
    account.wallet.updatedAt = timestamp;
    account.billingEntries.unshift({
      id: `demo-billing-${state.sequence++}`,
      type: BillingEntryType.TopUp,
      amountCents: input.amountCents,
      reference: `top-up:${state.sequence}`,
      description: "Simulated wallet top-up",
      createdAt: timestamp,
    });
    addDemoNotification(
      state,
      user.id,
      NotificationType.Billing,
      "Wallet topped up",
      `${input.amountCents} cents were credited.`,
      timestamp,
    );
    this.write(state);
    return cloneValue(account);
  }

  async createSshKey(input: CreateSshKeyInput): Promise<SshKeyView> {
    const state = this.read();
    const user = requireUser(state);
    const timestamp = this.now().toISOString();
    const key: SshKeyView = {
      id: `demo-ssh-${state.sequence++}`,
      name: input.name.trim(),
      fingerprint: `SHA256:demo-${state.sequence.toString(36)}`,
      publicKey: input.publicKey.trim(),
      createdAt: timestamp,
    };
    requireDemoAccount(state, user.id).sshKeys.unshift(key);
    addDemoNotification(
      state,
      user.id,
      NotificationType.Security,
      "SSH key registered",
      `${key.name} is ready for simulated instances.`,
      timestamp,
    );
    this.write(state);
    return cloneValue(key);
  }

  async deleteSshKey(keyId: string): Promise<void> {
    const state = this.read();
    const user = requireUser(state);
    const account = requireDemoAccount(state, user.id);
    const index = account.sshKeys.findIndex((key) => key.id === keyId);
    if (index < 0) throw notFound("SSH_KEY_NOT_FOUND");
    account.sshKeys.splice(index, 1);
    this.write(state);
  }

  async createApiKey(input: CreateApiKeyInput): Promise<ApiKeyView> {
    const state = this.read();
    const user = requireUser(state);
    const timestamp = this.now().toISOString();
    const token = `gpr_demo_${crypto.randomUUID().replaceAll("-", "")}`;
    const key: ApiKeyView = {
      id: `demo-api-${state.sequence++}`,
      name: input.name.trim(),
      prefix: token.slice(0, 12),
      token: null,
      createdAt: timestamp,
      lastUsedAt: null,
    };
    requireDemoAccount(state, user.id).apiKeys.unshift(key);
    addDemoNotification(
      state,
      user.id,
      NotificationType.Security,
      "API key created",
      `${key.name} was created. Its token is shown once.`,
      timestamp,
    );
    this.write(state);
    return { ...cloneValue(key), token };
  }

  async deleteApiKey(keyId: string): Promise<void> {
    const state = this.read();
    const user = requireUser(state);
    const account = requireDemoAccount(state, user.id);
    const index = account.apiKeys.findIndex((key) => key.id === keyId);
    if (index < 0) throw notFound("API_KEY_NOT_FOUND");
    account.apiKeys.splice(index, 1);
    this.write(state);
  }

  async createNetworkRule(
    input: CreateNetworkRuleInput,
  ): Promise<NetworkRuleView> {
    const state = this.read();
    const user = requireUser(state);
    findDemoInstance(state, input.instanceId, user.id);
    const account = requireDemoAccount(state, user.id);
    if (
      account.networkRules.some(
        (rule) =>
          rule.instanceId === input.instanceId &&
          rule.protocol === input.protocol &&
          rule.port === input.port,
      )
    ) {
      throw new GatewayError(
        "The instance already has this protocol and port",
        "NETWORK_RULE_EXISTS",
        409,
      );
    }
    const timestamp = this.now().toISOString();
    const rule: NetworkRuleView = {
      id: `demo-rule-${state.sequence++}`,
      instanceId: input.instanceId,
      name: input.name.trim(),
      protocol: input.protocol,
      port: input.port,
      sourceCidr: input.sourceCidr,
      simulated: true,
      createdAt: timestamp,
    };
    account.networkRules.unshift(rule);
    addDemoNotification(
      state,
      user.id,
      NotificationType.Security,
      "Port rule added",
      `${rule.protocol.toUpperCase()} ${rule.port} was added to the simulated firewall.`,
      timestamp,
    );
    this.write(state);
    return cloneValue(rule);
  }

  async deleteNetworkRule(ruleId: string): Promise<void> {
    const state = this.read();
    const user = requireUser(state);
    const account = requireDemoAccount(state, user.id);
    const index = account.networkRules.findIndex((rule) => rule.id === ruleId);
    if (index < 0) throw notFound("NETWORK_RULE_NOT_FOUND");
    account.networkRules.splice(index, 1);
    this.write(state);
  }

  async createVolume(input: CreateVolumeInput): Promise<VolumeView> {
    const state = this.read();
    const user = requireUser(state);
    const timestamp = this.now().toISOString();
    const volume: VolumeView = {
      id: `demo-volume-${state.sequence++}`,
      name: input.name.trim(),
      sizeGb: input.sizeGb,
      status: VolumeStatus.Available,
      attachedInstanceId: null,
      monthlyPriceCents: input.sizeGb * 25,
      snapshots: [],
      createdAt: timestamp,
      updatedAt: timestamp,
    };
    requireDemoAccount(state, user.id).volumes.unshift(volume);
    addDemoNotification(
      state,
      user.id,
      NotificationType.Storage,
      "Persistent volume created",
      `${volume.name} (${volume.sizeGb} GB) is ready to attach.`,
      timestamp,
    );
    this.write(state);
    return cloneValue(volume);
  }

  async attachVolume(
    volumeId: string,
    input: AttachVolumeInput,
  ): Promise<VolumeView> {
    const state = this.read();
    const user = requireUser(state);
    findDemoInstance(state, input.instanceId, user.id);
    const volume = findDemoVolume(state, user.id, volumeId);
    if (volume.status === VolumeStatus.Deleted) {
      throw new GatewayError("The volume was deleted", "VOLUME_DELETED", 409);
    }
    if (
      volume.status === VolumeStatus.Attached &&
      volume.attachedInstanceId !== input.instanceId
    ) {
      throw new GatewayError(
        "The volume is attached to another instance",
        "VOLUME_ALREADY_ATTACHED",
        409,
      );
    }
    volume.status = VolumeStatus.Attached;
    volume.attachedInstanceId = input.instanceId;
    volume.updatedAt = this.now().toISOString();
    this.write(state);
    return cloneValue(volume);
  }

  async detachVolume(volumeId: string): Promise<VolumeView> {
    const state = this.read();
    const user = requireUser(state);
    const volume = findDemoVolume(state, user.id, volumeId);
    if (volume.status === VolumeStatus.Deleted) {
      throw new GatewayError("The volume was deleted", "VOLUME_DELETED", 409);
    }
    volume.status = VolumeStatus.Available;
    volume.attachedInstanceId = null;
    volume.updatedAt = this.now().toISOString();
    this.write(state);
    return cloneValue(volume);
  }

  async createSnapshot(
    volumeId: string,
    input: CreateSnapshotInput,
  ): Promise<VolumeView> {
    const state = this.read();
    const user = requireUser(state);
    const volume = findDemoVolume(state, user.id, volumeId);
    if (volume.status === VolumeStatus.Deleted) {
      throw new GatewayError("The volume was deleted", "VOLUME_DELETED", 409);
    }
    const timestamp = this.now().toISOString();
    volume.snapshots.unshift({
      id: `demo-snapshot-${state.sequence++}`,
      name: input.name.trim(),
      sizeGb: volume.sizeGb,
      createdAt: timestamp,
    });
    volume.updatedAt = timestamp;
    addDemoNotification(
      state,
      user.id,
      NotificationType.Storage,
      "Snapshot completed",
      `${input.name.trim()} was captured from ${volume.name}.`,
      timestamp,
    );
    this.write(state);
    return cloneValue(volume);
  }

  async deleteVolume(volumeId: string): Promise<VolumeView> {
    const state = this.read();
    const user = requireUser(state);
    const volume = findDemoVolume(state, user.id, volumeId);
    if (volume.status === VolumeStatus.Attached) {
      throw new GatewayError(
        "Detach the volume before deleting it",
        "VOLUME_ATTACHED",
        409,
      );
    }
    volume.status = VolumeStatus.Deleted;
    volume.updatedAt = this.now().toISOString();
    this.write(state);
    return cloneValue(volume);
  }

  async markNotificationRead(
    notificationId: string,
  ): Promise<NotificationView> {
    const state = this.read();
    const user = requireUser(state);
    const notification = requireDemoAccount(state, user.id).notifications.find(
      (candidate) => candidate.id === notificationId,
    );
    if (!notification) throw notFound("NOTIFICATION_NOT_FOUND");
    notification.readAt ??= this.now().toISOString();
    this.write(state);
    return cloneValue(notification);
  }

  async markAllNotificationsRead(): Promise<void> {
    const state = this.read();
    const user = requireUser(state);
    const timestamp = this.now().toISOString();
    for (const notification of requireDemoAccount(state, user.id)
      .notifications) {
      notification.readAt ??= timestamp;
    }
    this.write(state);
  }

  async listTeams(): Promise<TeamView[]> {
    const state = this.read();
    const user = requireUser(state);
    return state.teams
      .filter((team) =>
        team.members.some((member) => member.userId === user.id),
      )
      .map((team) => demoTeamForUser(team, user.id));
  }

  async createTeam(input: CreateTeamInput): Promise<TeamView> {
    const state = this.read();
    const user = requireUser(state);
    if (
      state.teams.some(
        (team) =>
          team.members.some(
            (member) =>
              member.userId === user.id && member.role === TeamRole.Owner,
          ) && team.name.toLowerCase() === input.name.trim().toLowerCase(),
      )
    ) {
      throw new GatewayError(
        "You already own a team with this name",
        "TEAM_NAME_TAKEN",
        409,
      );
    }
    const timestamp = this.now().toISOString();
    const team: TeamView = {
      id: `demo-team-${state.sequence++}`,
      name: input.name.trim(),
      currentUserRole: TeamRole.Owner,
      members: [
        {
          userId: user.id,
          username: user.username,
          role: TeamRole.Owner,
          joinedAt: timestamp,
        },
      ],
      projects: [],
      createdAt: timestamp,
      updatedAt: timestamp,
    };
    state.teams.unshift(team);
    addDemoNotification(
      state,
      user.id,
      NotificationType.Team,
      "Team created",
      `${team.name} is ready for members and projects.`,
      timestamp,
    );
    this.write(state);
    return cloneValue(team);
  }

  async addTeamMember(
    teamId: string,
    input: AddTeamMemberInput,
  ): Promise<TeamView> {
    const state = this.read();
    const actor = requireUser(state);
    const team = findManageableDemoTeam(state, teamId, actor.id);
    const user = state.users.find(
      (candidate) =>
        candidate.username.toLowerCase() === input.username.toLowerCase(),
    );
    if (!user) throw notFound("TEAM_MEMBER_USER_NOT_FOUND");
    if (team.members.some((member) => member.userId === user.id)) {
      throw new GatewayError(
        "The user is already a team member",
        "TEAM_MEMBER_EXISTS",
        409,
      );
    }
    const timestamp = this.now().toISOString();
    team.members.push({
      userId: user.id,
      username: user.username,
      role: input.role,
      joinedAt: timestamp,
    });
    team.updatedAt = timestamp;
    addDemoNotification(
      state,
      user.id,
      NotificationType.Team,
      "Added to team",
      `You joined ${team.name} as ${input.role}.`,
      timestamp,
    );
    this.write(state);
    return demoTeamForUser(team, actor.id);
  }

  async createProject(
    teamId: string,
    input: CreateProjectInput,
  ): Promise<TeamView> {
    const state = this.read();
    const actor = requireUser(state);
    const team = findManageableDemoTeam(state, teamId, actor.id);
    if (
      team.projects.some(
        (project) => project.name.toLowerCase() === input.name.toLowerCase(),
      )
    ) {
      throw new GatewayError(
        "The team already has a project with this name",
        "PROJECT_NAME_TAKEN",
        409,
      );
    }
    const timestamp = this.now().toISOString();
    team.projects.push({
      id: crypto.randomUUID(),
      name: input.name.trim(),
      monthlyBudgetCents: input.monthlyBudgetCents,
      bookedCostCents: 0,
      createdAt: timestamp,
    });
    team.updatedAt = timestamp;
    this.write(state);
    return demoTeamForUser(team, actor.id);
  }

  async listResources(
    query: ResourceQuery = {},
  ): Promise<PaginatedResponse<GpuResourceView>> {
    const state = this.read();
    const resources = state.resources
      .filter((resource) => resource.listingStatus === GpuListingStatus.Online)
      .map((resource) => this.withAvailability(resource, state.orders));
    return paginate(filterResources(resources, query), query);
  }

  async getFacets(): Promise<GpuResourceFacets> {
    const resources = this.read().resources.filter(
      (resource) => resource.listingStatus === GpuListingStatus.Online,
    );
    return {
      models: unique(resources.map((resource) => resource.model)).sort(),
      regions: unique(resources.map((resource) => resource.region)).sort(),
      memoryGbValues: unique(
        resources.map((resource) => resource.memoryGb),
      ).sort((left, right) => left - right),
      maxHourlyPriceCents: Math.max(
        0,
        ...resources.map((resource) => resource.hourlyPriceCents),
      ),
    };
  }

  async listEnvironmentTemplates(): Promise<EnvironmentTemplateView[]> {
    return DEMO_TEMPLATES.map((template) => ({ ...template }));
  }

  async getResource(resourceId: string): Promise<GpuResourceView> {
    const state = this.read();
    const resource = state.resources.find(
      (candidate) =>
        candidate.id === resourceId &&
        candidate.listingStatus === GpuListingStatus.Online,
    );
    if (!resource) throw notFound("GPU_RESOURCE_NOT_FOUND");
    return this.withAvailability(resource, state.orders);
  }

  async createOrder(input: CreateOrderInput): Promise<OrderView> {
    const state = this.read();
    const user = requireUser(state);
    const resource = state.resources.find(
      (candidate) => candidate.id === input.gpuResourceId,
    );
    if (!resource || resource.listingStatus !== GpuListingStatus.Online) {
      throw new GatewayError(
        "The GPU resource is not online",
        "GPU_RESOURCE_UNAVAILABLE",
        409,
      );
    }
    if (hasActiveOrder(state.orders, resource.id)) {
      throw new GatewayError(
        "The GPU resource already has an active order",
        "GPU_RESOURCE_RENTED",
        409,
      );
    }
    const template = resolveDemoTemplate(input.environmentTemplateId);
    const project = resolveDemoProject(state, user.id, input.projectId);
    const startsAt = this.now();
    const timestamp = startsAt.toISOString();
    const order: OrderView = {
      id: `demo-order-${state.sequence++}`,
      userId: user.id,
      gpuResourceId: resource.id,
      gpuName: resource.name,
      gpuModel: resource.model,
      gpuMemoryGb: resource.memoryGb,
      gpuCount: resource.gpuCount,
      temporaryStorageGb: resource.storageGb,
      environmentTemplateId: template.id,
      environmentTemplateName: template.name,
      instanceName: input.instanceName?.trim() || `${resource.name} workload`,
      projectId: project?.project.id ?? null,
      projectName: project?.project.name ?? null,
      teamName: project?.team.name ?? null,
      region: resource.region,
      hourlyPriceCents: resource.hourlyPriceCents,
      durationHours: input.durationHours,
      totalPriceCents: resource.hourlyPriceCents * input.durationHours,
      status: OrderStatus.Active,
      startsAt: timestamp,
      endsAt: new Date(
        startsAt.getTime() + input.durationHours * 3_600_000,
      ).toISOString(),
      returnedAt: null,
      cancelledAt: null,
      createdAt: timestamp,
      updatedAt: timestamp,
    };
    debitDemoOrder(state, order, startsAt);
    if (project) project.project.bookedCostCents += order.totalPriceCents;
    state.orders.unshift(order);
    state.instances.unshift(
      createDemoInstance(
        `demo-instance-${state.sequence++}`,
        order,
        resource,
        template,
        startsAt,
      ),
    );
    this.write(state);
    return order;
  }

  async listMyOrders(
    query: OrderQuery = {},
  ): Promise<PaginatedResponse<OrderView>> {
    const state = this.read();
    const user = requireUser(state);
    return paginate(
      filterOrders(
        state.orders.filter((order) => order.userId === user.id),
        query,
      ),
      query,
    );
  }

  async returnOrder(orderId: string): Promise<OrderView> {
    const state = this.read();
    const user = requireUser(state);
    const order = state.orders.find(
      (candidate) => candidate.id === orderId && candidate.userId === user.id,
    );
    if (!order) throw notFound("ORDER_NOT_FOUND");
    if (order.status === OrderStatus.Returned) return order;
    if (order.status !== OrderStatus.Active) {
      throw terminalOrderError();
    }
    order.status = OrderStatus.Returned;
    order.returnedAt = this.now().toISOString();
    order.updatedAt = order.returnedAt;
    terminateDemoInstanceByOrder(state, order.id, this.now());
    this.write(state);
    return order;
  }

  async listMyInstances(
    query: InstanceQuery = {},
  ): Promise<PaginatedResponse<InstanceView>> {
    const state = this.read();
    const user = requireUser(state);
    const instances = state.instances
      .filter(
        (instance) =>
          instance.userId === user.id &&
          (!query.status || instance.status === query.status),
      )
      .map((instance) => refreshDemoInstance(instance, this.now()))
      .sort((left, right) => right.createdAt.localeCompare(left.createdAt));
    return paginate(instances, query);
  }

  async getInstance(instanceId: string): Promise<InstanceView> {
    const state = this.read();
    const user = requireUser(state);
    const instance = state.instances.find(
      (candidate) =>
        candidate.id === instanceId && candidate.userId === user.id,
    );
    if (!instance) throw notFound("INSTANCE_NOT_FOUND");
    return refreshDemoInstance(instance, this.now());
  }

  async startInstance(instanceId: string): Promise<InstanceView> {
    const state = this.read();
    const user = requireUser(state);
    const instance = findDemoInstance(state, instanceId, user.id);
    if (instance.status === InstanceStatus.Running) {
      return refreshDemoInstance(instance, this.now());
    }
    assertDemoInstanceMutable(instance);
    const now = this.now();
    if (now >= new Date(instance.endsAt)) {
      terminateDemoInstance(state, instance, now);
      this.write(state);
      throw new GatewayError(
        "The instance lease has expired",
        "INSTANCE_LEASE_EXPIRED",
        409,
      );
    }
    instance.status = InstanceStatus.Running;
    instance.runningSince = now.toISOString();
    instance.stoppedAt = null;
    instance.updatedAt = now.toISOString();
    addDemoNotification(
      state,
      user.id,
      NotificationType.Instance,
      "Instance started",
      `${instance.name} resumed billable runtime.`,
      instance.updatedAt,
    );
    this.write(state);
    return refreshDemoInstance(instance, now);
  }

  async stopInstance(instanceId: string): Promise<InstanceView> {
    const state = this.read();
    const user = requireUser(state);
    const instance = findDemoInstance(state, instanceId, user.id);
    if (instance.status === InstanceStatus.Stopped) return instance;
    assertDemoInstanceMutable(instance);
    const now = this.now();
    instance.billableSeconds = calculateDemoBillableSeconds(instance, now);
    instance.runningSince = null;
    instance.accruedCostCents = calculateDemoAccruedCost(instance, now);
    instance.status = InstanceStatus.Stopped;
    instance.stoppedAt = now.toISOString();
    instance.updatedAt = now.toISOString();
    addDemoNotification(
      state,
      user.id,
      NotificationType.Instance,
      "Instance stopped",
      `${instance.name} stopped accruing runtime charges.`,
      instance.updatedAt,
    );
    this.write(state);
    return instance;
  }

  async terminateInstance(instanceId: string): Promise<InstanceView> {
    const state = this.read();
    const user = requireUser(state);
    const instance = findDemoInstance(state, instanceId, user.id);
    const now = this.now();
    terminateDemoInstance(state, instance, now);
    const order = state.orders.find(
      (candidate) => candidate.id === instance.orderId,
    );
    if (order?.status === OrderStatus.Active) {
      order.status = OrderStatus.Returned;
      order.returnedAt = now.toISOString();
      order.updatedAt = order.returnedAt;
    }
    this.write(state);
    return instance;
  }

  async getAdminOverview(): Promise<AdminOverview> {
    const state = this.read();
    requireAdmin(state);
    const activeOrders = state.orders.filter(
      (order) => order.status === OrderStatus.Active,
    );
    return {
      usersTotal: state.users.length,
      resourcesTotal: state.resources.length,
      resourcesOnline: state.resources.filter(
        (resource) => resource.listingStatus === GpuListingStatus.Online,
      ).length,
      activeOrders: activeOrders.length,
      terminalOrders: state.orders.length - activeOrders.length,
      bookedRevenueCents: state.orders.reduce(
        (sum, order) => sum + order.totalPriceCents,
        0,
      ),
    };
  }

  async listAdminResources(
    query: AdminResourceQuery = {},
  ): Promise<PaginatedResponse<GpuResourceView>> {
    const state = this.read();
    requireAdmin(state);
    let resources = state.resources.map((resource) =>
      this.withAvailability(resource, state.orders),
    );
    if (query.listingStatus) {
      resources = resources.filter(
        (resource) => resource.listingStatus === query.listingStatus,
      );
    }
    return paginate(filterResources(resources, query), query);
  }

  async createResource(
    input: CreateGpuResourceInput,
  ): Promise<GpuResourceView> {
    const state = this.read();
    requireAdmin(state);
    if (state.resources.some((resource) => resource.name === input.name)) {
      throw new GatewayError(
        "A resource with the same name already exists",
        "GPU_RESOURCE_NAME_TAKEN",
        409,
      );
    }
    const timestamp = this.now().toISOString();
    const resource: GpuResourceView = {
      id: `demo-gpu-${state.sequence++}`,
      name: input.name,
      model: input.model,
      memoryGb: input.memoryGb,
      gpuCount: input.gpuCount ?? 1,
      cpuCores: input.cpuCores ?? 16,
      systemMemoryGb: input.systemMemoryGb ?? 64,
      storageGb: input.storageGb ?? 100,
      cudaVersion: input.cudaVersion ?? "12.4",
      driverVersion: input.driverVersion ?? "550",
      bandwidthMbps: input.bandwidthMbps ?? 1000,
      reliabilityPercent: input.reliabilityPercent ?? 99.9,
      region: input.region,
      hourlyPriceCents: input.hourlyPriceCents,
      tags: input.tags ?? [],
      resourceMode: ResourceMode.Simulated,
      listingStatus: input.listingStatus ?? GpuListingStatus.Offline,
      availability: GpuAvailability.Available,
      createdAt: timestamp,
      updatedAt: timestamp,
    };
    state.resources.unshift(resource);
    this.write(state);
    return resource;
  }

  async updateResource(
    resourceId: string,
    input: UpdateGpuResourceInput,
  ): Promise<GpuResourceView> {
    const state = this.read();
    requireAdmin(state);
    const resource = state.resources.find(
      (candidate) => candidate.id === resourceId,
    );
    if (!resource) throw notFound("GPU_RESOURCE_NOT_FOUND");
    Object.assign(resource, input, { updatedAt: this.now().toISOString() });
    this.write(state);
    return this.withAvailability(resource, state.orders);
  }

  async setListingStatus(
    resourceId: string,
    input: SetGpuListingStatusInput,
  ): Promise<GpuResourceView> {
    const state = this.read();
    requireAdmin(state);
    const resource = state.resources.find(
      (candidate) => candidate.id === resourceId,
    );
    if (!resource) throw notFound("GPU_RESOURCE_NOT_FOUND");
    if (
      input.listingStatus !== GpuListingStatus.Online &&
      hasActiveOrder(state.orders, resourceId)
    ) {
      throw new GatewayError(
        "An active order must finish before the resource can be taken offline",
        "RESOURCE_IN_USE",
        409,
      );
    }
    resource.listingStatus = input.listingStatus;
    resource.updatedAt = this.now().toISOString();
    this.write(state);
    return this.withAvailability(resource, state.orders);
  }

  async listAdminOrders(
    query: OrderQuery = {},
  ): Promise<PaginatedResponse<OrderView>> {
    const state = this.read();
    requireAdmin(state);
    return paginate(filterOrders(state.orders, query), query);
  }

  async cancelOrder(orderId: string): Promise<OrderView> {
    const state = this.read();
    requireAdmin(state);
    const order = state.orders.find((candidate) => candidate.id === orderId);
    if (!order) throw notFound("ORDER_NOT_FOUND");
    if (order.status === OrderStatus.Cancelled) return order;
    if (order.status !== OrderStatus.Active) throw terminalOrderError();
    order.status = OrderStatus.Cancelled;
    order.cancelledAt = this.now().toISOString();
    order.updatedAt = order.cancelledAt;
    terminateDemoInstanceByOrder(state, order.id, this.now());
    this.write(state);
    return order;
  }

  async resetDemo(): Promise<void> {
    this.storage.removeItem(STORAGE_KEY);
    this.write(createInitialState(this.now()));
  }

  private read(): DemoState {
    const raw = this.storage.getItem(STORAGE_KEY);
    let state: DemoState;
    try {
      state = raw
        ? (JSON.parse(raw) as DemoState)
        : createInitialState(this.now());
      if (state.version !== 3) throw new Error("Unsupported demo state");
    } catch {
      state = createInitialState(this.now());
    }
    if (expireDueOrders(state, this.now())) this.write(state);
    return state;
  }

  private withAvailability(
    resource: GpuResourceView,
    orders: OrderView[],
  ): GpuResourceView {
    return {
      ...resource,
      availability: hasActiveOrder(orders, resource.id)
        ? GpuAvailability.Rented
        : GpuAvailability.Available,
    };
  }

  private write(state: DemoState): void {
    this.storage.setItem(STORAGE_KEY, JSON.stringify(state));
  }
}

function createInitialState(now: Date): DemoState {
  const timestamp = now.toISOString();
  const users: UserView[] = [
    createUser("demo-user", "operator", UserRole.User, timestamp),
    createUser("demo-admin", "dispatcher", UserRole.Admin, timestamp),
  ];
  const resources = [
    createResource(
      "gpu-01",
      "CN-EAST / BAY 04",
      "NVIDIA H100",
      80,
      "cn-east",
      3290,
      ["Hopper", "80GB"],
      timestamp,
    ),
    createResource(
      "gpu-02",
      "CN-EAST / BAY 11",
      "NVIDIA A100",
      80,
      "cn-east",
      2290,
      ["Ampere", "80GB"],
      timestamp,
    ),
    createResource(
      "gpu-03",
      "AP-NE / BAY 03",
      "NVIDIA L40S",
      48,
      "ap-northeast",
      1290,
      ["Ada", "48GB"],
      timestamp,
    ),
    createResource(
      "gpu-04",
      "CN-SOUTH / BAY 08",
      "NVIDIA RTX 4090",
      24,
      "cn-south",
      690,
      ["Ada", "24GB"],
      timestamp,
    ),
    createResource(
      "gpu-05",
      "EU-CENTRAL / BAY 06",
      "AMD MI300X",
      192,
      "eu-central",
      2790,
      ["CDNA 3", "192GB"],
      timestamp,
    ),
    createResource(
      "gpu-06",
      "US-EAST / BAY 12",
      "RTX 6000 Ada",
      48,
      "us-east",
      1190,
      ["Ada", "48GB"],
      timestamp,
    ),
  ];
  const startsAt = new Date(now.getTime() - 45 * 60_000);
  const activeResource = resources[1]!;
  const team: TeamView = {
    id: "demo-team-01",
    name: "Applied AI Lab",
    currentUserRole: TeamRole.Owner,
    members: [
      {
        userId: users[0]!.id,
        username: users[0]!.username,
        role: TeamRole.Owner,
        joinedAt: timestamp,
      },
    ],
    projects: [
      {
        id: "4d8f1ce1-02d2-4b99-8ff2-71b6d701f10d",
        name: "Baseline training",
        monthlyBudgetCents: 200_000,
        bookedCostCents: 0,
        createdAt: timestamp,
      },
    ],
    createdAt: timestamp,
    updatedAt: timestamp,
  };
  const order: OrderView = {
    id: "demo-order-01",
    userId: users[0]!.id,
    gpuResourceId: activeResource.id,
    gpuName: activeResource.name,
    gpuModel: activeResource.model,
    gpuMemoryGb: activeResource.memoryGb,
    gpuCount: activeResource.gpuCount,
    temporaryStorageGb: activeResource.storageGb,
    environmentTemplateId: DEMO_TEMPLATES[0]!.id,
    environmentTemplateName: DEMO_TEMPLATES[0]!.name,
    instanceName: "baseline-training-run",
    projectId: team.projects[0]!.id,
    projectName: team.projects[0]!.name,
    teamName: team.name,
    region: activeResource.region,
    hourlyPriceCents: activeResource.hourlyPriceCents,
    durationHours: 12,
    totalPriceCents: activeResource.hourlyPriceCents * 12,
    status: OrderStatus.Active,
    startsAt: startsAt.toISOString(),
    endsAt: new Date(startsAt.getTime() + 12 * 3_600_000).toISOString(),
    returnedAt: null,
    cancelledAt: null,
    createdAt: startsAt.toISOString(),
    updatedAt: startsAt.toISOString(),
  };
  const instance = createDemoInstance(
    "demo-instance-01",
    order,
    activeResource,
    DEMO_TEMPLATES[0]!,
    startsAt,
  );
  const state: DemoState = {
    version: 3,
    sequence: 100,
    currentUserId: null,
    users,
    resources,
    orders: [order],
    instances: [instance],
    teams: [team],
    accounts: {
      "demo-user": createDemoCloudAccount(timestamp),
      "demo-admin": createDemoCloudAccount(timestamp),
    },
    credentials: {
      "demo-user": DEMO_PASSWORD_HASH,
      "demo-admin": DEMO_PASSWORD_HASH,
    },
  };
  debitDemoOrder(state, order, startsAt);
  team.projects[0]!.bookedCostCents = order.totalPriceCents;
  return state;
}

function createUser(
  id: string,
  username: string,
  role: UserRole,
  timestamp: string,
): UserView {
  return { id, username, role, createdAt: timestamp, updatedAt: timestamp };
}

function createResource(
  id: string,
  name: string,
  model: string,
  memoryGb: number,
  region: string,
  hourlyPriceCents: number,
  tags: string[],
  timestamp: string,
): GpuResourceView {
  return {
    id,
    name,
    model,
    memoryGb,
    gpuCount: 1,
    cpuCores: memoryGb >= 80 ? 32 : 16,
    systemMemoryGb: memoryGb >= 80 ? 128 : 64,
    storageGb: memoryGb >= 80 ? 250 : 100,
    cudaVersion: model.includes("AMD") ? "ROCm 6.3" : "12.4",
    driverVersion: model.includes("AMD") ? "6.8" : "550.54",
    bandwidthMbps: 10_000,
    reliabilityPercent: 99.9,
    region,
    hourlyPriceCents,
    tags,
    resourceMode: ResourceMode.Simulated,
    listingStatus: GpuListingStatus.Online,
    availability: GpuAvailability.Available,
    createdAt: timestamp,
    updatedAt: timestamp,
  };
}

function createDemoInstance(
  id: string,
  order: OrderView,
  resource: GpuResourceView,
  template: EnvironmentTemplateView,
  startsAt: Date,
): InstanceView {
  const host = `${id}.simulated.invalid`;
  const modes = new Set(template.connectionModes);
  return {
    id,
    orderId: order.id,
    userId: order.userId,
    gpuResourceId: resource.id,
    name: order.instanceName,
    gpuName: resource.name,
    gpuModel: resource.model,
    gpuCount: resource.gpuCount,
    gpuMemoryGb: resource.memoryGb,
    temporaryStorageGb: resource.storageGb,
    environmentTemplateId: template.id,
    environmentTemplateName: template.name,
    environmentImage: template.image,
    status: InstanceStatus.Running,
    simulated: true,
    startsAt: startsAt.toISOString(),
    endsAt: order.endsAt,
    runningSince: startsAt.toISOString(),
    stoppedAt: null,
    terminatedAt: null,
    billableSeconds: 0,
    accruedCostCents: 0,
    maximumCostCents: order.totalPriceCents,
    access: {
      sshCommand: modes.has(ConnectionMode.Ssh) ? `ssh operator@${host}` : null,
      jupyterUrl: modes.has(ConnectionMode.Jupyter)
        ? `https://${host}/jupyter`
        : null,
      webTerminalUrl: modes.has(ConnectionMode.WebTerminal)
        ? `https://${host}/terminal`
        : null,
      notice:
        "Simulation only. These .invalid endpoints cannot connect to physical infrastructure.",
    },
    createdAt: startsAt.toISOString(),
    updatedAt: startsAt.toISOString(),
  };
}

function resolveDemoTemplate(id?: string): EnvironmentTemplateView {
  const template = DEMO_TEMPLATES.find(
    (candidate) => candidate.id === (id ?? DEMO_TEMPLATES[0]!.id),
  );
  if (!template) throw notFound("ENVIRONMENT_TEMPLATE_NOT_FOUND");
  return template;
}

function findDemoInstance(
  state: DemoState,
  instanceId: string,
  userId: string,
): InstanceView {
  const instance = state.instances.find(
    (candidate) => candidate.id === instanceId && candidate.userId === userId,
  );
  if (!instance) throw notFound("INSTANCE_NOT_FOUND");
  return instance;
}

function assertDemoInstanceMutable(instance: InstanceView): void {
  if (
    instance.status === InstanceStatus.Terminated ||
    instance.status === InstanceStatus.Failed
  ) {
    throw new GatewayError(
      "A terminal instance cannot change status",
      "INSTANCE_TERMINAL",
      409,
    );
  }
}

function calculateDemoBillableSeconds(
  instance: InstanceView,
  now: Date,
): number {
  const currentSegment = instance.runningSince
    ? Math.max(
        0,
        Math.ceil(
          (now.getTime() - new Date(instance.runningSince).getTime()) / 1000,
        ),
      )
    : 0;
  const leaseSeconds = Math.ceil(
    (new Date(instance.endsAt).getTime() -
      new Date(instance.startsAt).getTime()) /
      1000,
  );
  return Math.min(leaseSeconds, instance.billableSeconds + currentSegment);
}

function calculateDemoAccruedCost(instance: InstanceView, now: Date): number {
  const orderHours = Math.max(
    1,
    (new Date(instance.endsAt).getTime() -
      new Date(instance.startsAt).getTime()) /
      3_600_000,
  );
  const hourlyPriceCents = instance.maximumCostCents / orderHours;
  return Math.min(
    instance.maximumCostCents,
    Math.ceil(
      (hourlyPriceCents * calculateDemoBillableSeconds(instance, now)) / 3600,
    ),
  );
}

function refreshDemoInstance(instance: InstanceView, now: Date): InstanceView {
  return {
    ...instance,
    billableSeconds: calculateDemoBillableSeconds(instance, now),
    accruedCostCents: calculateDemoAccruedCost(instance, now),
  };
}

function terminateDemoInstance(
  state: DemoState,
  instance: InstanceView,
  now: Date,
): void {
  if (instance.status === InstanceStatus.Terminated) return;
  instance.billableSeconds = calculateDemoBillableSeconds(instance, now);
  instance.runningSince = null;
  instance.accruedCostCents = calculateDemoAccruedCost(instance, now);
  instance.status = InstanceStatus.Terminated;
  instance.terminatedAt = now.toISOString();
  instance.updatedAt = instance.terminatedAt;
  refundDemoOrder(state, instance, now);
  const account = requireDemoAccount(state, instance.userId);
  for (const volume of account.volumes) {
    if (volume.attachedInstanceId === instance.id) {
      volume.status = VolumeStatus.Available;
      volume.attachedInstanceId = null;
      volume.updatedAt = now.toISOString();
    }
  }
  addDemoNotification(
    state,
    instance.userId,
    NotificationType.Instance,
    "Instance terminated",
    `${instance.name} was terminated and unused booked value was reconciled.`,
    now.toISOString(),
  );
}

function terminateDemoInstanceByOrder(
  state: DemoState,
  orderId: string,
  now: Date,
): void {
  const instance = state.instances.find(
    (candidate) => candidate.orderId === orderId,
  );
  if (instance) terminateDemoInstance(state, instance, now);
}

function filterResources(
  input: GpuResourceView[],
  query: ResourceQuery,
): GpuResourceView[] {
  const resources = input.filter(
    (resource) =>
      (!query.model || resource.model === query.model) &&
      (!query.region || resource.region === query.region) &&
      (!query.memoryGb || resource.memoryGb === query.memoryGb) &&
      (query.maxHourlyPriceCents === undefined ||
        resource.hourlyPriceCents <= query.maxHourlyPriceCents) &&
      (!query.availability || resource.availability === query.availability),
  );
  return resources.sort((left, right) => {
    if (query.sort === "priceAsc") {
      return left.hourlyPriceCents - right.hourlyPriceCents;
    }
    if (query.sort === "priceDesc") {
      return right.hourlyPriceCents - left.hourlyPriceCents;
    }
    return right.createdAt.localeCompare(left.createdAt);
  });
}

function filterOrders(input: OrderView[], query: OrderQuery): OrderView[] {
  return input
    .filter((order) => !query.status || order.status === query.status)
    .sort((left, right) => right.createdAt.localeCompare(left.createdAt));
}

function paginate<T>(
  input: T[],
  query: { page?: number; pageSize?: number },
): PaginatedResponse<T> {
  const page = query.page ?? 1;
  const pageSize = query.pageSize ?? 20;
  return {
    items: input.slice((page - 1) * pageSize, page * pageSize),
    page,
    pageSize,
    total: input.length,
  };
}

function requireUser(state: DemoState): UserView {
  const user = state.users.find(
    (candidate) => candidate.id === state.currentUserId,
  );
  if (!user)
    throw new GatewayError("Authentication required", "UNAUTHORIZED", 401);
  return user;
}

function requireAdmin(state: DemoState): UserView {
  const user = requireUser(state);
  if (user.role !== UserRole.Admin) {
    throw new GatewayError("Administrator access required", "FORBIDDEN", 403);
  }
  return user;
}

function hasActiveOrder(orders: OrderView[], resourceId: string): boolean {
  return orders.some(
    (order) =>
      order.gpuResourceId === resourceId && order.status === OrderStatus.Active,
  );
}

function expireDueOrders(state: DemoState, now: Date): boolean {
  let changed = false;
  for (const order of state.orders) {
    if (order.status === OrderStatus.Active && new Date(order.endsAt) <= now) {
      order.status = OrderStatus.Expired;
      order.updatedAt = now.toISOString();
      terminateDemoInstanceByOrder(state, order.id, now);
      changed = true;
    }
  }
  return changed;
}

function createDemoCloudAccount(timestamp: string): CloudAccountView {
  return {
    wallet: {
      balanceCents: 100_000,
      currency: "CNY",
      updatedAt: timestamp,
    },
    billingEntries: [
      {
        id: `opening-credit-${timestamp}`,
        type: BillingEntryType.OpeningCredit,
        amountCents: 100_000,
        reference: "opening-credit",
        description: "Simulated account opening credit",
        createdAt: timestamp,
      },
    ],
    sshKeys: [],
    apiKeys: [],
    networkRules: [],
    volumes: [],
    notifications: [
      {
        id: `account-ready-${timestamp}`,
        type: NotificationType.Billing,
        title: "Cloud account ready",
        message: "Your simulated wallet and operations workspace are ready.",
        readAt: null,
        createdAt: timestamp,
      },
    ],
  };
}

function requireDemoAccount(
  state: DemoState,
  userId: string,
): CloudAccountView {
  const account = state.accounts[userId];
  if (!account) throw notFound("CLOUD_ACCOUNT_NOT_FOUND");
  return account;
}

function debitDemoOrder(state: DemoState, order: OrderView, now: Date): void {
  const account = requireDemoAccount(state, order.userId);
  if (account.wallet.balanceCents < order.totalPriceCents) {
    throw new GatewayError(
      "The wallet balance is insufficient for this reservation",
      "INSUFFICIENT_BALANCE",
      402,
    );
  }
  const timestamp = now.toISOString();
  account.wallet.balanceCents -= order.totalPriceCents;
  account.wallet.updatedAt = timestamp;
  account.billingEntries.unshift({
    id: `demo-billing-${state.sequence++}`,
    type: BillingEntryType.OrderCharge,
    amountCents: order.totalPriceCents,
    reference: `order:${order.id}:charge`,
    description: `Booked ${order.instanceName}`,
    createdAt: timestamp,
  });
  addDemoNotification(
    state,
    order.userId,
    NotificationType.Billing,
    "Order charged",
    `${order.totalPriceCents} cents were reserved for ${order.instanceName}.`,
    timestamp,
  );
}

function refundDemoOrder(
  state: DemoState,
  instance: InstanceView,
  now: Date,
): void {
  const account = requireDemoAccount(state, instance.userId);
  const reference = `order:${instance.orderId}:refund`;
  if (account.billingEntries.some((entry) => entry.reference === reference)) {
    return;
  }
  const amountCents = Math.max(
    0,
    instance.maximumCostCents - instance.accruedCostCents,
  );
  if (amountCents === 0) return;
  const timestamp = now.toISOString();
  account.wallet.balanceCents += amountCents;
  account.wallet.updatedAt = timestamp;
  account.billingEntries.unshift({
    id: `demo-billing-${state.sequence++}`,
    type: BillingEntryType.OrderRefund,
    amountCents,
    reference,
    description: "Unused reservation value refunded",
    createdAt: timestamp,
  });
  addDemoNotification(
    state,
    instance.userId,
    NotificationType.Billing,
    "Order refund completed",
    `${amountCents} unused cents were returned to the wallet.`,
    timestamp,
  );
}

function addDemoNotification(
  state: DemoState,
  userId: string,
  type: NotificationType,
  title: string,
  message: string,
  createdAt: string,
): void {
  requireDemoAccount(state, userId).notifications.unshift({
    id: `demo-notification-${state.sequence++}`,
    type,
    title,
    message,
    readAt: null,
    createdAt,
  });
}

function findDemoVolume(
  state: DemoState,
  userId: string,
  volumeId: string,
): VolumeView {
  const volume = requireDemoAccount(state, userId).volumes.find(
    (candidate) => candidate.id === volumeId,
  );
  if (!volume) throw notFound("VOLUME_NOT_FOUND");
  return volume;
}

function findManageableDemoTeam(
  state: DemoState,
  teamId: string,
  userId: string,
): TeamView {
  const team = state.teams.find((candidate) => candidate.id === teamId);
  const member = team?.members.find((candidate) => candidate.userId === userId);
  if (
    !team ||
    !member ||
    (member.role !== TeamRole.Owner && member.role !== TeamRole.Admin)
  ) {
    throw notFound("TEAM_NOT_FOUND");
  }
  return team;
}

function demoTeamForUser(team: TeamView, userId: string): TeamView {
  const member = team.members.find((candidate) => candidate.userId === userId);
  if (!member) throw notFound("TEAM_NOT_FOUND");
  return cloneValue({ ...team, currentUserRole: member.role });
}

function resolveDemoProject(
  state: DemoState,
  userId: string,
  projectId?: string,
): { team: TeamView; project: TeamView["projects"][number] } | null {
  if (!projectId) return null;
  for (const team of state.teams) {
    if (!team.members.some((member) => member.userId === userId)) continue;
    const project = team.projects.find(
      (candidate) => candidate.id === projectId,
    );
    if (project) return { team, project };
  }
  throw notFound("PROJECT_NOT_FOUND");
}

function cloneValue<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function notFound(code: string): GatewayError {
  return new GatewayError("The requested record was not found", code, 404);
}

function terminalOrderError(): GatewayError {
  return new GatewayError(
    "A terminal order cannot change status",
    "ORDER_ALREADY_TERMINAL",
    409,
  );
}

function unique<T>(values: T[]): T[] {
  return [...new Set(values)];
}

async function hashPassword(value: string): Promise<string> {
  const data = new TextEncoder().encode(value);
  const digest = await crypto.subtle.digest("SHA-256", data);
  return [...new Uint8Array(digest)]
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("");
}
