# ADR 0001：首个工程基础与纵切片

- 状态：Accepted
- 日期：2026-07-24

## 背景

仓库处于绿地阶段，但目标是可长期演进的 PVE 云教室商业项目。首个版本必须同时验证管理端、控制平面、后台任务、PVE 适配器和部署边界，避免先堆砌无法联调的页面。

## 决策

### 管理端

- 使用 React、TypeScript 和 Vite。
- 服务端状态使用 TanStack Query。
- UI 原语使用 Base UI，通知使用 Sonner。
- 高频运维界面不引入装饰性动画；简单反馈使用 CSS。
- 管理端只访问平台 API，不直连 PVE。

### 控制平面

- 使用 Go 实现 API、Worker、PVE Adapter 和瘦客户机相关服务。
- MVP 使用模块化单体，API 与 Worker 共享领域和基础设施包。
- 所有耗时写操作创建持久化 Operation，由 Worker 异步执行。
- PVE 调用集中在 Adapter；开发和测试使用 Fake PVE。

### 数据与任务

- PostgreSQL 保存领域关系、任务、幂等键和审计。
- 第一阶段优先使用 PostgreSQL 任务租约，避免在需求未证明前引入 Redis。
- 本地开发允许显式启用内存存储；生产环境禁止静默退化到内存。

### 协议

- HTTP API 使用 REST 和 OpenAPI。
- 写操作返回任务标识；长任务进度使用 SSE，轮询作为降级。
- 所有写操作支持 Idempotency-Key。

### 远程桌面

- Desktop Broker 与 Remote Gateway 保持协议可插拔。
- 受控瘦客户机优先 FreeRDP，浏览器入口可使用 Guacamole。
- noVNC 仅作为管理员应急控制台。

## 后果

优点：

- Go 可构建 amd64 与 loong64 产物。
- 前后端职责明确，真实 PVE 凭据不会进入浏览器。
- Fake PVE 使 CI 能覆盖任务恢复和失败路径。
- 单体边界便于早期交付，同时保留后续拆分空间。

代价：

- API、Worker 和数据库状态机需要在早期投入更多工程工作。
- SSE、幂等和任务恢复提高了首个纵切片复杂度。
- 真实 PVE 能力仍必须在隔离实验集群验证。

## 后续验证

在进入真实 PVE 接入前确认：

1. 目标 PVE 版本与权限矩阵。
2. 存储的快照和链接克隆能力。
3. 龙芯瘦客户机上的 FreeRDP、浏览器和证书支持。
4. 学校认证与学生数据持久化方案。
