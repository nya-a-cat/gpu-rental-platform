# GPUStack v2.2.1 Phase 0 对照基准

## 业务结论

GPUStack v2.2.1 已提供可参考的 Kubernetes GPU Instance、持久卷、Worker Tunnel、多集群入口、组织访问控制和用量统计能力。源码中的统一 Principal 模型覆盖 `USER`、`ORG`、`GROUP`、`SYSTEM`，组织成员支持 `OWNER` 与 `MEMBER`，集群 ACL 可以授权给用户、组织或用户组。

GPU Container Cloud 的差异化验收集中在 `Domain / Reseller → Tenant → Project` 商业层级、三副本高可用控制面、OCM Fleet、显式 Reservation/Allocation、统一异步 Operation、三档隔离、财务账本与发票、Volcano 训练调度。当前报告已完成来源核验和首轮服务器运行基线：GS-00 服务器基线已通过，GS-04、GS-07、GS-08、GS-09、GS-10 已验证服务端 API；真实 Worker、GPU、实例、Tunnel、PVC 和用量产生仍待后续环境。

基准版本固定在 [GPUStack v2.2.1](https://github.com/gpustack/gpustack/releases/tag/v2.2.1)，后续版本变更需要新建对照记录。

## 来源核验范围

### 身份、组织与 ACL

- [Principal 源码](https://github.com/gpustack/gpustack/blob/v2.2.1/gpustack/schemas/principals.py)定义 `USER`、`ORG`、`GROUP`、`SYSTEM` 四类统一身份。资源通过 `owner_principal_id` 记录所有者，成员关系和 ACL 直接引用 Principal。
- 同一源码定义组织角色 `OWNER` 与 `MEMBER`。用户组加入组织后，组内活跃用户继承该成员关系的组织角色。
- [组织成员路由](https://github.com/gpustack/gpustack/blob/v2.2.1/gpustack/routes/organization_members.py)允许平台管理员或组织 Owner 管理用户和用户组成员，并保护最后一个有效 Owner。
- [集群 ACL 路由](https://github.com/gpustack/gpustack/blob/v2.2.1/gpustack/routes/cluster_access.py)允许集群所有者组织的 Owner 或平台管理员向 `USER`、`ORG`、`GROUP` 授权；仅持有访问授权的组织不能继续转授。
- `SYSTEM` Principal 用于集群和 Worker 的内部服务身份，其生命周期由基础设施关联承担。

这些来源证明 GPUStack v2.2.1 具备组织级身份与集群访问授权模型。它们没有形成 GPU Container Cloud 计划中的经销商层级、Project 资源边界和五级 Scope 角色验收证据。

### GPU Instance 与多集群

[GPU Service Instances 文档](https://github.com/gpustack/gpustack/blob/v2.2.1/docs/user-guide/gpuservice-instances.md)记录以下产品行为：

- GPU Instance 由 Kubernetes Pod 承载，可使用单卡或多卡，也支持 CPU-only 实例类型。
- GPUStack 通过统一界面管理多个 Kubernetes 集群并创建 GPU Instance。
- GPUStack Operator 自动发现设备并生成 Instance Type，规格包含厂商、显存、CPU、内存、操作系统和架构。
- 实例支持 SSH、日志、事件、停止、启动和删除。
- 停止会释放计算资源；再次启动会重建实例并分配新 IP；临时存储数据随重建丢失。

以上行为是 Phase 1 GPU Workspace 生命周期的重要业务基准。运行时正确性、重复请求幂等、节点故障恢复和计费一致性需要在 GS 验收中实测。

### PVC 与持久化

[GPU Service Storage 文档](https://github.com/gpustack/gpustack/blob/v2.2.1/docs/user-guide/gpuservice-storage.md)说明 Storage 在目标 Kubernetes 集群中实现为 PersistentVolumeClaim，可挂载到多个 GPU Service Instance。删除已挂载 Storage 时采用延迟清理，关联实例删除后才清理 PVC。

该行为提供共享持久卷与实例重建的数据保留基准。来源同时记录同名 Storage 重建可能受到残留 PVC 影响，GS 验收需要覆盖删除、延迟清理、重建和冲突场景。

### Worker Tunnel

[GPU Service Instances 文档的集群接入章节](https://github.com/gpustack/gpustack/blob/v2.2.1/docs/user-guide/gpuservice-instances.md#ensure-the-gpustack-worker-is-reachable)说明 `proxy_mode=tunnel` 会保持 Worker 到 GPUStack Server 的长连接，为 Server 访问集群提供隧道。该出站连接模型适合 NAT 或防火墙后的集群。

GPU Container Cloud 采用 OCM 负责注册、CSR、证书、Lease 和 ManifestWork，并由 GPU Platform Add-on 承担库存、执行、UsageFact 与访问隧道。GS 验收需要分别记录管理通道和用户访问通道的断线、重连、证书轮换及 Fencing 行为。

### 用量统计

[Usage 文档](https://github.com/gpustack/gpustack/blob/v2.2.1/docs/user-guide/usage.md)记录 Token、GPU/CPU Instance 和 Storage 三类用量：

- GPU Instance 按运行阶段计量，支持 Instance Hours 与 GPU Hours。
- Storage 从创建到删除持续计量，支持 GB-Hours 与 GB-Days。
- Resource Events 保存计量相关的生命周期事件。
- 管理员可查看全局并按用户过滤，普通用户查看自己的用量。
- 已删除资源保留历史用量；汇总数据默认保留约十三个月并可配置归档。

这些能力构成 UsageFact 和用量查询的产品基准。GPU Container Cloud 还需交付不可变 UsageFact、RatedUsage、LedgerEntry、Invoice、价格版本、冲正、预算和影子双算。

## 差异化交付边界

| 领域         | GPUStack v2.2.1 来源基线                                      | GPU Container Cloud 必须自证的交付                                                                                  |
| ------------ | ------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| 商业层级     | Principal、组织、用户组、组织角色和集群 ACL                   | `System → Domain / Reseller → Tenant / Account → Project`，以及 System、Domain、Tenant、Project、Cluster Scope 角色 |
| 控制面高可用 | 本轮来源核验未获得三副本滚动升级和单副本故障验收证据          | 三副本 Go 控制面、PostgreSQL 高可用部署约束、滚动升级、单副本故障和 99.9% 目标                                      |
| 集群 Fleet   | GPUStack Server、Worker、Operator 和可选 Tunnel               | OCM ManagedCluster、CSR、证书轮换、Lease、Placement、ManifestWork、Addon Framework                                  |
| 容量一致性   | Operator 发现设备并生成 Instance Type                         | ResourceProvider、Trait、Inventory Generation、Reservation、Allocation、DeviceClaim 和提交后配额复核                |
| 异步执行     | GPU Instance 生命周期与控制器状态                             | HTTP 202、Operation ID、步骤、进度、Deadline、Retryable、父 Operation、结构化错误和审计关联                         |
| 租户隔离     | 组织所有权与集群访问 ACL                                      | `shared`、`dedicated-node-pool`、`dedicated-cluster`，以及 NetworkPolicy、Pod Security、Taint、CapacityPool 验收    |
| 财务闭环     | GPU Hours、Instance Hours、GB-Hours、Token 和 Resource Events | 不可变 UsageFact、RatedUsage、LedgerEntry、Invoice、价格版本、预算、冲正与 OpenMeter 影子双算                       |
| 训练调度     | 来源文档重点覆盖 GPU Instance 和推理工作负载                  | Volcano Gang、DRF、公平共享、抢占、队列、多节点训练和检查点                                                         |

“本轮来源核验未获得”表示本报告没有形成相应证据，不用于判断 GPUStack 的全部能力范围。

## GS-00 至 GS-15 验收矩阵

状态使用 `运行基线通过`、`服务端 API 已验证`、`来源核验` 与 `未执行`。`运行基线通过`表示该项的服务器级验收证据完整；`服务端 API 已验证`表示真实服务进程上的协议表面和认证访问已通过，相关 Kubernetes/GPU 行为仍待执行；`来源核验`表示官方 tag 下的源码或文档能够确认产品设计。

| ID    | 验收主题           | GPUStack v2.2.1 基准                                        | GPU Container Cloud 对照目标                                    | 验证方式                                          | 当前状态          |
| ----- | ------------------ | ----------------------------------------------------------- | --------------------------------------------------------------- | ------------------------------------------------- | ----------------- |
| GS-00 | 版本与可复现性     | v2.2.1 tag 和 release 可定位                                | 固定依赖、发行物摘要、配置快照和证据归档                        | 记录版本、发行物、配置、时间和环境指纹            | 运行基线通过      |
| GS-01 | Principal 类型     | `USER`、`ORG`、`GROUP`、`SYSTEM`                            | System、Domain、Tenant、Project 与 Service Account 映射明确     | API 创建、查询、禁用、删除和审计                  | 来源核验          |
| GS-02 | 组织成员与角色     | `OWNER`、`MEMBER`，Group 角色可传递给活跃用户               | Domain/Reseller、Tenant、Project 的 Scope RoleBinding           | 用户、组、跨层级角色和最后 Owner 保护测试         | 来源核验          |
| GS-03 | 集群 ACL           | USER、ORG、GROUP 可获集群访问，所有者控制转授               | Cluster Scope 授权、撤销、审计和最小权限                        | 正向、越权、撤销、缓存失效和重放测试              | 来源核验          |
| GS-04 | 多集群入口         | 统一界面管理多个 Kubernetes 集群                            | OCM ManagedCluster、Region、Zone、Cluster 与 Placement          | 三集群注册、断线、恢复和库存收敛                  | 服务端 API 已验证 |
| GS-05 | GPU 库存与规格     | Operator 自动发现 GPU 并生成 Instance Type                  | Inventory Generation、Trait、AcceleratorProfile 和 CapacityPool | 物理清单对账、更新冲突和陈旧库存测试              | 来源核验          |
| GS-06 | 创建幂等           | 需要运行时对照                                              | 100 次同键请求产生一个 Instance、Reservation 和 Allocation      | 并发 API、数据库和集群对象计数                    | 未执行            |
| GS-07 | 实例生命周期       | 创建、日志、事件、停止、启动、删除；启动重建实例            | desired、observed、provisioning 状态与 Operation 全链路         | 逐状态执行并核对 Pod、PVC、Operation、审计        | 服务端 API 已验证 |
| GS-08 | SSH 与 Tunnel      | SSH 地址；Worker `proxy_mode=tunnel` 出站长连接             | 十分钟访问令牌、统一网关、管理通道与访问通道分离                | NAT 环境连接、过期、撤销、断线重连和审计          | 服务端 API 已验证 |
| GS-09 | PVC 生命周期       | PVC 持久卷、跨实例挂载、延迟删除                            | Volume、Snapshot、停止保留、删除策略和冲突保护                  | 写入数据后停止、启动、删除、重建和快照恢复        | 服务端 API 已验证 |
| GS-10 | 用量事实           | GPU Hours、Instance Hours、GB-Hours、Token、Resource Events | UsageFact、RatedUsage、LedgerEntry、Invoice 和冲正              | 重复、乱序、迟到、重放、价格版本和对账            | 服务端 API 已验证 |
| GS-11 | Worker/Add-on 故障 | 需要运行时对照                                              | Agent Epoch、序列号、Fencing Token、15/45/90 秒状态语义         | 进程终止、网络隔离、证书轮换和 N/N-1 测试         | 未执行            |
| GS-12 | 控制面高可用       | 需要运行时对照                                              | 三副本滚动升级、单副本故障、RPO/RTO 与 99.9% 目标               | 故障注入、数据库切换、流量和 Operation 连续性测试 | 未执行            |
| GS-13 | 三档租户隔离       | 组织所有权和集群 ACL 来源已确认                             | shared、dedicated-node-pool、dedicated-cluster                  | Pod、Secret、网络、节点和集群边界渗透测试         | 未执行            |
| GS-14 | 商业账本与发票     | 用量统计和 Resource Events 来源已确认                       | 价格版本、预算、账本、发票、冲正和 OpenMeter 双算               | 两个账期影子双算与零差异对账                      | 未执行            |
| GS-15 | Volcano 训练       | 需要运行时对照                                              | Gang、DRF、队列、公平共享、抢占和多节点训练                     | 资源不足排队、All-or-Nothing、抢占和恢复测试      | 未执行            |

## 执行顺序

1. 在 GitHub Actions 固定部署 GPUStack v2.2.1 可运行基线，并保存版本、API 和对象快照。
2. 先执行 GS-00、GS-04、GS-07、GS-08、GS-09、GS-10，形成实例、Tunnel、PVC 和用量的首轮对照证据。
3. GPU Container Cloud 完成 OCM 与最小 Add-on 后执行 GS-04、GS-05、GS-11。
4. Real Alpha 进入真实 GPU 环境后执行 GS-06、GS-07、GS-08、GS-09。
5. Private Beta 至 Partner Beta 依次执行 GS-02、GS-03、GS-10、GS-12、GS-13、GS-14、GS-15。

任一 GS 项只有在命令、日志、对象快照、失败诊断和清理结果齐全时才能登记通过。

## GitHub Actions 运行基线

仓库已加入 `gpustack-baseline` 作业及 `deploy/gpustack` 固定版本配置。该作业使用 GPUStack v2.2.1 官方 wheel、上游 `uv.lock` 导出的精确依赖和 GitHub Ubuntu 24.04 预装 PostgreSQL，执行服务启动、管理员登录、API 表面检查、集合读取和数据库持久化重启检查。

首轮成功证据来自 Pipeline `29713314184`、job `88261048162`、commit `aed41cb339af1965d5787db10d5a227df331ab8d` 和 Artifact `8449510409`。完整证据策略检查 12 个文件且违规为 0，SHA-256 清单逐文件复核通过。

运行环境为 GPUStack v2.2.1 commit `9e9f841`、Python 3.12.13、uv 0.9.6 和 PostgreSQL 16.14。官方 wheel 校验通过，实际安装 135 个包，最大缓存文件为 63,816,496 bytes，低于 100 MiB 边界。服务启动两次，重启前后管理员 ID 均为 `3`，健康、就绪、登录、集合访问和持久化断言均通过。

首轮覆盖范围限定为 GS-00 运行来源，以及 GS-04、GS-07、GS-08、GS-09、GS-10 的服务端 API 可达性。实例创建、Worker Tunnel、PVC 创建、GPU 用量产生和多集群调度仍需要 Kubernetes Worker 与 GPU 环境，继续保留在 Real Alpha 和 GPU 自托管门禁。
