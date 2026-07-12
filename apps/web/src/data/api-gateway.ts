import type {
  AdminOverview,
  AuthResponse,
  CreateGpuResourceInput,
  CreateOrderInput,
  GpuResourceFacets,
  GpuResourceView,
  LoginInput,
  OrderView,
  PaginatedResponse,
  RegisterInput,
  SetGpuListingStatusInput,
  UpdateGpuResourceInput,
  UserView,
} from "@gpu-rental/contracts";

import type {
  AdminResourceQuery,
  DataGateway,
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

  listResources(
    query: ResourceQuery = {},
  ): Promise<PaginatedResponse<GpuResourceView>> {
    return this.request(`/gpu-resources${toQueryString(query)}`);
  }

  getFacets(): Promise<GpuResourceFacets> {
    return this.request("/gpu-resources/facets");
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
