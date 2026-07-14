import type {
  AdminOverview,
  AuthResponse,
  CreateGpuResourceInput,
  CreateOrderInput,
  EnvironmentTemplateView,
  GpuAvailability,
  GpuListingStatus,
  GpuResourceFacets,
  GpuResourceView,
  InstanceStatus,
  InstanceView,
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

export interface InstanceQuery {
  page?: number;
  pageSize?: number;
  status?: InstanceStatus;
}

export interface DataGateway {
  readonly mode: RuntimeMode;
  cancelOrder(orderId: string): Promise<OrderView>;
  createOrder(input: CreateOrderInput): Promise<OrderView>;
  createResource(input: CreateGpuResourceInput): Promise<GpuResourceView>;
  getInstance(instanceId: string): Promise<InstanceView>;
  getAdminOverview(): Promise<AdminOverview>;
  getFacets(): Promise<GpuResourceFacets>;
  getResource(resourceId: string): Promise<GpuResourceView>;
  getSession(): Promise<UserView | null>;
  listEnvironmentTemplates(): Promise<EnvironmentTemplateView[]>;
  listAdminOrders(query?: OrderQuery): Promise<PaginatedResponse<OrderView>>;
  listAdminResources(
    query?: AdminResourceQuery,
  ): Promise<PaginatedResponse<GpuResourceView>>;
  listMyOrders(query?: OrderQuery): Promise<PaginatedResponse<OrderView>>;
  listMyInstances(
    query?: InstanceQuery,
  ): Promise<PaginatedResponse<InstanceView>>;
  listResources(
    query?: ResourceQuery,
  ): Promise<PaginatedResponse<GpuResourceView>>;
  login(input: LoginInput): Promise<AuthResponse>;
  logout(): Promise<void>;
  logoutAll(): Promise<void>;
  register(input: RegisterInput): Promise<AuthResponse>;
  resetDemo(): Promise<void>;
  startInstance(instanceId: string): Promise<InstanceView>;
  stopInstance(instanceId: string): Promise<InstanceView>;
  terminateInstance(instanceId: string): Promise<InstanceView>;
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
