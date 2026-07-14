import type {
  AdminOverview,
  AddTeamMemberInput,
  ApiKeyView,
  AttachVolumeInput,
  AuthResponse,
  CloudAccountView,
  CreateApiKeyInput,
  CreateGpuResourceInput,
  CreateNetworkRuleInput,
  CreateOrderInput,
  CreateProjectInput,
  CreateSnapshotInput,
  CreateSshKeyInput,
  CreateTeamInput,
  CreateVolumeInput,
  EnvironmentTemplateView,
  GpuResourceFacets,
  GpuResourceView,
  InstanceView,
  LoginInput,
  NetworkRuleView,
  NotificationView,
  OrderView,
  PaginatedResponse,
  RegisterInput,
  SetGpuListingStatusInput,
  SshKeyView,
  TeamView,
  TopUpInput,
  UpdateGpuResourceInput,
  UserView,
  VolumeView,
} from "@gpu-rental/contracts";

import type {
  AdminResourceQuery,
  DataGateway,
  InstanceQuery,
  OrderQuery,
  ResourceQuery,
} from "./gateway";
import { GatewayError } from "./gateway";

interface ErrorPayload {
  code?: string;
  message?: string;
}

export class ApiGateway implements DataGateway {
  readonly mode = "api" as const;
  private readonly baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
  }

  async getSession(): Promise<UserView | null> {
    try {
      const response = await this.request<AuthResponse>("/auth/me");
      return response.user;
    } catch (error) {
      if (error instanceof GatewayError && error.status === 401) return null;
      throw error;
    }
  }

  login(input: LoginInput): Promise<AuthResponse> {
    return this.request("/auth/login", { method: "POST", body: input });
  }

  register(input: RegisterInput): Promise<AuthResponse> {
    return this.request("/auth/register", { method: "POST", body: input });
  }

  logout(): Promise<void> {
    return this.request("/auth/logout", { method: "POST" });
  }

  logoutAll(): Promise<void> {
    return this.request("/auth/logout-all", { method: "POST" });
  }

  getCloudAccount(): Promise<CloudAccountView> {
    return this.request("/cloud-account");
  }

  topUp(input: TopUpInput): Promise<CloudAccountView> {
    return this.request("/cloud-account/top-ups", {
      method: "POST",
      body: input,
    });
  }

  createSshKey(input: CreateSshKeyInput): Promise<SshKeyView> {
    return this.request("/cloud-account/ssh-keys", {
      method: "POST",
      body: input,
    });
  }

  deleteSshKey(keyId: string): Promise<void> {
    return this.request(
      `/cloud-account/ssh-keys/${encodeURIComponent(keyId)}`,
      {
        method: "DELETE",
      },
    );
  }

  createApiKey(input: CreateApiKeyInput): Promise<ApiKeyView> {
    return this.request("/cloud-account/api-keys", {
      method: "POST",
      body: input,
    });
  }

  deleteApiKey(keyId: string): Promise<void> {
    return this.request(
      `/cloud-account/api-keys/${encodeURIComponent(keyId)}`,
      {
        method: "DELETE",
      },
    );
  }

  createNetworkRule(input: CreateNetworkRuleInput): Promise<NetworkRuleView> {
    return this.request("/cloud-account/network-rules", {
      method: "POST",
      body: input,
    });
  }

  deleteNetworkRule(ruleId: string): Promise<void> {
    return this.request(
      `/cloud-account/network-rules/${encodeURIComponent(ruleId)}`,
      { method: "DELETE" },
    );
  }

  createVolume(input: CreateVolumeInput): Promise<VolumeView> {
    return this.request("/cloud-account/volumes", {
      method: "POST",
      body: input,
    });
  }

  attachVolume(
    volumeId: string,
    input: AttachVolumeInput,
  ): Promise<VolumeView> {
    return this.request(
      `/cloud-account/volumes/${encodeURIComponent(volumeId)}/attach`,
      { method: "POST", body: input },
    );
  }

  detachVolume(volumeId: string): Promise<VolumeView> {
    return this.request(
      `/cloud-account/volumes/${encodeURIComponent(volumeId)}/detach`,
      { method: "POST" },
    );
  }

  createSnapshot(
    volumeId: string,
    input: CreateSnapshotInput,
  ): Promise<VolumeView> {
    return this.request(
      `/cloud-account/volumes/${encodeURIComponent(volumeId)}/snapshots`,
      { method: "POST", body: input },
    );
  }

  deleteVolume(volumeId: string): Promise<VolumeView> {
    return this.request(
      `/cloud-account/volumes/${encodeURIComponent(volumeId)}`,
      {
        method: "DELETE",
      },
    );
  }

  markNotificationRead(notificationId: string): Promise<NotificationView> {
    return this.request(
      `/cloud-account/notifications/${encodeURIComponent(notificationId)}/read`,
      { method: "POST" },
    );
  }

  markAllNotificationsRead(): Promise<void> {
    return this.request("/cloud-account/notifications/read-all", {
      method: "POST",
    });
  }

  listTeams(): Promise<TeamView[]> {
    return this.request("/teams/me");
  }

  createTeam(input: CreateTeamInput): Promise<TeamView> {
    return this.request("/teams", { method: "POST", body: input });
  }

  addTeamMember(teamId: string, input: AddTeamMemberInput): Promise<TeamView> {
    return this.request(`/teams/${encodeURIComponent(teamId)}/members`, {
      method: "POST",
      body: input,
    });
  }

  createProject(teamId: string, input: CreateProjectInput): Promise<TeamView> {
    return this.request(`/teams/${encodeURIComponent(teamId)}/projects`, {
      method: "POST",
      body: input,
    });
  }

  listResources(
    query: ResourceQuery = {},
  ): Promise<PaginatedResponse<GpuResourceView>> {
    return this.request(`/gpu-resources${toQueryString(query)}`);
  }

  getFacets(): Promise<GpuResourceFacets> {
    return this.request("/gpu-resources/facets");
  }

  listEnvironmentTemplates(): Promise<EnvironmentTemplateView[]> {
    return this.request("/environment-templates");
  }

  getResource(resourceId: string): Promise<GpuResourceView> {
    return this.request(`/gpu-resources/${encodeURIComponent(resourceId)}`);
  }

  createOrder(input: CreateOrderInput): Promise<OrderView> {
    return this.request("/orders", { method: "POST", body: input });
  }

  listMyOrders(query: OrderQuery = {}): Promise<PaginatedResponse<OrderView>> {
    return this.request(`/orders/me${toQueryString(query)}`);
  }

  listMyInstances(
    query: InstanceQuery = {},
  ): Promise<PaginatedResponse<InstanceView>> {
    return this.request(`/instances/me${toQueryString(query)}`);
  }

  getInstance(instanceId: string): Promise<InstanceView> {
    return this.request(`/instances/${encodeURIComponent(instanceId)}`);
  }

  startInstance(instanceId: string): Promise<InstanceView> {
    return this.request(`/instances/${encodeURIComponent(instanceId)}/start`, {
      method: "POST",
    });
  }

  stopInstance(instanceId: string): Promise<InstanceView> {
    return this.request(`/instances/${encodeURIComponent(instanceId)}/stop`, {
      method: "POST",
    });
  }

  terminateInstance(instanceId: string): Promise<InstanceView> {
    return this.request(
      `/instances/${encodeURIComponent(instanceId)}/terminate`,
      { method: "POST" },
    );
  }

  returnOrder(orderId: string): Promise<OrderView> {
    return this.request(`/orders/${encodeURIComponent(orderId)}/return`, {
      method: "POST",
    });
  }

  getAdminOverview(): Promise<AdminOverview> {
    return this.request("/admin/overview");
  }

  listAdminResources(
    query: AdminResourceQuery = {},
  ): Promise<PaginatedResponse<GpuResourceView>> {
    return this.request(`/admin/gpu-resources${toQueryString(query)}`);
  }

  createResource(input: CreateGpuResourceInput): Promise<GpuResourceView> {
    return this.request("/admin/gpu-resources", {
      method: "POST",
      body: input,
    });
  }

  updateResource(
    resourceId: string,
    input: UpdateGpuResourceInput,
  ): Promise<GpuResourceView> {
    return this.request(
      `/admin/gpu-resources/${encodeURIComponent(resourceId)}`,
      { method: "PATCH", body: input },
    );
  }

  setListingStatus(
    resourceId: string,
    input: SetGpuListingStatusInput,
  ): Promise<GpuResourceView> {
    return this.request(
      `/admin/gpu-resources/${encodeURIComponent(resourceId)}/listing-status`,
      { method: "PATCH", body: input },
    );
  }

  listAdminOrders(
    query: OrderQuery = {},
  ): Promise<PaginatedResponse<OrderView>> {
    return this.request(`/admin/orders${toQueryString(query)}`);
  }

  cancelOrder(orderId: string): Promise<OrderView> {
    return this.request(`/admin/orders/${encodeURIComponent(orderId)}/cancel`, {
      method: "POST",
    });
  }

  async resetDemo(): Promise<void> {
    throw new GatewayError(
      "Reset is only available in demo mode",
      "DEMO_ONLY",
      400,
    );
  }

  private async request<T>(
    path: string,
    options: { body?: unknown; method?: string } = {},
  ): Promise<T> {
    const response = await fetch(`${this.baseUrl}${path}`, {
      body:
        options.body === undefined ? undefined : JSON.stringify(options.body),
      credentials: "include",
      headers:
        options.body === undefined
          ? undefined
          : { "content-type": "application/json" },
      method: options.method || "GET",
    });

    if (!response.ok) {
      const payload = await readErrorPayload(response);
      throw new GatewayError(
        payload.message || `Request failed with status ${response.status}`,
        payload.code || "REQUEST_FAILED",
        response.status,
      );
    }

    if (response.status === 204) return undefined as T;
    return (await response.json()) as T;
  }
}

function toQueryString(query: object): string {
  const parameters = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== "") parameters.set(key, String(value));
  }
  const serialized = parameters.toString();
  return serialized ? `?${serialized}` : "";
}

async function readErrorPayload(response: Response): Promise<ErrorPayload> {
  try {
    return (await response.json()) as ErrorPayload;
  } catch {
    return {};
  }
}
