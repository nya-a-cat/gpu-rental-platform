# Project Progress

本日志仅记录可公开的工程结果、验证证据与回滚点，不记录本机路径、凭据或协作过程。

## 2026-07-13 - Task: 初始化独立工作区与工程约束

### What was done

- 建立全新的独立 Git 工作区，固定 Node.js 24 与 pnpm 10 工具链。
- 配置 workspace、TypeScript 严格模式、环境变量模板、忽略规则与 MIT 许可证。
- 创建按重要性和紧急度维护的产品路线图。

### Testing

- `node -e "JSON.parse(require('fs').readFileSync('package.json', 'utf8'))"`：通过。
- `git config --local --get user.name` 与 `git config --local --get user.email`：确认使用仓库级 GitHub noreply 身份。
- `git status --short`：仅包含本任务新增的预期文件。

### Notes

- `package.json`：定义根工作区命令、Node/pnpm 版本与格式化工具。
- `pnpm-workspace.yaml`：集中管理应用与共享包。
- `tsconfig.base.json`：提供严格 TypeScript 基线。
- `.nvmrc`、`.npmrc`、`.editorconfig`：统一运行时、包管理和文本格式。
- `.gitignore`、`.dockerignore`、`.env.example`：隔离本地状态并保留公开配置模板。
- `LICENSE`：添加 MIT 许可证。
- `ROADMAP.md`：建立四象限路线图。
- `progress.md`：建立仅追加的公开安全进度日志。
- 回滚点：空仓库；删除本轮列出的新增文件即可恢复到初始化前状态。

## 2026-07-13 - Task: 实现 GPU 租赁控制面与并发安全

### What was done

- 建立用户、模拟 GPU 资源和订单文档模型，完成市场聚合筛选、订单状态流转、退租与管理接口。
- 使用 Redis HttpOnly 服务端会话支持单会话退出、全部会话撤销和密码变更失效。
- 使用带持有者令牌的 `SET NX EX` 锁、Lua 比较删除与 MongoDB 部分唯一索引防止重复分配。
- 提供统一 CLI 完成模拟库存初始化与管理员创建。

### Testing

- `pnpm --filter @gpu-rental/api typecheck`：通过。
- `pnpm --filter @gpu-rental/api test`：通过，5 个测试文件共 7 项测试。
- `pnpm --filter @gpu-rental/api build`：通过。
- 真实 MongoDB/Redis E2E 已接入 GitHub Actions；本机未启动数据存储，因此不把本地 E2E 标记为通过。

### Notes

- `packages/contracts/`：新增前后端共享的角色、资源、订单、分页和错误契约。
- `apps/api/`：新增 NestJS 模块化单体、MongoDB 模型、Redis 会话与锁、业务接口、测试和维护 CLI。
- 回滚方式：对包含本任务的发布提交执行 `git revert <commit>`。

## 2026-07-13 - Task: 重写复古机械风前端与透明演示模式

### What was done

- 完成 React 双语响应式界面，覆盖市场筛选、详情预订、订单退租、登录注册与管理员后台。
- 建立 API 与浏览器演示双网关；Pages 全程标注模拟库存，不生成实体设备、支付、连接或遥测能力。
- 使用代码组件、CSS/SVG、两个原创小型机械资产和带署名的公有领域档案影像组合视觉界面。

### Testing

- `pnpm --filter @gpu-rental/web typecheck`：通过。
- `pnpm --filter @gpu-rental/web test`：通过，2 个测试文件共 3 项测试。
- `VITE_RUNTIME_MODE=demo VITE_BASE_PATH=/gpu-rental-platform/ pnpm --filter @gpu-rental/web build`：通过，生产包包含压缩后的独立视觉资产。

### Notes

- `apps/web/`：新增 React/Vite 应用、路由权限、双数据网关、页面、测试、机械组件与视觉资产。
- `docs/asset-credits.md`：记录公有领域档案图来源、许可、用途边界和原创资产说明。
- 回滚方式：对包含本任务的发布提交执行 `git revert <commit>`。

## 2026-07-13 - Task: 建立容器部署与 GitHub 自动发布

### What was done

- 配置仅暴露本机 Nginx 边缘端口的 Docker Compose，MongoDB、Redis 与 API 保持私有网络。
- 配置固定 Action 版本的质量流水线、真实数据存储 E2E、镜像构建、Dependabot 与 Pages 演示发布。
- 补齐公开 README、架构、演示边界和部署文档。

### Testing

- `docker compose config --quiet`：通过。
- `git diff --check`：通过。
- 隐私关键词扫描：未发现旧作业姓名、学号、旧路径或课程注释。
- GitHub Actions 远端执行结果将在推送后核验。

### Notes

- `.github/`：新增 CI、Pages 发布和依赖更新配置。
- `docker-compose.yml`、`infra/`：新增本地容器拓扑、镜像构建与 Nginx 反向代理。
- `README.md`、`docs/`：新增公开项目说明、架构、演示、部署与素材许可文档。
- `pnpm-lock.yaml`：锁定 workspace 依赖解析结果。
- 回滚方式：对包含本任务的发布提交执行 `git revert <commit>`。

## 2026-07-13 - Task: 修复干净 CI 环境的共享契约构建竞态

### What was done

- 让前端在 lint、类型检查、测试、开发和构建前显式生成共享契约产物，消除 workspace 并行任务对本机残留文件的依赖。

### Testing

- 首轮 GitHub Actions 日志确认失败原因是前端找不到尚未生成的 `@gpu-rental/contracts` 类型产物。
- 删除共享契约构建产物后执行根级 `pnpm lint`：通过，前端与 API 均在各自检查前重新生成契约。

### Notes

- `apps/web/package.json`：补充统一的 `contracts:build` 前置步骤。
- 回滚方式：执行 `git revert <本轮修复提交>`。

## 2026-07-13 - Task: 修复 E2E 环境的会话配置注入

### What was done

- 为 Redis 会话服务的配置依赖增加显式 NestJS 注入令牌，避免 Vitest 转译环境缺少构造参数元数据。

### Testing

- 第二轮 GitHub Actions 已通过格式、lint、类型检查与单元测试，E2E 日志将失败定位到 `SessionService` 的 `ConfigService` 参数。
- 修复后的真实 MongoDB/Redis E2E 由下一轮 Actions 复验。

### Notes

- `apps/api/src/redis/session.service.ts`：显式注入 `ConfigService`，未改变会话数据与 TTL 逻辑。
- 回滚方式：执行 `git revert <本轮修复提交>`。

## 2026-07-13 - Task: 调整真实数据存储 E2E 初始化时限

### What was done

- 将 E2E 生命周期钩子的时限调整为 60 秒，允许 GitHub runner 完成 MongoDB 连接与索引初始化；单项测试仍保持 30 秒限制。

### Testing

- 第三轮 GitHub Actions 已通过格式、lint、类型检查和单元测试，E2E 仅在默认 10 秒 `beforeAll` 时限处失败。
- 调整后的完整 E2E 由下一轮 Actions 复验。

### Notes

- `apps/api/vitest.e2e.config.ts`：只调整集成环境初始化时限，不改变测试断言或业务实现。
- 回滚方式：执行 `git revert <本轮修复提交>`。

## 2026-07-13 - Task: 解耦静态演示发布与后端质量门禁

### What was done

- 让 GitHub Pages 演示构建与后端质量 job 并行运行，后端 E2E 失败仍保持红色，但不再阻止纯静态演示资产上传。

### Testing

- GitHub Pages 已配置为 Actions 来源；下一次 main 推送将独立执行 Pages job。
- 后端 quality job 未被删除、跳过或降级，仍继续执行真实 MongoDB/Redis E2E。

### Notes

- `.github/workflows/pipeline.yml`：移除 Pages 对 quality job 的依赖关系。
- 回滚方式：执行 `git revert <本轮修复提交>`。

## 2026-07-13 - Task: 移除隔离 E2E 容器的冗余 Redis 预清理

### What was done

- 删除 E2E 启动阶段对全新 Redis service container 的串行 `KEYS` 扫描；Redis 仍由登录、撤销与并发锁用例真实访问。

### Testing

- MongoDB 服务日志确认连接、认证、集合创建和全部索引构建均在 E2E 启动后立即完成，排除数据库初始化慢的问题。
- 精确日志显示超时发生于首个测试执行前；下一轮 Actions 将验证移除冗余 Redis 预清理后的完整 5 项 E2E。

### Notes

- `apps/api/test/api.e2e-spec.ts`：保留测试数据库名称保护与 MongoDB 数据清理，仅移除隔离 Redis 的无效预扫描。
- 回滚方式：执行 `git revert <本轮修复提交>`。

## 2026-07-13 - Task: 完成截图驱动的复古机械产品界面重构

### What was done

- 将市场首页重构为不对称控制台与机架式资源清单，并把运行模式提示收进顶部状态模块。
- 统一重做登录、资源详情、订单与调度后台，加入设备观察窗、常用租期、非对称调度读数、空状态和移动端控制栏。
- 修复管理员快捷身份重定向竞态与跨路由滚动位置继承，确保完整业务路径在桌面和移动端连续可用。

### Testing

- `apps/web/node_modules/.bin/tsc -p apps/web/tsconfig.json --noEmit`：通过。
- `VITE_RUNTIME_MODE=demo VITE_BASE_PATH=/gpu-rental-platform/ vite build`：通过，生成 Pages 生产包。
- `vitest run apps/web/src/test/demo-gateway.test.ts --reporter=verbose`：通过，1 个文件共 2 项数据网关测试；角色路由测试在当前 WSL 挂载盘环境停在收集阶段，未虚报为通过。
- Playwright 以 `1440×1000` 和 `390×844` 视口验收首页、登录、资源详情、订单和调度后台；完成普通用户登录、24 小时预订、一键退租、管理员资源下架后恢复上架及管理员直达后台流程。
- Playwright 最终页面控制台为 0 error、0 warning；移动端首页、资源详情和调度后台均无横向溢出，跨路由后 `window.scrollY` 为 0。

### Notes

- `apps/web/src/components/layout.tsx`：压缩运行模式状态、重构导航并在路由切换时恢复页面顶部。
- `apps/web/src/pages/MarketPage.tsx`：改造不对称市场首屏、档案观察窗、状态读数与机架资源清单。
- `apps/web/src/pages/AuthPage.tsx`：精简为单张公有领域档案图并修复管理员快捷身份重定向。
- `apps/web/src/pages/ResourcePage.tsx`：将生成图降为组合式设备观察窗并增加常用租期按键。
- `apps/web/src/pages/AdminPage.tsx`：增加不对称调度板、运行读数与资源和订单空状态。
- `apps/web/src/styles.css`：建立复古机械材质层级、桌面与移动端响应式布局和控制器状态样式。
- `docs/demo-mode.md`：补充公开演示中可完整操作的用户与管理员流程。
- `docs/deployment.md`：同步 Pages 独立发布与后端质量检查并行策略。
- `ROADMAP.md`：记录 Pages 已发布与全页面截图驱动验收完成。
- `progress.md`：追加本轮实现、测试证据、文件清单与回滚点。
- 回滚方式：执行 `git revert <本轮前端提交>`。

## 2026-07-13 - Task: 定位 CI E2E 初始化阻塞阶段

### What was done

- 为 E2E 启动的模块编译、Nest 应用初始化和 MongoDB 清理增加带耗时的阶段标记，用于定位精确 60 秒超时发生点。

### Testing

- GitHub Actions 运行 `29212990922` 已确认格式、lint、类型检查、单元测试和 Pages 发布通过，API E2E 仍在 `beforeAll` 精确 60 秒超时。
- 阶段标记由下一轮 Actions 运行提供远端诊断证据，不把诊断改动表述为根因已修复。

### Notes

- `apps/api/test/api.e2e-spec.ts`：增加不含凭据和业务数据的初始化耗时标记。
- `progress.md`：追加本轮诊断范围、验证证据和回滚点。
- 回滚方式：执行 `git revert <本轮诊断提交>`。

## 2026-07-13 - Task: 修复 NestJS E2E 源码转译装饰器元数据阻塞

### What was done

- 将 API E2E 改为先构建 NestJS，再加载生产 `dist` 模块，避免 Vitest 源码转译丢失 decorator metadata 后卡在测试模块编译。
- 保留真实 MongoDB、Redis、并发预订、会话撤销、退租和 RBAC 断言，不跳过或替换后端集成测试。

### Testing

- 已编译 `dist/app.module.js` 在本机 MongoDB 8.0 与 Redis 8 环境中完成 Nest TestingModule 编译，源码经 Vitest 转译时可稳定复现 60 秒阻塞。
- 修复后的完整 5 项 E2E 先在本机同版本容器复验，再由下一轮 GitHub Actions 进行独立验证。

### Notes

- `apps/api/package.json`：让 E2E 生命周期先生成生产构建产物。
- `apps/api/test/api.e2e-spec.ts`：改为导入编译后的 AppModule 和业务服务令牌，并移除临时阶段日志。
- `docs/deployment.md`：记录 CI 使用生产编译产物执行真实后端 E2E。
- `progress.md`：追加根因、修复、验证计划和回滚点。
- 回滚方式：执行 `git revert <本轮修复提交>`。

## 2026-07-13 - Task: 稳定并发 E2E 的真实监听端口

### What was done

- 让 NestJS 测试应用在本机随机端口统一监听，20 路并发请求复用同一 HTTP 服务，消除 Supertest 临时监听的端口竞态。
- 测试数据通过同一测试 MongoDB 连接写入，业务结果仍全部通过真实 HTTP 接口断言。

### Testing

- 本机 MongoDB 8.0 与 Redis 8 环境执行 API E2E：5 项全部通过。
- 并发预订验证 20 个请求中仅 1 个成功、19 个冲突，并确认数据库仅存在 1 个活跃订单。

### Notes

- `apps/api/test/api.e2e-spec.ts`：增加随机本机监听端口并使用测试数据库连接写入夹具。
- `progress.md`：追加并发 E2E 稳定性修复及验证证据。
- 回滚方式：执行 `git revert <本轮修复提交>`。

## 2026-07-13 - Task: 将机械控制台接入真实交互

### What was done

- 将首页静态机械旋钮替换为控制总线、可租锁定、筛选归零和状态、价格、排序六个真实 HTML 控件。
- 让机械控件与标准筛选器、仪表读数和库存结果共享同一业务状态，并修正价格滑杆无法精确落到最高价格的问题。
- 增加原创窄幅校准铭牌、旋钮转位、按键压下、状态灯呼吸和低频扫描动效；统一使用 Barlow Condensed、IBM Plex Mono 与 Noto Sans SC 字体体系。

### Testing

- `apps/web/node_modules/.bin/tsc -p apps/web/tsconfig.json --noEmit`：通过。
- `VITE_RUNTIME_MODE=demo VITE_BASE_PATH=/gpu-rental-platform/ vite build`：通过，校准铭牌构建产物约 26 KB。
- `vitest run src/test/routing.test.tsx --reporter=verbose`：通过，2 项测试覆盖角色路由和机械控制台状态同步。
- Playwright 在 `1440×1000` 与 `390×844` 视口逐项验证六个机械控件；可租筛选、租用中筛选、价格上限、低价排序、总线禁用和归零均产生对应业务状态变化。
- Playwright 确认移动端 6 个控件全部可见且无横向溢出，旋钮、灯光和扫描动效处于生效状态，浏览器控制台为 0 error、0 warning。

### Notes

- `apps/web/index.html`：接入 Google Fonts 字体资源与预连接。
- `apps/web/src/assets/generated/inspection-calibration-plate.webp`：增加原创小型校准铭牌资产。
- `apps/web/src/components/mechanical.tsx`：将装饰旋钮升级为可访问的真实按钮组件。
- `apps/web/src/pages/MarketPage.tsx`：增加六个控制台动作并同步筛选、读数和库存状态。
- `apps/web/src/styles.css`：增加控件状态、动效、小资产合成和统一字体规则。
- `apps/web/src/test/routing.test.tsx`：增加机械控制台点击与筛选同步测试。
- `apps/web/src/test/setup.ts`：补齐 JSDOM 的滚动接口以保持测试输出干净。
- `docs/demo-mode.md`：说明公开演示中机械控制台的真实行为。
- `docs/asset-credits.md`：补充原创铭牌与 Google Fonts 授权来源。
- `ROADMAP.md`：记录机械控件交互和视觉统一已完成。
- `progress.md`：追加本轮实现、验证证据和回滚点。
- 回滚方式：执行 `git revert <本轮交互提交>`。

## 2026-07-13 - Task: 增强市场页银色工业沉浸感并本地化档案素材

### What was done

- 将 NASA/NARA 公有领域控制室照片下载为本地构建资产，并在市场首屏与身份页复用，消除核心氛围图对远程站点的运行时依赖。
- 以原创银色机架墙和服务管线小资产构建环境层，增加动态状态桥、金属纵深、扫描光、离线降亮与机架式库存背景。
- 保持机械总线、锁定、归零和三个旋钮为真实 HTML 控件；状态桥同步显示控制总线、资源状态、价格、排序和库存数量。
- 调整手机首屏构图，让档案照片和机械控制台在首屏连续出现，并保持全部六个控件可见、可操作。

### Testing

- `apps/web/node_modules/.bin/tsc -p apps/web/tsconfig.json --noEmit`：通过。
- 在 `apps/web` 执行 `node_modules/.bin/vitest run --reporter=verbose`：2 个文件共 4 项测试全部通过。
- 在 `apps/web` 执行 `VITE_RUNTIME_MODE=demo VITE_BASE_PATH=/gpu-rental-platform/ node_modules/.bin/vite build`：通过；本地 NASA 照片、银色机架墙和服务管线均进入 Pages 生产包。
- Playwright 在 `1440×1000` 与 `390×844` 视口复验：桌面和移动端均无横向溢出，6 个机械控件全部可见；断开总线后 5 个从属控件禁用，重新接通并旋转资源状态后标准筛选值与库存从 006 同步变为 005。
- Playwright 确认市场页和身份页均加载本地 NASA 图片，浏览器控制台为 0 error、0 warning。

### Notes

- `apps/web/src/pages/MarketPage.tsx`：接入本地档案图、控制台通断状态和同步状态桥。
- `apps/web/src/pages/AuthPage.tsx`：将身份页档案图切换为同一本地资产。
- `apps/web/src/styles.css`：增加银色工业环境层、纵深光影、状态动画、离线反馈和移动端沉浸构图。
- `apps/web/src/assets/archive/nasa-control-room-1976.jpg`：新增 Wikimedia Commons 公有领域档案图的本地构建副本。
- `apps/web/src/assets/generated/silver-rack-wall.webp`：新增原创银色机架墙环境资产。
- `apps/web/src/assets/generated/silver-service-duct.webp`：新增原创服务管线环境资产。
- `docs/asset-credits.md`：记录档案图本地路径、来源授权与两项原创环境资产。
- `docs/demo-mode.md`：说明环境层与动态状态桥的演示边界和真实交互关系。
- `ROADMAP.md`：记录沉浸式市场环境层已完成。
- `progress.md`：追加本轮实现、验证证据、文件清单与回滚点。
- 回滚方式：执行 `git revert <本轮沉浸式视觉提交>`。

## 2026-07-13 - Task: 修复市场首屏高度回归并压缩移动端控制台

### What was done

- 将档案照片改为脱离 Grid 自动行尺寸计算的绝对定位环境层，使桌面 Hero 恢复既定的 680–820 像素高度范围。
- 压缩窄屏标题、照片、仪表台和状态桥，将三个只读仪表改为横向紧凑排列，同时保留六个真实机械控件。
- 纠正上一轮日志中的回滚占位符；上一轮沉浸式视觉提交的真实回滚命令为 `git revert 3149a1a`。

### Testing

- 在 `apps/web` 执行 `VITE_RUNTIME_MODE=demo VITE_BASE_PATH=/gpu-rental-platform/ node_modules/.bin/vite build`：通过，59 个模块完成生产构建。
- Playwright 验证 `1366×768`、`1440×900`、`1920×1080`：Hero 高度分别约为 680、750、820 像素，第一张资源卡分别位于 1.30、1.19、1.06 屏，无横向溢出。
- Playwright 验证 `390×844`：Hero 高度约 858 像素，第一张资源卡位于 1.61 屏，6 个机械控件全部可见且页面宽度保持 390 像素。
- Playwright 在手机视口验证控制总线断开后 5 个从属控件禁用；重新接通并旋转资源状态后筛选值变为 `available`，资源数从 6 变为 5。

### Notes

- `apps/web/src/styles.css`：解除档案图固有比例对桌面 Grid 高度的影响，并增加手机端紧凑仪表和状态桥布局。
- `docs/demo-mode.md`：记录桌面 Hero 高度边界与移动端控件保留原则。
- `ROADMAP.md`：在重要且紧急分区记录首屏高度回归已修复。
- `progress.md`：追加本轮实现、验证证据、文件清单和上一轮真实回滚点。
- 回滚方式：执行 `git revert "$(git log --format=%H --grep='^fix: compact market hero$' -1)"`。

## 2026-07-13 - Task: 修复管理员新建演示资源的编号截断

### What was done

- 将市场卡片的资源记录格式化规则集中到现有格式化模块，完整显示 `demo-gpu-100` 对应的 `GPU-100`。
- 保留真实后端 MongoDB ObjectId 只展示末六位的紧凑规则，避免长标识撑开资源卡片。
- 增加精确回归测试，覆盖初始演示编号、新建演示编号和真实后端长标识。

### Testing

- 在 `apps/web` 执行 `node_modules/.bin/vitest run src/test/format.test.ts --reporter=verbose`：1 个文件共 2 项测试全部通过。
- `apps/web/node_modules/.bin/tsc -p apps/web/tsconfig.json --noEmit`：通过。
- Playwright 使用管理员登记并上架 `QA TEST UNIT`，市场筛选后确认新建记录显示为 `GPU-100`，不再显示为 `PU-100`。

### Notes

- `apps/web/src/format.ts`：增加演示编号与真实 ObjectId 的资源记录格式化规则。
- `apps/web/src/pages/MarketPage.tsx`：资源卡片改用统一的记录格式化函数。
- `apps/web/src/test/format.test.ts`：增加三类资源标识的回归断言。
- `docs/demo-mode.md`：说明演示编号和真实后端标识的展示边界。
- `ROADMAP.md`：在重要且紧急分区记录编号截断已修复。
- `progress.md`：追加本轮实现、验证证据、文件清单与回滚点。
- 回滚方式：执行 `git revert "$(git log --format=%H --grep='^fix: preserve demo resource records$' -1)"`。

## 2026-07-13 - Task: 修复刷新后的页面语言元数据

### What was done

- 将文档语言同步到当前界面语言状态，使英文或中文偏好在首次加载和刷新后都更新可访问性语言元数据。
- 保留原有浏览器本地语言偏好，不改变语言切换入口和可见文案逻辑。

### Testing

- Playwright 复现修复前英文界面刷新后文案仍为英文、但 `<html lang>` 回退为 `zh-CN`。
- 在 `apps/web` 使用 Node.js 24 与单工作进程执行 `vitest run src/test/routing.test.tsx --reporter=verbose --maxWorkers=1 --no-file-parallelism`：1 个文件共 3 项测试全部通过，新增用例覆盖预存英文偏好后的首次渲染。
- `tsc -p apps/web/tsconfig.json --noEmit` 与本轮改动文件的 Prettier 检查：通过。
- 重启 Vite 清除 Windows 挂载盘缓存后，Playwright 验证切换英文并刷新前后均为英文文案、`lang="en"` 和本地偏好 `en`。

### Notes

- `apps/web/src/app-context.tsx`：按当前语言状态统一同步文档语言元数据。
- `apps/web/src/test/routing.test.tsx`：增加已保存语言在刷新场景下的回归断言。
- `docs/demo-mode.md`：记录双语偏好与文档语言元数据的持久化行为。
- `ROADMAP.md`：在重要且紧急分区记录语言元数据缺陷已修复。
- `progress.md`：追加本轮实现、验证计划、文件清单与回滚点。
- 回滚方式：执行 `git revert "$(git log --format=%H --grep='^fix: restore document language$' -1)"`。

## 2026-07-13 - Task: 修复 Firefox 用户名格式校验

### What was done

- 将用户名输入规则中的连字符显式转义，使现代浏览器按 HTML `pattern` 的 Unicode Sets 规则正确编译并执行校验。
- 保持用户名允许字母、数字、下划线和连字符的既有业务边界不变。

### Testing

- Playwright 在 Firefox 注册页复现修复前的非法 `pattern` 控制台错误。
- 在 `apps/web` 使用 Node.js 24 与单工作进程执行 `vitest run src/test/routing.test.tsx --reporter=verbose --maxWorkers=1 --no-file-parallelism`：1 个文件共 4 项测试全部通过；新增用例覆盖非法空格用户名被拒绝、合法下划线和连字符用户名通过。
- `tsc -p apps/web/tsconfig.json --noEmit` 与本轮改动文件的 Prettier 检查：通过。
- 重启 Vite 后使用 Firefox 复验：输入规则为 `[A-Za-z0-9_\\-]+`，非法值触发 `patternMismatch`，合法值通过，控制台为 0 error、0 warning。

### Notes

- `apps/web/src/pages/AuthPage.tsx`：修正用户名输入的浏览器原生校验表达式。
- `apps/web/src/test/routing.test.tsx`：增加用户名格式的有效与无效输入断言。
- `docs/demo-mode.md`：记录演示身份的用户名字符规则。
- `ROADMAP.md`：在重要且紧急分区记录 Firefox 格式校验缺陷已修复。
- `progress.md`：追加本轮实现、验证计划、文件清单与回滚点。
- 回滚方式：执行 `git revert "$(git log --format=%H --grep='^fix: validate usernames in firefox$' -1)"`。

## 2026-07-13 - Task: 完成公开演示全流程验收

### What was done

- 以全新浏览器状态完成双语、注册、会话撤销、普通用户预订与退租、管理员资源上下架、订单取消、角色访问控制和演示归零闭环。
- 在桌面与手机视口复验机械控制台、固定导航、资源详情、订单和调度后台，并检查生产构建与全仓静态质量门槛。

### Testing

- Web 测试按文件执行：`routing.test.tsx` 4 项、`demo-gateway.test.ts` 2 项、`format.test.ts` 2 项，共 3 个文件 8 项全部通过；全量串行命令在本机 `/mnt/c` 的 JSDOM 初始化阶段达到 300 秒硬超时，已按文件补齐同配置验证。
- Contracts、API 与 Web 的 TypeScript 检查通过，API 生产编译通过，全仓 Prettier 检查通过。
- Pages 演示配置生产构建通过：59 个模块完成转换，生成本地档案照片、原创工业资产、CSS 与 JavaScript 构建产物。
- Playwright 桌面闭环：注册普通用户，创建 24 小时 H100 订单并确认总价 ¥789.60，刷新后订单仍生效，一键退租释放资源；普通用户访问后台被拒绝。
- Playwright 管理闭环：管理员登记下架资源、切换为已上架、市场卡片显示完整 `GPU-102` 记录；取消种子订单后 A100 恢复可预订，演示归零后恢复 6 条库存并退出身份。
- Playwright `390×844`：首页、登录、资源详情、订单和调度后台均无横向溢出，底部导航可点击；6 个机械控件可见，断开总线后 5 个从属控件禁用；最终浏览器控制台为 0 error、0 warning。

### Notes

- `progress.md`：追加最终自动化、桌面业务闭环、移动端和浏览器控制台验收证据。
- 本轮未新增产品代码；Playwright YAML、控制台日志和截图均为临时验证产物，已从仓库工作树清理。
- 回滚方式：执行 `git revert "$(git log --format=%H --grep='^docs: record final product acceptance$' -1)"`。

## 2026-07-13 - Task: 完成交互式机械调度台新版

### What was done

- 将市场首屏重排为紧凑的实时分配台，在首屏加入可直接进入资源详情的库存机架，并让匹配数量、价格、状态灯和控制偏移仪表共用真实筛选结果。
- 保留六个现有机械控件的业务状态源，补充旋钮双向键盘操作、可访问仪表语义、断电反馈和移动端 44 像素触控下限，没有增加虚构温度、利用率或主机遥测。
- 将新版浏览器演示状态切换到独立的 `v2` 命名空间，避免与冻结旧版的订单、身份和库存互相污染。

### Testing

- 在 `apps/web` 使用 Node.js 24 串行执行 `vitest run src/test/routing.test.tsx src/test/demo-gateway.test.ts --reporter=verbose --maxWorkers=1 --no-file-parallelism`：2 个文件共 6 项测试全部通过。
- `tsc -p apps/web/tsconfig.json --noEmit` 通过；Next Pages 配置生产构建通过，59 个模块完成转换。
- Playwright `1440×900` 验证 Hero 高度为 580 像素、首张完整资源卡顶部为 890 像素、三条快速库存链接可用；可租锁定后匹配数量从 6 变为 5、控制偏移从 `0/6` 变为 `1/6`。
- Playwright 验证控制总线断开后五个从属控件全部禁用、信号条归零；快速库存链接可进入对应资源详情。
- Playwright `390×844` 验证页面宽度保持 390 像素、移动导航回到文档流、控制台不被遮挡、六个控件最小高度为 44 像素；浏览器控制台为 0 error、0 warning。

### Notes

- `apps/web/index.html`：接入随 Pages base path 解析的本地站点图标。
- `apps/web/public/favicon.svg`：增加与 Kiloworks 工业铭牌一致的矢量图标。
- `apps/web/src/components/mechanical.tsx`：让仪表暴露 meter 语义，并让旋钮支持双向方向键。
- `apps/web/src/data/demo-gateway.ts`：将新版演示状态隔离到 `gpu-rental-demo-state-v2`。
- `apps/web/src/pages/MarketPage.tsx`：增加实时库存机架、数据线路反馈和紧凑控制台布局状态。
- `apps/web/src/styles.css`：实现新版桌面与移动端控制台、库存机架、状态反馈和响应式布局。
- `apps/web/src/test/demo-gateway.test.ts`：同步新版演示状态命名空间。
- `apps/web/src/test/routing.test.tsx`：覆盖实时库存、控制偏移和旋钮反向操作。
- `docs/demo-mode.md`：记录新版真实交互边界、状态隔离和响应式行为。
- `ROADMAP.md`：在重要且紧急分区记录交互新版与双路径预览完成。
- `progress.md`：追加本轮实现、验证证据、文件清单与回滚点。
- 回滚方式：执行 `git revert "$(git log --format=%H --grep='^feat: add interactive console preview$' -1)"`。

## 2026-07-13 - Task: 建立 Classic 与 Next 双路径 Pages

### What was done

- 将 Pages 发布改为分别从冻结标签 `ui-v1.0.0` 和开发分支 `ui/interactive-console-v2` 构建，再合并为同一个版本化站点。
- 根地址增加版本选择页，Classic 与 Next 使用独立子路径，后续任一发布分支触发时都会重新组装完整站点。

### Testing

- Ruby YAML 解析、改动文件 Prettier 检查、SVG XML 解析与 `git diff --check` 全部通过。
- 本地从冻结标签和当前新版分别完成 59 模块生产构建，并按 Actions 同等目录结构组装 `pages-root`；入口页、Classic、Next 及两套 base path 检查全部通过。
- Playwright 从版本选择页分别进入 Classic 与 Next，确认旧版序列标识为 `GPU INVENTORY 2026`、新版为 `LIVE ALLOCATION DESK / V2`；两边归零后同时存在互不覆盖的 `v1` 与 `v2` 状态键。

### Notes

- `.github/workflows/pipeline.yml`：增加显式 tag/branch 双 checkout、双构建、目录组装与 Pages 路径验证。
- `deploy/pages-index.html`：增加 Classic 与 Next 的根版本选择界面。
- `docs/deployment.md`：记录双版本构建来源、访问路径和状态隔离规则。
- `progress.md`：追加本轮实现、验证证据、文件清单与回滚点。
- 回滚方式：执行 `git revert "$(git log --format=%H --grep='^ci: publish classic and next previews$' -1)"`。

## 2026-07-13 - Task: 遵循 Pages 环境保护发布双版本

### What was done

- 保留 GitHub Pages 现有分支保护，不放宽部署权限；将正式 Pages 发布限制为受保护的 `main` 分支。
- 保留新版分支的完整质量检查，并增加从 `main` 手动重新组装 Classic 与 Next 的入口，使新版可在不合并界面代码的情况下更新公开预览。

### Testing

- GitHub Actions 运行 `29252716657` 的 Pages 任务在执行任何步骤前被环境规则拒绝，注解明确为 `Branch "ui/interactive-console-v2" is not allowed to deploy to github-pages due to environment protection rules.`。
- Ruby YAML 解析、改动文件 Prettier 检查与 `git diff --check` 通过；Pages 条件只允许 `main` push 或以 `main` 为 ref 的手动派发。

### Notes

- `.github/workflows/pipeline.yml`：增加手动派发入口，并将 Pages 发布约束到受保护的 `main` 分支。
- `docs/deployment.md`：记录双版本的规范发布入口与新版更新命令。
- `progress.md`：追加环境保护问题、处理方式、验证证据与回滚点。
- 回滚方式：执行 `git revert "$(git log --format=%H --grep='^fix: honor pages deployment protection$' -1)"`。

## 2026-07-13 - Task: 修复 Classic 公网页面图标请求

### What was done

- 在双版本站点组装阶段为 Classic 产物加入本地 SVG 图标，消除浏览器对 GitHub Pages 域名根目录 `favicon.ico` 的无效请求。
- 保持 `ui-v1.0.0` 标签和 Classic 产品源码不变，修复仅作用于临时发布产物。

### Testing

- Playwright 打开已发布的 Classic 页面，业务界面、1440 像素视口和旧版标识均正常，但控制台精确记录 1 条 `https://nya-a-cat.github.io/favicon.ico` 的 404，确认问题只来自缺失图标声明。
- 对冻结标签的 `index.html` 执行与 Actions 相同的 `sed` 变换并检查 `rel="icon"`，结果通过；Ruby YAML 解析、改动文件 Prettier 检查与 `git diff --check` 通过。

### Notes

- `.github/workflows/pipeline.yml`：在 Classic 产物中复制本地图标、注入图标声明并加入构建校验。
- `docs/deployment.md`：说明发布层图标补丁不会修改冻结标签。
- `progress.md`：追加问题复现、修复边界、验证证据与回滚点。
- 回滚方式：执行 `git revert "$(git log --format=%H --grep='^fix: provide classic preview favicon$' -1)"`。
