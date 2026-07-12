export enum UserRole {
  User = "user",
  Admin = "admin",
}

export enum ResourceMode {
  Simulated = "simulated",
}

export enum GpuListingStatus {
  Online = "online",
  Offline = "offline",
  Maintenance = "maintenance",
}

export enum GpuAvailability {
  Available = "available",
  Rented = "rented",
}

export enum OrderStatus {
  Active = "active",
  Returned = "returned",
  Expired = "expired",
  Cancelled = "cancelled",
}

export interface UserView {
  id: string;
  username: string;
  role: UserRole;
  createdAt: string;
  updatedAt: string;
}

export interface AuthResponse {
  user: UserView;
}

export interface RegisterInput {
  username: string;
  password: string;
}

export interface LoginInput {
  username: string;
  password: string;
}

export interface ChangePasswordInput {
  currentPassword: string;
  newPassword: string;
}

export interface GpuResourceView {
  id: string;
  name: string;
  model: string;
  memoryGb: number;
  region: string;
  hourlyPriceCents: number;
  tags: string[];
  resourceMode: ResourceMode;
  listingStatus: GpuListingStatus;
  availability: GpuAvailability;
  createdAt: string;
  updatedAt: string;
}

export interface CreateGpuResourceInput {
  name: string;
  model: string;
  memoryGb: number;
  region: string;
  hourlyPriceCents: number;
  tags?: string[];
  listingStatus?: GpuListingStatus;
}

export interface UpdateGpuResourceInput {
  name?: string;
  model?: string;
  memoryGb?: number;
  region?: string;
  hourlyPriceCents?: number;
  tags?: string[];
}

export interface SetGpuListingStatusInput {
  listingStatus: GpuListingStatus;
}

export interface GpuResourceFacets {
  models: string[];
  regions: string[];
  memoryGbValues: number[];
  maxHourlyPriceCents: number;
}

export interface CreateOrderInput {
  gpuResourceId: string;
  durationHours: number;
}

export interface OrderView {
  id: string;
  userId: string;
  gpuResourceId: string;
  gpuName: string;
  gpuModel: string;
  gpuMemoryGb: number;
  region: string;
  hourlyPriceCents: number;
  durationHours: number;
  totalPriceCents: number;
  status: OrderStatus;
  startsAt: string;
  endsAt: string;
  returnedAt: string | null;
  cancelledAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface AdminOverview {
  usersTotal: number;
  resourcesTotal: number;
  resourcesOnline: number;
  activeOrders: number;
  terminalOrders: number;
  bookedRevenueCents: number;
}

export interface PaginatedResponse<T> {
  items: T[];
  page: number;
  pageSize: number;
  total: number;
}

export interface ApiErrorResponse {
  code: string;
  message: string;
  requestId: string;
}

export interface HealthResponse {
  status: "ok";
}
