import type {
  AdminOverview,
  AuthResponse,
  CreateGpuResourceInput,
  CreateOrderInput,
  GpuAvailability,
  GpuListingStatus,
  GpuResourceFacets,
  GpuResourceView,
  LoginInput,
  OrderStatus,
  OrderView,
  PaginatedResponse,
  RegisterInput,
  SetGpuListingStatusInput,
  UpdateGpuResourceInput,
  UserView,
} from "@gpu-rental/contracts";

import { ApiGateway } from "./api-gateway";
import { DemoGateway } from "./demo-gateway";

export type RuntimeMode = "api" | "demo";

export interface ResourceQuery {
  availability?: GpuAvailability;
  maxHourlyPriceCents?: number;
  memoryGb?: number;
  model?: string;
  page?: number;
  pageSize?: number;
  region?: string;
  sort?: "newest" | "priceAsc" | "priceDesc";
}

export interface AdminResourceQuery extends ResourceQuery {
  listingStatus?: GpuListingStatus;
}

export interface OrderQuery {
  page?: number;
  pageSize?: number;
  status?: OrderStatus;
}

export interface DataGateway {
  readonly mode: RuntimeMode;
  cancelOrder(orderId: string): Promise<OrderView>;
  createOrder(input: CreateOrderInput): Promise<OrderView>;
  createResource(input: CreateGpuResourceInput): Promise<GpuResourceView>;
  getAdminOverview(): Promise<AdminOverview>;
  getFacets(): Promise<GpuResourceFacets>;
  getResource(resourceId: string): Promise<GpuResourceView>;
  getSession(): Promise<UserView | null>;
  listAdminOrders(query?: OrderQuery): Promise<PaginatedResponse<OrderView>>;
  listAdminResources(
    query?: AdminResourceQuery,
  ): Promise<PaginatedResponse<GpuResourceView>>;
  listMyOrders(query?: OrderQuery): Promise<PaginatedResponse<OrderView>>;
  listResources(
    query?: ResourceQuery,
  ): Promise<PaginatedResponse<GpuResourceView>>;
  login(input: LoginInput): Promise<AuthResponse>;
  logout(): Promise<void>;
  logoutAll(): Promise<void>;
  register(input: RegisterInput): Promise<AuthResponse>;
  resetDemo(): Promise<void>;
  returnOrder(orderId: string): Promise<OrderView>;
  setListingStatus(
    resourceId: string,
    input: SetGpuListingStatusInput,
  ): Promise<GpuResourceView>;
  updateResource(
    resourceId: string,
    input: UpdateGpuResourceInput,
  ): Promise<GpuResourceView>;
}

export class GatewayError extends Error {
  constructor(
    message: string,
    readonly code: string,
    readonly status: number,
  ) {
    super(message);
    this.name = "GatewayError";
  }
}

export function createGateway(mode = resolveRuntimeMode()): DataGateway {
  return mode === "demo"
    ? new DemoGateway()
    : new ApiGateway(import.meta.env.VITE_API_BASE_URL || "/api");
}

export function resolveRuntimeMode(): RuntimeMode {
  return import.meta.env.VITE_RUNTIME_MODE === "api" ? "api" : "demo";
}
