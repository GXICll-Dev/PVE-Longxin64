# PVE-Longxin64

PVE-Longxin64 是面向学校和计算机教室的第三方 Proxmox VE 云教室控制平台。

项目把 PVE 的虚拟机、模板、快照和任务能力组织成教室工作流：管理员维护教学模板和桌面池，教师按课程完成课前检查、批量开关机、还原和下课收机，瘦客户机连接被分配的学生虚拟桌面。

> 当前处于工程基础和首个纵切片阶段。仓库已包含真实 PVE HTTPS Adapter 基础，但尚未完成集群纳管、统一认证、审计闭环或生产远程桌面网关，也未在隔离 PVE 集群验证。不要把当前版本部署为生产系统。

## 当前纵切片

- Go 控制平面 API 与独立 Worker。
- React + TypeScript 管理端。
- 云教室总览、教室列表、座位状态和任务追踪。
- 教室批量 PRECHECK、START、SHUTDOWN、RESTORE 任务。
- Operation 与 OperationItem 状态、幂等键和分波次执行。
- PVE Adapter 边界、Fake PVE 开发实现及严格 TLS 的真实 HTTPS Adapter。
- 可重连 Operation SSE、轮询降级和座位级任务结果。
- 显式基线快照名称，RESTORE 不猜测或硬编码快照。
- PostgreSQL 数据模型与迁移基础。
- OpenAPI 契约、CI、容器和反向代理配置。

## 核心架构

~~~text
管理员/教师浏览器
        │ HTTPS
        ▼
React 管理端 ─────► Go Control Plane API ─────► PostgreSQL
                            │                       │
                            ├────► Go Worker ◄─────┘
                            │          │
                            │          ▼
                            │      PVE Adapter ───► Proxmox VE
                            │
瘦客户机 Launcher ─────────┼────► Desktop Broker
                            │          │
                            ▼          ▼
                       Remote Gateway ─► 学生虚拟桌面
~~~

设计原则和完整路线图见 [AGENTS.md](AGENTS.md)。

## 技术栈

### 管理端

- React
- TypeScript
- Vite
- TanStack Query
- TanStack Router
- Base UI
- Sonner
- zustand、clsx 和 cva 按实际页面需要逐步引入

### 控制平面

- Go
- PostgreSQL
- REST + OpenAPI
- SSE 任务增量（有界重放，轮询降级）
- 模块化单体 API 与独立 Worker

### 目标平台

- 服务端优先支持 linux/amd64。
- Go 组件保留 linux/loong64 构建目标。
- 龙芯 LoongArch64 Linux 瘦客户机是重要兼容目标。

## 仓库结构

~~~text
.
├── AGENTS.md                 项目总纲、边界和验收标准
├── api/openapi.yaml          HTTP API 契约
├── server/                   Go API、Worker、领域和 PVE Adapter
├── web/                      React TypeScript 管理端
├── deploy/                   Compose 与 Nginx 配置
├── docs/adr/                 架构决策记录
├── .github/workflows/        持续集成
└── Makefile                  常用开发命令
~~~

## 本地开发

### 前置条件

- Go 1.24 或更高版本
- Node.js 24 或更高版本
- npm
- 可选：Docker 与 Docker Compose

### 安装依赖

~~~bash
make setup
~~~

### 启动 Go API

开发默认使用内存 Store、Fake PVE 和内嵌 Worker：

~~~bash
make dev-api
~~~

API 默认监听 http://localhost:8080。

### 启动管理端

另开一个终端：

~~~bash
make dev-web
~~~

Vite 默认监听 http://localhost:5173，并把 /api 请求代理到 Go API。

### 独立 Worker

独立 API 与 Worker 必须共享 PostgreSQL。不要使用两个互不共享的内存 Store：

~~~bash
export PVE_STORE_DRIVER=postgres
export PVE_DATABASE_URL='postgres://user:password@localhost:5432/pve_classroom?sslmode=disable'
export PVE_EMBEDDED_WORKER=false

make dev-api
make dev-worker
~~~

具体变量见 server/.env.example。

## 容器开发

本机安装 Docker 后：

~~~bash
cp deploy/.env.example deploy/.env
make compose-up
~~~

完成后访问 http://localhost:8088。

停止环境：

~~~bash
make compose-down
~~~

deploy/.env.example 只适用于 Fake PVE 本地联调。共享或生产环境必须替换密码、启用入口 TLS，并完成认证、审计、凭据管理和真实 PVE 实验室验收。

已有数据库升级时必须在启动新版 API/Worker 前按编号应用新增迁移。开发 Compose 的 `docker-entrypoint-initdb.d` 只初始化空数据卷，不会自动升级既有 `postgres-data`；例如从首版升级时，先备份数据库，再执行：

~~~bash
cd deploy
docker compose exec -T postgres sh -c \
  'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  < ../server/migrations/000002_operation_snapshot_names.up.sql
~~~

## 真实 PVE Adapter（实验性）

独立 Worker（或开发模式的内嵌 Worker）可以通过环境变量选择单集群 HTTPS Adapter。生产 API 在 `PVE_EMBEDDED_WORKER=false` 时不构造 PVE Adapter，因此不需要注入 PVE Token，也不需要访问 PVE 管理网络：

~~~bash
export PVE_ADAPTER=http
export PVE_CLUSTER_ID='70000000-0000-4000-8000-000000000001'
export PVE_MANAGED_POOL='cloud-classroom-managed'
export PVE_BASE_URL='https://pve.example.edu:8006'
export PVE_TOKEN_ID='cloudclass@pve!controller'
export PVE_TOKEN_SECRET_FILE='/run/secrets/pve-token'
# 学校内部 CA 时配置；不提供跳过 TLS 校验的选项。
export PVE_CA_CERT_FILE='/run/secrets/school-internal-ca.pem'
~~~

Token Secret 也可由密钥管理系统注入 `PVE_TOKEN_SECRET`，但不能和 Secret 文件同时配置。生产 Compose 应只把 Token Secret 文件挂载给 Worker，不要放入 API 与 Worker 共用的环境文件。

真实 Adapter 会同时校验任务记录的 `cluster_id` 与 `PVE_CLUSTER_ID`，并在每次操作前通过 `PVE_MANAGED_POOL` 确认 VMID、资源类型和节点；不在受管 Pool 内、节点不一致或无法验证的资源默认拒绝。PVE Token 本身也应只在该 Pool 上授予最小权限，作为防止校验与操作之间状态变化的第二道边界。

当前 Adapter 支持 PRECHECK、START、SHUTDOWN 和基线快照回滚；RESTORE 只使用任务创建时从桌面记录固化的 `baseline_snapshot_name`，缺失时该座位明确失败。

这只是 Adapter 与失败语义基础，不代表真实 PVE 已验收。上线前仍需完成最小权限角色实测、能力检测、集群资源同步和隔离实验室故障测试。

## 常用命令

~~~bash
make test
make lint
make openapi-lint
make build
~~~

也可单独执行：

~~~bash
cd server
go test -race ./...
go vet ./...

cd ../web
npm test
npm run lint
npm run build
~~~

## API

首个契约位于 [api/openapi.yaml](api/openapi.yaml)。

当前纵切片包含：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /api/v1/health | 健康状态 |
| GET | /api/v1/readiness | 数据存储就绪状态 |
| GET | /api/v1/dashboard | 云教室总览 |
| GET | /api/v1/classrooms | 教室列表 |
| GET | /api/v1/classrooms/{classroomId} | 教室与座位详情 |
| POST | /api/v1/classrooms/{classroomId}/operations | 创建批量任务 |
| GET | /api/v1/operations | 获取最近任务 |
| GET | /api/v1/operations/{operationId} | 查询任务和单项进度 |
| GET | /api/v1/operations/{operationId}/events | 可重连的任务 SSE 增量 |

写操作必须携带 Idempotency-Key。服务端返回任务只表示请求已受理，不代表所有学生桌面已经操作成功。

## 配置原则

- 开发环境可以显式使用内存 Store 和 Fake PVE。
- 生产环境禁止使用内存 Store。
- 生产环境禁止使用 Fake PVE。
- 浏览器和瘦客户机不能持有 PVE Token。
- PVE TLS 证书不得默认跳过验证。
- PVE Token 优先通过只读 Secret 文件或密钥管理系统注入，不写入源码和普通日志。
- 所有破坏性操作最终都必须具备权限、预检、幂等、锁和审计。

## 测试策略

- 领域状态机、幂等和权限使用单元测试。
- HTTP API 使用 httptest。
- PVE 失败和 UPID 恢复使用可编程 Fake PVE 与 TLS `httptest` Adapter 测试。
- PostgreSQL 任务租约、幂等、迁移和进程重启持久化使用隔离 schema 集成测试。
- 管理端关键工作流使用 Testing Library 与 Vitest。
- 真实 PVE 克隆、快照和回滚必须在隔离实验集群验证。

CI 会执行：

- Go 格式检查、go vet、竞态测试、PostgreSQL 17 集成测试和 linux/amd64、linux/loong64 构建。
- Web lint、测试和生产构建。
- OpenAPI 规范校验。

## 安全声明

项目仍在开发中，尚不具备生产安全承诺。接入真实环境前至少需要完成：

1. 真实 PVE 隔离集群、版本能力和最小权限角色验证。
2. PostgreSQL 任务持久化与 Worker 崩溃恢复验证。
3. OIDC、LDAP 或 AD 认证及服务端 RBAC。
4. PVE Token 和设备证书的密钥管理。
5. 审计日志、防重放、限流和安全测试。
6. Remote Gateway 与短期桌面连接票据。
7. 安装、升级、备份和事故恢复手册。

## 开发约束

提交代码前请先阅读 [AGENTS.md](AGENTS.md)。特别注意：

- UI 和 Handler 不得直接调用 PVE。
- 已发布模板不可静默修改。
- 快照不是备份。
- 未纳管 PVE 资源不得自动删除。
- 批量操作必须拆成父任务和单资源子任务。
- 高频运维界面不添加无意义动效。

## 许可证

项目许可证尚未确定。在许可证明确前，不应推断代码可被任意复制、分发或用于商业再授权。
