# ADR-0001: Go、PostgreSQL 与 OCM 作为 v2 生产控制面基础

- 状态：Accepted
- 日期：2026-07-18
- 决策范围：GPU Container Cloud v2 控制面

## 背景

仓库现有 React、NestJS、MongoDB 与 Redis 应用已经覆盖模拟市场、订单、实例、钱包和团队流程。其执行层使用模拟 GPU 与保留域名，适合产品流程验证，无法承担真实多集群 GPU 容量、强一致资源预留、厂商级审计和生产故障恢复。

v2 需要支持云厂商私有部署、多租户商业模型、Kubernetes GPU 工作负载、可回放计费事实以及跨集群执行。集群证书、Lease、工作分发和 Add-on 生命周期已有成熟 Kubernetes 项目可以复用。

## 决策

1. 新生产控制面使用 Go 模块化单体，对外 API 固定为 `/api/v1`，契约由 OpenAPI 3.1 管理。
2. PostgreSQL 保存业务事实、幂等记录、Operation、Outbox、资源预留、分配、账本和审计；需要原子一致性的变更在单一数据库事务中完成。
3. Redis 继续用于缓存和短期协调，不作为业务事实的唯一存储。
4. OCM 负责 ManagedCluster 注册、CSR、证书轮换、Lease、Placement、ManifestWork 与 Add-on 生命周期。
5. GPU Platform Add-on 通过 Kubernetes/OCM 契约与中央控制面通信，不直接访问 PostgreSQL。
6. 现有 React 应用继续作为控制台基础，通过生成客户端逐步接入 v2 API。
7. 现有 NestJS/MongoDB 模拟版本保留为产品流程和 UI 回归基准，真实能力按领域并行替换。

## 约束

- Phase 0 首个切片只建立可验证基础；OCM 注册、GPU 调度、计费和完整租户隔离分别按后续切片验收。
- 所有状态变更使用 Idempotency-Key，长任务返回可查询 Operation。
- 中央数据库不向集群 Add-on 开放网络访问。
- 物理 GPU 标识不进入租户 API。
- 当前没有生产数据，因此不设计 MongoDB 在线迁移。未来出现试点数据时需要单独 ADR。

## 结果

### 正向结果

- PostgreSQL 事务可以同时保护配额预留、Allocation、Operation 与 Outbox。
- Go 与 Kubernetes 客户端生态适合实现 OCM adapter、控制循环和并发执行器。
- OCM 减少自研集群注册、证书、心跳和工作分发协议的范围。
- OpenAPI 为 React 客户端、厂商集成和兼容测试提供单一外部契约。
- 模块化单体保持首版部署简单，同时保留计费、授权和任务引擎的替换接口。

### 成本与风险

- 迁移期间存在两套后端，需要明确路由、状态归属和功能切换条件。
- 团队需要同时维护 Go、TypeScript 和 Kubernetes 运维能力。
- OCM、GPU Operator、Volcano 与 KServe 的版本组合必须通过固定兼容矩阵验证。
- PostgreSQL schema 演进和 Outbox 消费需要严格的幂等、重试与清理策略。

## 评估过的方案

- 继续扩展 NestJS/MongoDB：保留为模拟基准；资源预留、账本和控制循环需要额外一致性设计。
- 自研出站 Agent 协议：证书、Lease、升级和工作投递范围过大，OCM 已覆盖这些基础能力。
- 直接采用 OpenStack、CloudStack 或 OpenNebula：其 VM/IaaS 领域边界与首个 Kubernetes 容器产品不匹配；资源声明和分配模型继续作为设计参考。
- 使用 Karmada 或 Cluster API 作为首个集群层：当前需求由 OCM 满足，达到真实生命周期管理或多云编排触发条件后再评估。

## 验证要求

- Go 控制面三副本滚动升级和单副本故障通过。
- PostgreSQL 事务证明重复请求只创建一个业务资源和一个 Allocation。
- ManagedCluster 双向注册、证书轮换、Lease 和 ManifestWork 通过真实环境验证。
- 每个生产能力的切换必须有 GitHub Actions、集成测试和真实集群故障测试证据。
