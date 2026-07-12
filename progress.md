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
