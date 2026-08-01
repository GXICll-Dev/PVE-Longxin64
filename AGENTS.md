# PVE-Longxin64 项目协作指南

> 本文件只保留每次任务都必须知道的项目入口和硬约束。详细产品、架构、前端与交付规范已迁移到项目级 skill：`.agents/skills/pve-longxin64-project/`。
>
> 除非用户在当前任务中明确覆盖，开发者和 AI Agent 均须遵守本文件。规划文档描述的是项目决策，不代表对应功能已经实现；开始工作前先检查当前代码、OpenAPI、迁移和 ADR。

## 1. 项目定位与技术基线

PVE-Longxin64 是面向学校、培训机构和计算机教室的第三方 Proxmox VE 云教室管理平台。产品优先解决“如何稳定地上完一节课”，而不是复制完整 PVE 管理后台或向教师暴露节点、存储和 VMID。

领域与界面以“组织/校区 → 云教室 → 座位”为中心。`Seat`、`ThinClient`、`VirtualDesktop` 和 `User` 必须分开建模；MAC 地址、座位号和 PVE VMID 均不得作为平台全局主键，内部主键使用 UUID，VMID 只在 `cluster_id` 内唯一。

默认架构和技术栈：

- 模块化单体 Control Plane API + 独立 Task Worker + Desktop Broker/Remote Gateway。
- 管理端使用 React、TypeScript、Vite、TanStack Router/Query/Table。
- 后端与 Worker 使用 Go，并优先保证 `linux/amd64` 与 `linux/loong64` 可构建。
- API 使用 REST + OpenAPI；任务进度使用 SSE，并保留轮询降级。
- PostgreSQL 保存业务数据和持久化任务；没有明确需要时不引入 Redis 或大量微服务。
- PVE 集成统一经过 PVE Adapter；瘦客户机 Agent 独立构建。
- 更换上述决策必须记录 ADR。不要为了符合目录示意图而创建空目录。

当前默认假设为局域网单组织、MVP 单 PVE 集群、PVE 8/9 通过能力检测确认兼容性；`Longxin64` 指 LoongArch64 瘦客户机兼容目标，不代表 PVE 主机架构。公网访问必须置于 VPN、零信任网关或等价安全边界之后。

## 2. 始终生效的工程约束

### 2.1 状态、领域与 PVE 边界

- PVE 是节点、存储、VM 电源和 PVE 任务结果的事实来源；平台数据库是教室、座位、课程、模板版本、期望状态、权限和审计的事实来源。
- 虚拟桌面至少区分 `desired_state`、`observed_state`、`operation_state`、`last_reconciled_at` 和 `config_hash`。
- Reconciler 只在规则明确且安全时自动修复；未知或孤儿 PVE 资源只告警，不得自动删除或自动纳管。
- React 组件、HTTP Handler 和定时器不得直接拼接或散落 PVE API 调用；领域规则进入服务层/领域层，PVE 版本差异和错误归一化只进入 Adapter。
- 优先使用正式 PVE REST API。不得为了方便使用任意 SSH 命令管理 PVE，也不得仅按版本号猜测快照、克隆或存储能力。
- 受管资源必须通过平台 UUID、专用 PVE Pool/Tag 和明确纳管范围识别，不能只依赖名称。

### 2.2 异步任务、幂等与恢复

- 所有批量或耗时操作异步执行。HTTP 只负责校验和创建任务，返回 `202`、`operation_id` 和查询地址；“已受理”不得描述成“已成功”。
- 批量任务使用 `Operation` + `OperationItem`，必须呈现成功、失败、跳过、执行中和未知，支持只重试失败项。
- 所有写 API 支持 `Idempotency-Key`。重复请求不得重复创建 VM、快照或任务。
- PVE 返回 UPID 后立即持久化。Worker 重启后先查询 UPID 和实际状态再恢复跟踪，不得盲目重发。
- 重试必须有限、带退避和抖动；认证、权限和参数错误不得无限重试。
- 同一 VM 同时最多一个互斥操作；教室级锁和 VM 级锁固定顺序；集群、节点、存储和桌面池均应有并发上限，批量启动/克隆必须分波次。
- 取消采用协作式取消；取消任务不等于撤销已经完成的单机操作，也不能把未知终态报告为成功。

### 2.3 安全、权限与审计

- 权限必须在服务端校验并支持组织、校区、教室和资源类型范围；前端隐藏按钮不能替代授权。
- 浏览器和瘦客户机永远不得持有 PVE Token。PVE Token、设备私钥、远程桌面长期密码、Cookie 和学生敏感数据不得进入日志或错误响应。
- 所有流量使用 TLS；PVE 证书必须校验并支持学校内部 CA。不得默认关闭 TLS 校验。
- 每个集群使用最小权限的独立服务账户/API Token；凭据保存加密值或密钥引用，不返回明文。
- 删除、回滚、强制停止、模板发布、大规模版本切换和教师远程控制等高风险操作，必须同时具备独立权限、资源与冲突预检、明确影响范围、二次确认、操作原因、幂等键、对象锁和不可变审计事件。
- 学生只能枚举和连接当前分配给自己的桌面；远程授权必须短期、一次性、目标受限，并能在下课、解绑或撤销后立即失效。
- 不把快照称为备份；快照回滚、模板重建和 PBS 备份恢复必须是不同动作。

### 2.4 API、数据与外部调用

- API 统一位于 `/api/v1`，OpenAPI 是前后端契约来源；ID 使用字符串 UUID，时间使用 UTC/RFC 3339。
- 错误返回稳定 `error_code`、用户可读信息、`request_id` 和可选字段详情；不得直接返回 PVE 原始错误、内部堆栈或敏感字段。
- SSE 事件可重连并携带 `operation_id`、`item_id`、`sequence` 和时间戳。
- 所有数据库变更使用迁移，并提供向前兼容和回滚说明。
- 所有外部调用设置超时、取消、有限重试和关联 ID；不得把字符串匹配 PVE 错误作为唯一业务判断。

### 2.5 前端与可访问性

- 文档和用户界面默认使用简体中文；代码标识符、API 字段和提交信息使用清晰英文。
- 云教室详情默认进入课堂控制台，不进入 VM 配置页；必须分别展示终端连接、桌面电源、模板合规和任务/维护状态。
- 新增页面时同时实现加载、空、成功、部分失败、无权限、断线和陈旧数据状态。
- 机器、任务和日志大型列表必须虚拟化；筛选、排序、搜索和分页在服务端完成，筛选条件保存到 URL。
- 开关机、还原、删除和模板发布不得乐观宣告成功；Toast 只做摘要，详细结果进入持久任务抽屉/任务中心。
- 危险操作使用准确动词、数量和作用范围，禁用时解释原因；不能只靠红色表达危险。
- 满足 WCAG 2.2 AA：完整键盘操作、可见焦点、图标按钮可读名称、状态同时使用文字/图标/颜色、支持 `prefers-reduced-motion`。
- 高频运维界面避免装饰性动画。常规动效不超过 300ms，优先只动画 `transform` 和 `opacity`，不得因实时更新重绘整个大型表格。

### 2.6 测试、可观测性与实现方式

- 关键领域状态机、权限、幂等、锁、重试、PVE 错误归一化和 Broker 授权必须有单元测试。
- 跨 PostgreSQL、Worker、PVE Adapter、SSE 的关键路径应有集成测试；核心课堂流程和越权/恢复路径应有端到端测试。
- 没有真实环境时使用可编程 Fake PVE；高风险 PVE 操作只在隔离测试集群验证，不在开发代码中硬编码生产凭据。
- 日志使用 JSON 并贯穿 API、Worker、PVE UPID 的 `correlation_id`；审计日志与普通日志分离，日志不得包含任何密钥或学生敏感数据。

## 3. 项目级 skill 路由

涉及本项目的产品行为、架构设计、实现、审查或验收时，使用 `.agents/skills/pve-longxin64-project/SKILL.md`，并按任务只加载所需 reference：

| 任务类型 | 必读 reference |
| --- | --- |
| 产品范围、角色、领域模型、状态机、模板/课程/桌面池/瘦客户机/远程连接流程 | `references/product-domain.md` |
| 系统架构、Go API、Worker、任务、幂等、并发、PVE、安全、审计、OpenAPI、SSE、可观测性 | `references/platform-engineering.md` |
| React 页面、课堂控制台、表格、危险操作、任务反馈、组件库、动效、可访问性、性能 | `references/frontend-experience.md` |
| 测试计划、路线图、MVP 验收、Definition of Done、真实环境确认 | `references/delivery-quality.md` |

跨领域功能可读取多个 reference，但不要无条件加载全部规范。

管理端 UI 设计或审查时，若相应 skill 可用则使用 `emil-design-eng`。只有明确进行前端库选型时才使用 `pick-ui-library`，且先检查 `web/package.json`。手势、拖拽、抽屉、弹簧或物理感交互确有价值时才考虑 `apple-design`；不得为了使用 skill 添加不必要动画。

## 4. 完成条件与未知环境

功能交付至少确认：业务与权限边界、API/错误契约、异步幂等与恢复、危险操作保护、完整 UI 状态、风险相称的测试、日志/指标/审计、秘密信息保护、文档同步和基础可访问性。

涉及验收或功能完成判断时，必须读取项目 skill 的 `references/delivery-quality.md`。目标 PVE、存储、网络、VM、LoongArch 硬件、远程协议、重定向需求、认证和学生数据策略尚未确认时，不阻塞骨架与 Fake PVE 开发，但必须保留 Adapter、能力检测和配置边界，并通过 ADR 固化后续选择。
