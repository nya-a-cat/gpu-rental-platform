# Kubernetes 1.34 首个认证矩阵

## 业务结论

Phase 0 固定 Kubernetes 1.34.x 为首个生产认证线，生产目标为 Kubernetes 1.34.9。GitHub Actions 使用 kind 0.32.0 官方发布的 Kubernetes 1.34.8 节点镜像及完整摘要。GitHub-hosted CI 集成、ManagedCluster 与 Add-on 客户端证书轮换，以及控制面三副本 Helm 生命周期与单副本故障恢复均已通过。生产目标 patch、GPU 硬件和厂商交付仍为 `unverified`。

本文件定义版本选择、上游依据和项目自证范围。机器可读版本位于 [`config/certification/versions.yaml`](../../config/certification/versions.yaml)。版本进入生产支持清单需要同时满足对应的 GitHub Actions 门禁和 GPU 自托管认证门禁。

## 固定版本

| 层级                      | 固定版本                                                                                       | 用途                                                        | 当前状态 |
| ------------------------- | ---------------------------------------------------------------------------------------------- | ----------------------------------------------------------- | -------- |
| Kubernetes 生产目标       | 1.34.9                                                                                         | 厂商安装、升级、回滚和故障认证                              | 未执行   |
| GitHub Actions Kubernetes | 1.34.8                                                                                         | kind 上的 OCM、ManifestWork、Add-on 和控制器集成            | 已通过   |
| kind                      | 0.32.0                                                                                         | GitHub-hosted runner 的可复现 Kubernetes 环境               | 来源核验 |
| kind 节点镜像             | `kindest/node:v1.34.8@sha256:02722c2dedddcfc00febf5d27fbeb9b7b2c14294c82109ff4a85d89ac9ba3256` | 固定 Actions 节点镜像内容                                   | 来源核验 |
| Actions 客户端证书时长    | 7m                                                                                             | 在临时 kind Hub 内触发 OCM 原生证书轮换                     | 已通过   |
| Open Cluster Management   | 1.3.1                                                                                          | Hub、ManagedCluster、注册、Lease、Placement 和 ManifestWork | 来源核验 |
| clusteradm                | 1.3.1                                                                                          | Actions 中初始化 Hub 和导入测试集群                         | 来源核验 |
| kubectl                   | 1.34.8                                                                                         | Actions 中操作固定 Kubernetes 1.34.8 集群                   | 来源核验 |
| Helm                      | 3.19.0                                                                                         | 渲染 Hub manager Chart 并安装 Add-on manager                | 来源核验 |
| OCM API                   | 1.3.0                                                                                          | Go 控制面与 Add-on 使用的 API 和客户端                      | 来源核验 |
| OCM Addon Framework       | 1.3.0                                                                                          | GPU Platform Add-on 的注册、安装和状态管理                  | 来源核验 |
| NVIDIA GPU Operator       | 26.3.3                                                                                         | 驱动、Device Plugin、GFD、MIG、DCGM                         | 来源核验 |
| Volcano                   | 1.15.0                                                                                         | `hpc-volcano` Scheduler Profile                             | 来源核验 |
| KServe                    | 0.19.0                                                                                         | Standard InferenceService 生产路径                          | 来源核验 |

## 上游兼容依据

| 组件                  | 官方依据                                                                                                                                                                                                                                                       | 对本项目的含义                                                                                                   |
| --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Kubernetes 1.34.9     | Kubernetes 官方发布了 [v1.34.9](https://github.com/kubernetes/kubernetes/releases/tag/v1.34.9)，补丁明细进入 [CHANGELOG-1.34](https://github.com/kubernetes/kubernetes/blob/v1.34.9/CHANGELOG/CHANGELOG-1.34.md)。                                             | 作为生产认证目标；安装、升级、回滚和故障场景由本项目留证。                                                       |
| kind 0.32.0           | kind [v0.32.0 发布说明](https://github.com/kubernetes-sigs/kind/releases/tag/v0.32.0)列出 Kubernetes 1.34.8 的预构建多架构节点镜像，并要求使用摘要固定镜像。                                                                                                   | GitHub Actions 使用官方镜像摘要复现 OCM 和控制器集成。                                                           |
| kubectl 1.34.8        | Kubernetes 官方 [下载索引](https://dl.k8s.io/)发布与集群 patch 版本一致的 Linux amd64 客户端及 SHA-256。                                                                                                                                                       | Actions 使用与 kind 节点一致的客户端版本，避免 runner 预装版本漂移。                                             |
| Helm 3.19.0           | Helm [v3.19.0 发布说明](https://github.com/helm/helm/releases/tag/v3.19.0)提供固定 Linux amd64 归档与 SHA-256，并将 Kubernetes 依赖线更新到 1.34。                                                                                                             | Chart lint、render 与 OCM manager 安装使用同一固定客户端。                                                       |
| OCM 1.3.1             | OCM [v1.3.1](https://github.com/open-cluster-management-io/ocm/releases/tag/v1.3.1)是 1.3 线修复版本；clusteradm [v1.3.1](https://github.com/open-cluster-management-io/clusteradm/releases/tag/v1.3.1)同步升级到该版本。                                      | Hub 与管理 CLI 保持同一补丁线。ManagedCluster 注册、CSR、Lease 和 ManifestWork 仍由本项目 Actions 验证。         |
| OCM API 1.3.0         | OCM API [v1.3.0](https://github.com/open-cluster-management-io/api/releases/tag/v1.3.0)升级 Kubernetes 库到 1.35；其 [README](https://github.com/open-cluster-management-io/api/blob/v1.3.0/README.md)声明 Kubernetes 1.28+。                                  | Kubernetes 1.34 落在上游声明的最低范围内；精确组合由本项目集成测试确认。                                         |
| Addon Framework 1.3.0 | Addon Framework [v1.3.0](https://github.com/open-cluster-management-io/addon-framework/releases/tag/v1.3.0)升级 Kubernetes 库到 1.35，并固定 OCM API 1.3.0。                                                                                                   | Add-on 使用同一 OCM API 线；注册、证书、Lease、安装和状态回传进入 Actions 门禁。                                 |
| GPU Operator 26.3.3   | NVIDIA [26.3.3 发布说明](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/release-notes.html#gpu-operator-v26-3-3)固定 Device Plugin 与 GFD 0.19.3。该发布线的说明记录 Kubernetes 1.34 支持，并在 26.3.2 扩展到 Kubernetes 1.36。           | Kubernetes 1.34 属于该发布线支持范围。驱动、内核、容器运行时、真实 GPU 和 DCGM 行为需要 GPU 自托管 runner 验证。 |
| Volcano 1.15.0        | Volcano v1.15 的 [兼容矩阵](https://github.com/volcano-sh/volcano/blob/v1.15.0/README.md#kubernetes-compatibility)明确标记 Kubernetes 1.34 兼容；[v1.15.0 发布说明](https://github.com/volcano-sh/volcano/releases/tag/v1.15.0)同时记录 Kubernetes 1.35 支持。 | Actions 可验证 CRD、Webhook、Scheduler 和无 GPU Gang 语义；多节点 GPU 训练进入 GPU 集群验收。                    |
| KServe 0.19.0         | KServe [v0.19.0](https://github.com/kserve/kserve/releases/tag/v0.19.0)为固定发布版本；其 [go.mod](https://github.com/kserve/kserve/blob/v0.19.0/go.mod)固定 Kubernetes API、apimachinery 和 client-go 0.34.5。                                                | 依赖线与 Kubernetes 1.34 对齐。Standard InferenceService、Gateway、扩缩容和 GPU 推理需要本项目验证。             |

上游证据用于版本选型。它没有覆盖本产品的完整安装拓扑、配置组合、升级路径、厂商操作系统、GPU 型号和故障语义。

## GitHub-hosted Actions 验证范围

GitHub-hosted runner 没有 NVIDIA GPU。当前 Phase 0 门禁与后续扩展项分开记录。

### Phase 0 当前门禁

- 校验 `versions.yaml` 与 `deploy/ocm/versions.env` 的执行版本一致。
- 通过 SHA-256 固定 kind、clusteradm、kubectl 与 Helm，并记录客户端版本。
- 使用 kind 0.32.0 和固定摘要创建 Hub 与 Managed Cluster。
- 验证 OCM 双向注册、首次 CSR 批准与证书签发、ManagedCluster 条件和 Lease 续期。
- 验证 ManifestWork 下发及 smoke ConfigMap 到达托管集群。
- 验证 GPU Platform Add-on 注册、安装、Lease 健康和脱敏容量指纹上报。
- 验证控制面 Helm Chart 的三副本部署、迁移 Hook、PDB、安全上下文、Secret 轮换、滚动升级、失败升级回滚、单副本强制故障、共享持久化和卸载清理。
- 执行 Add-on Go 格式、vet、单元测试、构建、Helm lint/render 与 shell 语法检查。

运行 `29694658483` 已成功完成：Quality job `88213213524`、OCM conformance job `88213213557` 和 Pages job `88214243600` 均通过。OCM job 上传 artifact `ocm-conformance-48a58987f845cdb21431a9e49862330b8029ba12`，证明临时 Hub 的 `7m` 签发上限实际生效。ManagedCluster 和 Add-on 均创建了新的自动批准 CSR，Secret UID 保持稳定，Secret resourceVersion、客户端证书、私钥、证书序列号和指纹均完成更新，轮换重叠时间分别为 115 秒和 118 秒。两个 Agent Pod 在轮换期间保持原 Pod UID 且重启次数为 0；旧证书过期后，Hub kube-apiserver 容器已重建，ManagedCluster Lease、Add-on Lease 和库存上报继续推进，独立临时凭据 API 检查通过。

Actions 产物保留完整 smoke 日志、客户端与镜像版本、ManagedCluster、ManifestWork、ManagedClusterAddOn、Lease、Deployment、库存 ConfigMap，以及脱敏后的 CSR/Secret 元数据和证书轮换摘要。上传内容排除 CSR 请求体、CSR `status.certificate` 客户端证书正文、Secret data、kubeconfig 和私钥。artifact 复核确认 CSR 仅保留批准条件，Secret 仅保留元数据和字段名。GPU 硬件和生产认证保持未执行。

### 后续扩展门禁

- 增加 Add-on 升级、N/N-1、删除清理和陈旧库存回收验证。
- 增加多集群 Add-on 凭据反向授权断言，验证每个 Agent 仅能写入所属 ManagedCluster 命名空间。
- 增加控制面与 OCM API 的幂等、重试、超时、Fencing 和结构化错误验证。
- 安装 Volcano 与 KServe 控制器，验证 CRD、Webhook、调度器和 CPU smoke workload。
- 使用无 GPU fixture 验证 Inventory、Reservation、Allocation 和 UsageFact 数据流。
- 在测试框架产出后增加 JUnit、Helm 渲染包和更完整的控制器诊断快照。

## GPU Platform Add-on lifecycle compatibility

The GitHub-hosted lifecycle profile pins current Add-on `0.2.0` and N-1 Add-on `0.1.0` at revision `074046e1138f190ac8b90b5e10548c0a27cce975`. It uses the same Kubernetes 1.34.8, OCM 1.3.1, OCM API 1.3.0 and Addon Framework 1.3.0 matrix. A `30m` temporary client-certificate signing duration isolates lifecycle assertions from the dedicated `7m` certificate-rotation profile.

The lifecycle gate requires distinct Add-on source-tree hashes and immutable revision tags. Its acceptance scope includes idempotent installation, both current/N-1 manager-agent combinations, agent rollback, stable active credentials during upgrades, stale inventory garbage collection, per-cluster deletion, re-enablement, Helm uninstall and final reinstall. The evidence artifact must pass the repository evidence policy before upload.

The lifecycle profile is verified by [Pipeline `29700482298`](https://github.com/nya-a-cat/gpu-rental-platform/actions/runs/29700482298) for commit `c9d9655057adac61ecf7083f610981e69e63ec27`. Quality job `88228601999` completed in 3m06s, OCM conformance job `88228601989` completed in 12m28s, lifecycle job `88228601978` completed in 14m44s, and Pages job `88229892975` deployed successfully. The lifecycle run passed idempotent installation, both current/N-1 manager-agent combinations, agent rollback, stable credentials, per-cluster deletion, Lease/Secret/RBAC/inventory cleanup, re-enablement, Helm uninstall, final reinstall and pre-upload evidence policy checks.

Lifecycle artifact `8446431580` (`ocm-addon-lifecycle-c9d9655057adac61ecf7083f610981e69e63ec27`) and conformance artifact `8446410025` (`ocm-conformance-c9d9655057adac61ecf7083f610981e69e63ec27`) were downloaded and reviewed. Both SHA-256 manifests matched every file, both evidence-policy reports were `passed`, lifecycle and policy exit codes were zero, and the current/N-1 source trees and two-image provenance were distinct. The review found no private-key or kubeconfig markers, runner or Docker storage paths, node identity fields, CSR request/certificate/free-text fields, or Secret data. GPU hardware certification remains unexecuted.

## Dual-cluster Add-on authorization and agent session semantics

[Pipeline `29737714428`](https://github.com/nya-a-cat/gpu-rental-platform/actions/runs/29737714428) verified commit `a8b659c6fa72e5868ed57497c484420e4d58667c`. Quality job `88336898498`, OCM conformance job `88336898506`, Add-on lifecycle job `88336898582`, control-plane HA job `88336898524`, GPUStack job `88336898622`, observability job `88336898459` and Pages job `88339880003` all completed successfully.

OCM conformance artifact `8459299601` (`ocm-conformance-a8b659c6fa72e5868ed57497c484420e4d58667c`) passed its evidence policy. Two managed clusters used their issued Add-on client credentials to read, update and create inventory objects in their own Hub namespaces. The same get, update and create operations were denied in the other cluster namespace in both directions. The Hub RoleBinding contains the exact cluster-specific registration user, preserving per-cluster authorization boundaries.

The inventory reports a random 128-bit Agent Epoch, a monotonic report sequence and the ManagedClusterAddOn UID as its fencing token. Lifecycle artifact `8459377791` (`ocm-addon-lifecycle-a8b659c6fa72e5868ed57497c484420e4d58667c`) passed current/N-1 manager-agent combinations, rollback, deletion, re-enablement and final reinstall. It verified the compatibility state where a current Agent managed by the N-1 manager reports fencing disabled, then restores the current UID token after the current manager resumes.

The control plane now exposes configurable 15-second heartbeat, 45-second degraded/inventory-stale and 90-second offline thresholds. Its evaluator derives `Connected`, `Schedulable`, `InventoryFresh` and `ExecutionHealthy` independently. Command sequence and fencing-token enforcement will be certified with the first workload command channel. Persisted cluster Conditions remain part of the Phase 1 resource API work.

## GPU control plane Helm and high availability

The Phase 0 control-plane profile deploys the Go API as three replicas with a `RollingUpdate` strategy, `maxUnavailable: 0`, `maxSurge: 1`, a `minAvailable: 2` PodDisruptionBudget, restricted security contexts and an external PostgreSQL credential Secret. Database migrations run through a Helm pre-install/pre-upgrade hook. Secret revision changes explicitly roll the Pods while retaining the externally managed Secret.

[Pipeline `29711315388`](https://github.com/nya-a-cat/gpu-rental-platform/actions/runs/29711315388) verified commit `65074eb5ce832e9465ffcc262264a71ffc8278f2`. Quality job `88255544699`, OCM conformance job `88255544708`, Add-on lifecycle job `88255544719`, control-plane HA job `88255544721` and Pages job `88256460224` all completed successfully. The HA profile used kind 0.32.0 with Kubernetes 1.34.8 and distinct baseline/candidate container image IDs.

HA artifact `8449023806` (`gpu-control-plane-ha-65074eb5ce832e9465ffcc262264a71ffc8278f2`) passed the full evidence policy across 61 files. Its SHA-256 manifest matched every file. Assertions covered migration `000001_phase0_foundation`, PDB and workload security, external Secret rotation, rejected invalid upgrade, rolling image upgrade, endpoint continuity, shared PostgreSQL persistence, zero-grace single-Pod failure recovery, Helm uninstall and post-run cleanup. Rolling upgrade and forced Pod failure recorded zero probe failures; ready endpoints stayed at three during the rolling upgrade and at least two during the single-Pod failure.

This result certifies the repository's Phase 0 Helm lifecycle on ephemeral Kubernetes 1.34.8 kind infrastructure. PostgreSQL uses the CI fixture image `postgres:17`. Kubernetes 1.34.9, production PostgreSQL topology, real GPU hardware, GPU Operator and vendor operating-system combinations remain unverified.

## GPU 自托管认证范围

带 NVIDIA GPU 的自托管 runner 或认证实验集群负责硬件相关门禁：

- 固定 Linux 发行版、内核、containerd、NVIDIA 驱动、GPU 型号和节点固件信息。
- 安装 GPU Operator 26.3.3，等待 ClusterPolicy、Driver、Toolkit、Device Plugin、GFD 和 DCGM Exporter 就绪。
- 在真实容器中执行 `nvidia-smi`，验证整卡申请、释放和重新申请。
- 验证节点标签、GPU 库存、显存、健康状态和 DCGM 指标与物理设备一致。
- 验证驱动 Pod、Device Plugin、Add-on、节点和 Kubernetes API 故障后的恢复语义。
- 验证 GPU Operator、Kubernetes patch 和 Add-on 的升级、N/N-1 兼容及回滚。
- 在进入 Private Beta 前验证 MIG 配置、维护、重建和计量。
- 在进入 Partner Beta 前验证 Volcano 多节点 Gang、队列、公平共享、抢占和检查点恢复。
- 在进入 Release Candidate 前验证 KServe GPU 推理、灰度、回滚、HPA/KEDA 和 Gateway 路径。

硬件门禁要求上传集群版本、GPU 清单、测试日志、指标快照、故障时间线和清理结果。认证矩阵只登记具备完整证据的组合。

## 认证判定

| 等级         | 判定条件                                                 | 当前结果       |
| ------------ | -------------------------------------------------------- | -------------- |
| 来源核验     | 官方 release、源码依赖或兼容矩阵能够支撑选型             | 已完成版本选型 |
| CI 集成      | 固定 Actions workflow 在 kind 环境完成 OCM 和控制器验收  | 已通过证书轮换 |
| GPU 硬件认证 | 固定软硬件组合完成整卡、指标、故障、升级和回滚验收       | 未执行         |
| 生产认证     | CI 集成与 GPU 硬件认证均有可追溯证据，厂商安装包完成演练 | 未执行         |

## 版本变更规则

- 生产目标 patch 升级需要独立变更记录和完整回归，不沿用其他 patch 的结果。
- OCM、clusteradm、OCM API 与 Addon Framework 作为一个兼容单元评审。
- GPU Operator 升级需要重新固定其驱动、Device Plugin、GFD、MIG Manager 和 DCGM 组件矩阵。
- Volcano 与 KServe 升级需要重新验证 CRD 迁移、Webhook、升级和回滚。
- 新增 Kubernetes 1.35 或 1.36 时创建独立矩阵，保留 1.34.x 的 LTS 支持结论和退出日期。
