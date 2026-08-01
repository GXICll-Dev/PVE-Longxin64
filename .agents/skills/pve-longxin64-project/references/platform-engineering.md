# 系统架构、任务、PVE、安全与 API 契约

## 目录

- 推荐系统架构与技术栈
- 异步任务、恢复与并发控制
- PVE 集成边界
- 安全与审计
- API 与可观测性约定

## 4. 推荐系统架构

MVP 采用“模块化单体 API + 独立任务 Worker + 桌面连接代理”，不要过早拆成大量微服务。

~~~text
管理员/教师浏览器
        │ HTTPS
        ▼
Web 控制台 ───────► Control Plane API ───────► PostgreSQL
                          │                         │
                          ├────► Task Worker ◄─────┘
                          │           │
                          │           ▼
                          │       PVE Adapter ─────► Proxmox VE API
                          │
瘦客户机 Launcher ───────┼────► Desktop Broker
                          │           │
                          ▼           ▼
                    Remote Gateway ──► 学生虚拟桌面
~~~

### 4.1 组件职责

- **Web 控制台**：管理员、运维和教师使用的管理界面。
- **Control Plane API**：认证、授权、领域规则、期望状态、任务创建和审计。
- **PVE Adapter**：集中封装 PVE API、版本差异、能力检测、错误归一化和 UPID 查询。
- **Task Worker**：执行克隆、开关机、快照、还原、模板发布、更新和对账。
- **Desktop Broker**：根据课程、座位、终端、用户和策略选择目标桌面。
- **Remote Gateway**：代理 RDP/VNC 等会话，不向终端暴露 PVE 管理网络。
- **Thin Client Launcher/Agent**：设备注册、心跳、桌面发现、连接拉起和断线重连。
- **PostgreSQL**：保存业务关系、期望状态、任务、幂等键、锁信息和审计。
- **可观测性模块**：结构化日志、指标、任务链路、PVE 请求耗时和告警。

### 4.2 建议技术栈

这是绿地项目的默认技术决策；更换时必须记录 ADR。

- 管理端：React + TypeScript + Vite。
- 路由、服务端状态和表格：TanStack Router、TanStack Query、TanStack Table。
- 后端与 Worker：Go，优先保证 linux/amd64 与 linux/loong64 可构建。
- API：REST + OpenAPI；任务进度使用 SSE，保留轮询降级。
- 数据库：PostgreSQL。
- 任务队列：优先使用 PostgreSQL 持久化任务和 Worker 租约；只有明确需要时再引入 Redis。
- 数据访问：显式 SQL、迁移和类型安全查询；避免隐藏关键事务行为的重 ORM。
- 部署：容器化控制平面；瘦客户机 Agent 提供独立架构构建产物。

### 4.3 建议目录

~~~text
/
├── AGENTS.md
├── README.md
├── web/                 # React 管理端
├── server/              # Go API、领域层和 PVE Adapter
├── worker/              # Go 后台任务；可与 server 共享内部包
├── agent/               # 瘦客户机 Launcher/Agent
├── api/                 # OpenAPI、生成代码和协议说明
├── deploy/              # Compose、反向代理和部署清单
├── docs/
│   ├── adr/             # 架构决策记录
│   ├── architecture/
│   └── operations/
└── tests/
    ├── integration/
    └── e2e/
~~~

共享 Go 代码可在确认模块边界后放入 internal；不要为了符合目录图而创建空目录。

## 9. 任务系统

所有批量或耗时操作必须异步执行。HTTP 请求只负责校验和创建任务。

API 接收任务后返回 202 和 operation_id，不能把“已受理”描述成“已成功”。

### 9.1 状态机

~~~text
QUEUED
  → VALIDATING
  → RUNNING
  → WAITING_PVE
  → VERIFYING
  → SUCCEEDED
~~~

异常终态：

- PARTIALLY_SUCCEEDED
- FAILED
- CANCEL_REQUESTED
- CANCELLED

每个批量 Operation 必须拆成多个 OperationItem。界面分别显示成功、失败、跳过、执行中和未知。

### 9.2 幂等与恢复

- 每个写 API 接受 Idempotency-Key。
- 重复请求返回同一任务，不得重复创建 VM 或快照。
- PVE 返回 UPID 后立即持久化。
- Worker 重启后根据 UPID 恢复跟踪，不能盲目重发命令。
- 重试前先查询实际状态；目标已达到时按成功收敛。
- 重试使用有限次数、指数退避和随机抖动。
- 认证、权限和参数错误不得无限重试。
- 取消采用协作式取消；无法安全取消的 PVE 任务继续跟踪到终态。

### 9.3 并发控制

- 同一 VM 同时最多执行一个互斥操作。
- 集群、节点、存储和桌面池分别设置并发上限。
- 批量启动和克隆必须分波次，避免开机风暴和克隆风暴。
- 教室级锁和 VM 级锁使用固定顺序，避免死锁。
- 数据库实体使用 resource_version 做乐观并发控制。
- VMID 使用平台预留和唯一约束，处理 PVE nextid 并发冲突。
- PVE 不可用时启动熔断，停止继续制造重复操作。

## 10. PVE 集成边界

- 所有 PVE 调用只能经过 PVE Adapter。
- HTTP Handler 和 UI 组件不得直接拼接 PVE API 请求。
- 优先使用正式 REST API；SSH 只能作为有明确设计和审计的例外适配器。
- 必须校验 PVE TLS 证书，支持导入学校内部 CA。
- 禁止默认关闭 TLS 校验。
- 每个集群使用独立服务账户和 API Token，并启用 privilege separation。
- Token 使用应用主密钥或密钥服务加密保存。
- PVE 请求记录关联 ID、节点、目标 VMID、UPID、耗时和脱敏结果。
- 平台只操作显式纳管的 PVE Pool、Tag、存储和网络。
- 受管 VM 同时使用平台 UUID、PVE Pool 和 Tag 标识，不能只靠名称识别。
- 已存在 VM 必须通过显式“纳管”流程导入。
- 快照、链接克隆和存储特性通过能力检测确认，不能只依据版本号猜测。

PVE 账户不得为方便直接授予 Administrator。权限应按实际功能限制到专用 Pool、存储和网络范围，并在安装时验证目标 PVE 版本的权限名称。

## 11. 安全与审计

- 所有流量使用 TLS。
- PVE Token、设备私钥、远程桌面长期密码不得进入浏览器、日志或错误响应。
- 敏感配置只保存加密值或密钥引用。
- Web 会话具备安全 Cookie、CSRF 防护、过期和撤销机制。
- 设备注册使用短期一次性码，注册后轮换为设备证书。
- 所有授权判断在服务端完成，并记录授权失败。
- 审计记录至少包含操作者、来源 IP、动作、目标范围、参数摘要、原因、关联任务和结果。
- 审计记录不能由普通管理员修改。
- QEMU Guest Agent 只开放明确需要的能力，禁止无边界任意命令执行。
- Windows、RDS/VDI、Office 和教学软件许可必须在上线前确认。
- 课堂观察、录制、剪贴板和文件重定向必须符合学校隐私制度。

高风险操作包括：

- 删除桌面、模板、快照或桌面池。
- 快照回滚和从模板重建。
- 大范围强制停止。
- 模板发布或大规模版本切换。
- 教师远程控制学生桌面。

这些操作必须：

1. 在服务端重新校验权限和目标范围。
2. 执行资源、容量、课程和冲突预检。
3. 展示受影响对象数量和数据损失。
4. 要求明确二次确认和操作原因。
5. 使用幂等键和对象锁。
6. 创建完整审计事件。

## 16. API 约定

- API 统一放在 /api/v1。
- 使用 OpenAPI 作为前后端契约来源。
- 所有 ID 使用字符串 UUID；外部 VMID 单独表达。
- 时间存储为 UTC，API 使用 RFC 3339，前端按教室时区显示。
- 批量操作返回 202、operation_id 和可查询地址。
- 所有写接口支持 Idempotency-Key。
- 列表统一支持游标或稳定分页、服务端过滤和排序。
- 错误返回稳定 error_code、用户可读信息、request_id 和可选字段级详情。
- 不把 PVE 原始错误、内部堆栈或敏感字段直接返回前端。
- SSE 事件必须可重连；事件包含 operation_id、item_id、sequence 和时间戳。
- 删除、还原、发布等接口必须支持预检或 dry-run。

## 17. 可观测性

必须具备：

- JSON 结构化日志和贯穿 API、Worker、PVE UPID 的 correlation_id。
- API、Worker、PVE 和 Gateway 的延迟与错误指标。
- 任务队列长度、运行时长、重试、失败和卡住任务指标。
- PVE 集群、节点、存储容量、证书到期和同步延迟告警。
- ThinClient 在线率、心跳延迟、Agent 版本和连接失败指标。
- 审计日志与普通应用日志分离保存。

日志不得包含 Token、Cookie、设备私钥、完整桌面密码或学生敏感数据。
