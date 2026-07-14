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

export enum InstanceStatus {
  Provisioning = "provisioning",
  Running = "running",
  Stopped = "stopped",
  Failed = "failed",
  Terminated = "terminated",
}

export enum ConnectionMode {
  Ssh = "ssh",
  Jupyter = "jupyter",
  WebTerminal = "web-terminal",
}

export enum BillingEntryType {
  OpeningCredit = "opening-credit",
  TopUp = "top-up",
  OrderCharge = "order-charge",
  OrderRefund = "order-refund",
}

export enum NetworkProtocol {
  Tcp = "tcp",
  Udp = "udp",
}

export enum VolumeStatus {
  Available = "available",
  Attached = "attached",
  Deleted = "deleted",
}

export enum TeamRole {
  Owner = "owner",
  Admin = "admin",
  Member = "member",
}

export enum NotificationType {
  Billing = "billing",
  Instance = "instance",
  Security = "security",
  Storage = "storage",
  Team = "team",
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
  gpuCount: number;
  cpuCores: number;
  systemMemoryGb: number;
  storageGb: number;
  cudaVersion: string;
  driverVersion: string;
  bandwidthMbps: number;
  reliabilityPercent: number;
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
  gpuCount?: number;
  cpuCores?: number;
  systemMemoryGb?: number;
  storageGb?: number;
  cudaVersion?: string;
  driverVersion?: string;
  bandwidthMbps?: number;
  reliabilityPercent?: number;
  region: string;
  hourlyPriceCents: number;
  tags?: string[];
  listingStatus?: GpuListingStatus;
}

export interface UpdateGpuResourceInput {
  name?: string;
  model?: string;
  memoryGb?: number;
  gpuCount?: number;
  cpuCores?: number;
  systemMemoryGb?: number;
  storageGb?: number;
  cudaVersion?: string;
  driverVersion?: string;
  bandwidthMbps?: number;
  reliabilityPercent?: number;
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
  environmentTemplateId?: string;
  instanceName?: string;
  projectId?: string;
}

export interface OrderView {
  id: string;
  userId: string;
  gpuResourceId: string;
  gpuName: string;
  gpuModel: string;
  gpuMemoryGb: number;
  gpuCount: number;
  temporaryStorageGb: number;
  environmentTemplateId: string;
  environmentTemplateName: string;
  instanceName: string;
  projectId: string | null;
  projectName: string | null;
  teamName: string | null;
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

export interface EnvironmentTemplateView {
  id: string;
  name: string;
  description: string;
  image: string;
  category: string;
  connectionModes: ConnectionMode[];
}

export interface InstanceAccessView {
  sshCommand: string | null;
  jupyterUrl: string | null;
  webTerminalUrl: string | null;
  notice: string;
}

export interface InstanceView {
  id: string;
  orderId: string;
  userId: string;
  gpuResourceId: string;
  name: string;
  gpuName: string;
  gpuModel: string;
  gpuCount: number;
  gpuMemoryGb: number;
  temporaryStorageGb: number;
  environmentTemplateId: string;
  environmentTemplateName: string;
  environmentImage: string;
  status: InstanceStatus;
  simulated: boolean;
  startsAt: string;
  endsAt: string;
  runningSince: string | null;
  stoppedAt: string | null;
  terminatedAt: string | null;
  billableSeconds: number;
  accruedCostCents: number;
  maximumCostCents: number;
  access: InstanceAccessView;
  createdAt: string;
  updatedAt: string;
}

export interface WalletView {
  balanceCents: number;
  currency: "CNY";
  updatedAt: string;
}

export interface BillingEntryView {
  id: string;
  type: BillingEntryType;
  amountCents: number;
  reference: string;
  description: string;
  createdAt: string;
}

export interface SshKeyView {
  id: string;
  name: string;
  fingerprint: string;
  publicKey: string;
  createdAt: string;
}

export interface ApiKeyView {
  id: string;
  name: string;
  prefix: string;
  token: string | null;
  createdAt: string;
  lastUsedAt: string | null;
}

export interface NetworkRuleView {
  id: string;
  instanceId: string;
  name: string;
  protocol: NetworkProtocol;
  port: number;
  sourceCidr: string;
  simulated: true;
  createdAt: string;
}

export interface SnapshotView {
  id: string;
  name: string;
  sizeGb: number;
  createdAt: string;
}

export interface VolumeView {
  id: string;
  name: string;
  sizeGb: number;
  status: VolumeStatus;
  attachedInstanceId: string | null;
  monthlyPriceCents: number;
  snapshots: SnapshotView[];
  createdAt: string;
  updatedAt: string;
}

export interface NotificationView {
  id: string;
  type: NotificationType;
  title: string;
  message: string;
  readAt: string | null;
  createdAt: string;
}

export interface CloudAccountView {
  wallet: WalletView;
  billingEntries: BillingEntryView[];
  sshKeys: SshKeyView[];
  apiKeys: ApiKeyView[];
  networkRules: NetworkRuleView[];
  volumes: VolumeView[];
  notifications: NotificationView[];
}

export interface TopUpInput {
  amountCents: number;
}

export interface CreateSshKeyInput {
  name: string;
  publicKey: string;
}

export interface CreateApiKeyInput {
  name: string;
}

export interface CreateNetworkRuleInput {
  instanceId: string;
  name: string;
  protocol: NetworkProtocol;
  port: number;
  sourceCidr: string;
}

export interface CreateVolumeInput {
  name: string;
  sizeGb: number;
}

export interface AttachVolumeInput {
  instanceId: string;
}

export interface CreateSnapshotInput {
  name: string;
}

export interface TeamMemberView {
  userId: string;
  username: string;
  role: TeamRole;
  joinedAt: string;
}

export interface ProjectView {
  id: string;
  name: string;
  monthlyBudgetCents: number;
  bookedCostCents: number;
  createdAt: string;
}

export interface TeamView {
  id: string;
  name: string;
  currentUserRole: TeamRole;
  members: TeamMemberView[];
  projects: ProjectView[];
  createdAt: string;
  updatedAt: string;
}

export interface CreateTeamInput {
  name: string;
}

export interface AddTeamMemberInput {
  username: string;
  role: TeamRole.Admin | TeamRole.Member;
}

export interface CreateProjectInput {
  name: string;
  monthlyBudgetCents: number;
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
