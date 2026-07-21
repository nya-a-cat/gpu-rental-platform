# GPU Container Cloud Roadmap

路线图以可验证交付为准。勾选项表示代码、配置或契约已经进入仓库；阶段验收仍以对应 GitHub Actions、真实集群证据和认证文档为准。

## Phase 0 — 组件验证与生产基础

### 本轮已落地

- [x] 建立 Go 1.25 控制面模块、运行配置和进程生命周期基础。
- [x] 建立 PostgreSQL 迁移入口及 Operation、幂等记录、Outbox 和审计基础模型。
- [x] 建立 `/api/v1` OpenAPI 3.1 契约，以及健康、指标、系统信息和 Operation 查询边界。
- [x] 建立 Operation 与 Outbox 的事务持久化基础。
- [x] 建立 `BillingEngine`、`AuthorizationEngine`、`JobEngine` 和 OCM FleetManager 内部接口。
- [x] 建立独立 `docker-compose.v2.yml` 交付栈、隔离的项目/网络/数据卷、Go 容器构建和 GitHub Actions 运行时冒烟门禁。
- [x] 建立 OCM 1.3.1 双 kind 集群 Actions 验证脚本，覆盖 ManagedCluster、CSR、Lease 和 ManifestWork；运行证据由服务器门禁补齐。
- [x] 建立最小 GPU Platform Add-on、Hub manager Helm Chart、脱敏容量指纹和托管集群 Add-on Lease。

### Phase 0 后续

- [x] 部署 OCM Hub 并完成 ManagedCluster 双向注册、首次 CSR、证书签发和 Lease 验证。
- [x] 开发最小 GPU Platform Add-on，并通过 ManifestWork 完成安装和状态回传。
- [x] 验证 ManagedCluster 与 Add-on CSR 证书轮换。
- [x] 验证 Add-on 升级、删除清理和 N/N-1 兼容。
- [x] 收紧 Add-on Agent 跨集群库存写权限，并增加双集群反向授权断言。
- [x] 实现 Agent Epoch、单调上报序列、Fencing Token 报告与 15/45/90 秒状态求值。
- [ ] 在首个工作负载命令通道中实现命令序列、Fencing Token 校验与陈旧命令拒绝。
- [x] 固定 Kubernetes 1.34.x、OCM、GPU Operator、Volcano 和 KServe 的首个候选认证版本矩阵，并区分 GitHub-hosted 与 GPU 自托管证据。
- [x] 完成 GPUStack v2.2.1 源码与文档对照报告，建立实例生命周期、Worker Tunnel、多集群、PVC 和用量统计的 GS 验收矩阵。
- [x] 在 GitHub Actions 部署 GPUStack v2.2.1 运行基线，并执行首轮 GS 对照项。
- [x] 建立 Prometheus、OTel Collector、审计归档和基础告警链路。
- [x] 提供 Helm Chart，并完成三副本滚动升级和单副本故障验证。

## Phase 1 — Real Alpha：真实整卡实例

- [x] 建立 Tenant、Project、RoleBinding、Quota 和 `shared` 隔离。
  - [x] 建立 Tenant、Project、RoleBinding 与 Quota 的 PostgreSQL 事实、幂等 API、审计和配额预留事务。
  - [x] 通过 OCM ManifestWork 落地 Namespace、RBAC、ResourceQuota、NetworkPolicy 与 Restricted Pod Security。
- [ ] 建立 Cluster、Node、GPU、CapacityPool 和 AcceleratorProfile。
  - [x] 建立 ResourceProvider、Trait、Inventory、Generation、整卡 AcceleratorProfile 与 CapacityPool 的 PostgreSQL 事实和厂商 API。
  - [x] 扩展 GPU Platform Add-on 生成 NodePool、节点与整卡逻辑设备级脱敏库存。
  - [x] 通过 OCM Hub 的固定库存 ConfigMap 将详细快照接入控制面库存消费器。
- [x] 通过 NVIDIA GPU Operator 与 Device Plugin 交付整卡 GPU Workspace 的首个控制面切片。
  - [x] `/api/v1/instances` 提供异步创建、查询和 desiredState 更新，持久化 generation 与条件字段。
  - [x] OCM ManifestWork 构建 StatefulSet + headless Service，并使用 `nvidia.com/gpu` 资源请求。
  - [x] Workspace 创建支持 `storageGiB`，运行与停止状态保留 PVC，终止状态清理计算与卷资源。
  - [x] Workspace 支持 VolumeSnapshot 创建、查询和 OCM ManifestWork 发布。
  - [x] Workspace 运行态变更按 `gpu.nvidia.full` 项目配额原子增加或释放 allocated 容量。
  - [x] Workspace ManifestWork 自带默认拒绝、内部通信和 DNS 例外 NetworkPolicy。
- [ ] 完成实例创建、停止、启动、终止，以及 desired/observed/provisioning 状态协调。
  - [x] 将 workspace outbox 事件接入 OCM 执行协调器，回写 observed/provisioning 状态。
- [ ] 完成 PVC、快照、安全组、SSH/Jupyter/Web Terminal 访问网关。
  - [x] 为 SSH、Web Terminal、Jupyter 建立十分钟默认短期令牌发行、哈希存储、幂等重放和审计基础。
  - [x] 支持访问令牌撤销，并通过 Operation、审计和幂等事件传播撤销状态。
  - [x] 提供受认证保护的令牌 introspection 接口，供网关在建立会话前校验状态。
  - [x] OCM Workspace 清单暴露 SSH、Web Terminal、Jupyter Service 端口并生成 HTTPRoute/ReferenceGrant。
- [ ] 接入 DCGM 库存、健康指标和节点维护状态。
- [ ] 验证真实容器 `nvidia-smi`、100 次重复请求幂等、Agent 重连、Pod 驱逐和节点故障恢复。

## Phase 2 — Private Beta：多集群与商业闭环

- [ ] 建立 Region、Zone、Cluster 与 NodePool Placement。
- [ ] 建立 Domain/Reseller、Tenant/Account 和 Project 商业层级。
- [ ] 交付 `shared`、`dedicated-node-pool` 和 `dedicated-cluster` 三档隔离。
- [ ] 交付 MIG 固定规格资源池和维护流程。
- [ ] 建立 UsageFact、RatedUsage、LedgerEntry、Invoice、预算和冲正。
  - [x] 接入不可变 UsageFact、固定单价 RatedUsage、价格版本和可重放查询 API。
  - [x] RatedUsage 原子写入不可变 LedgerEntry，并提供项目账期 Invoice 生成与查询。
  - [x] 提供追加式 credit LedgerEntry 冲正/额度调整 API，保留引用号和说明。
  - [x] 提供 Project Budget 上限、余额查询和超预算 UsageFact 拒绝。
- [ ] 让 OpenMeter 完成两个完整账期的影子双算。
- [ ] 提供经过验证的 Keycloak OIDC Profile。
- [ ] 交付 Prometheus HA、Thanos、对象存储和白标厂商控制台。
- [ ] 验证三个集群调度、库存一分钟收敛、双算零差异和独占节点池隔离。

## Phase 3 — Partner Beta：批训练与分数 GPU

- [ ] 交付默认 `hpc-volcano` Profile，以及 Gang、DRF、公平共享、抢占和队列。
- [ ] 支持 JobSet、MPIJob、PyTorchJob、检查点、日志、产物和成本归属。
- [ ] 交付 HAMi 分数 GPU 可选资源池并验证限制与计量。
- [ ] 提供 `standard-kueue` 兼容 Profile。
- [ ] 强制同一 CapacityPool 只绑定一个 Scheduler Profile。
- [ ] 验证多节点 All-or-Nothing 启动和资源不足排队语义。

## Phase 4 — Release Candidate：推理与生产加固

- [ ] 交付 KServe Standard InferenceService、Gateway API 和 HPA/KEDA。
- [ ] 完成模型版本、灰度流量、并发限制、升级和回滚。
- [ ] 完成 API Key、Webhook、镜像策略、Secret 加密、签名和 SBOM。
- [ ] 完成备份恢复、Agent N/N-1、离线安装包、升级和回滚。
- [ ] 完成安装文档、运维 Runbook、API 文档和厂商接入指南。
- [ ] 验证 RPO ≤ 5 分钟、RTO ≤ 30 分钟、无高危漏洞和控制面 99.9% 可用性目标。

## Phase 5 — GPU Container Cloud GA

- [ ] 完成 10 集群、1000 GPU、1 万租户和 10 万资源对象容量验证。
- [ ] 达到 API 读取 p95 < 300 ms、异步写入受理 p95 < 500 ms。
- [ ] 达到在线集群 Operation 分发 p95 < 5 秒。
- [ ] 通过每日 100 万 UsageFact 持续写入和重算。
- [ ] 与试点厂商完成安装、升级、计费对账和故障演练。
- [ ] 发布签名 OCI 镜像、Helm Chart、SBOM、兼容矩阵和 LTS 策略。

## Phase 6 — GPU VM 产品线

- [ ] VM Alpha：KubeVirt/Harvester 整卡 PCI Passthrough。
- [ ] VM Beta：NVIDIA vGPU 和 MIG-backed vGPU。
- [ ] VM GA：Cloud-init、磁盘、网络、控制台、备份和厂商一体机交付。
- [ ] 独立验收厂商提供的 NVIDIA vGPU 商业许可证。

## 模拟产品基线历史

以下能力已经在 React、NestJS、MongoDB 与 Redis 模拟轨中交付，用作 UI 和业务流程回归基准：

- [x] 用户认证、可撤销 Redis Session 和角色保护。
- [x] 模拟 GPU 资源搜索、环境模板、订单预留和实例生命周期。
- [x] Redis 资源锁、MongoDB 唯一约束和并发预订 E2E 验证。
- [x] 模拟钱包账本、退款、SSH/API Key、防火墙、持久卷和快照。
- [x] 团队、项目、预算和成本归属。
- [x] React 中英文桌面与移动端控制台，以及 Classic/Next Pages 预览。
- [x] Docker Compose、GitHub Actions 和 GitHub Pages 发布。

模拟轨不提供物理 GPU、可访问工作负载、真实支付或生产遥测。其功能不会自动计入 v2 阶段验收。
