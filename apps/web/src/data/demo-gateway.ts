import {
  GpuAvailability,
  GpuListingStatus,
  OrderStatus,
  ResourceMode,
  UserRole,
  type AdminOverview,
  type AuthResponse,
  type CreateGpuResourceInput,
  type CreateOrderInput,
  type GpuResourceFacets,
  type GpuResourceView,
  type LoginInput,
  type OrderView,
  type PaginatedResponse,
  type RegisterInput,
  type SetGpuListingStatusInput,
  type UpdateGpuResourceInput,
  type UserView,
} from "@gpu-rental/contracts";

import type {
  AdminResourceQuery,
  DataGateway,
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
  credentials: Record<string, string>;
  currentUserId: string | null;
  orders: OrderView[];
  resources: GpuResourceView[];
  sequence: number;
  users: UserView[];
  version: 1;
}

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
    const startsAt = this.now();
    const timestamp = startsAt.toISOString();
    const order: OrderView = {
      id: `demo-order-${state.sequence++}`,
      userId: user.id,
      gpuResourceId: resource.id,
      gpuName: resource.name,
      gpuModel: resource.model,
      gpuMemoryGb: resource.memoryGb,
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
    state.orders.unshift(order);
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
    this.write(state);
    return order;
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
      if (state.version !== 1) throw new Error("Unsupported demo state");
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
  const order: OrderView = {
    id: "demo-order-01",
    userId: users[0]!.id,
    gpuResourceId: activeResource.id,
    gpuName: activeResource.name,
    gpuModel: activeResource.model,
    gpuMemoryGb: activeResource.memoryGb,
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
  return {
    version: 1,
    sequence: 100,
    currentUserId: null,
    users,
    resources,
    orders: [order],
    credentials: {
      "demo-user": DEMO_PASSWORD_HASH,
      "demo-admin": DEMO_PASSWORD_HASH,
    },
  };
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
      changed = true;
    }
  }
  return changed;
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
