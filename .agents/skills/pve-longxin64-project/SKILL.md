---
name: pve-longxin64-project
description: PVE-Longxin64 云教室平台的项目级产品、领域、架构、PVE 集成、异步任务、安全、前端体验、测试和交付规范。处理本仓库中的需求设计、领域建模、API、Go 服务、Worker、PVE Adapter、React 管理端、瘦客户机、远程桌面、测试、审查或验收任务时使用；按任务只加载相关 reference。
---

# PVE-Longxin64 项目规范

先阅读仓库根目录 `AGENTS.md` 中始终生效的约束，再根据任务选择下列最少必要参考资料。不要无条件加载全部文件。

## 参考资料路由

- 产品范围、角色权限、领域对象、状态来源、模板/课程/桌面池/瘦客户机/远程连接工作流：读取 [references/product-domain.md](references/product-domain.md)。
- 系统架构、Go API、Worker、任务状态机、幂等、并发、PVE Adapter、安全、审计、OpenAPI、SSE、可观测性：读取 [references/platform-engineering.md](references/platform-engineering.md)。
- React 页面、课堂控制台、表格、危险操作、批量任务反馈、组件库、动效、可访问性、性能：读取 [references/frontend-experience.md](references/frontend-experience.md)。
- 测试范围、路线图、MVP 验收、Definition of Done、真实环境待确认项：读取 [references/delivery-quality.md](references/delivery-quality.md)。

跨领域任务可以读取多个 reference，但只读取实际影响当前决策的文件。

## 工作方式

1. 先检查当前实现、契约和 ADR，不把规划文档误当成已经实现的事实。
2. 从相关 reference 提取必须满足的业务规则、失败语义和安全边界。
3. 设计或实现时保持模块边界：PVE 细节只进入 Adapter，领域规则不散落在 Handler、React 组件或定时器中。
4. 同时覆盖权限、幂等、锁、审计、恢复、部分失败、加载/空/无权限/断线/陈旧状态等适用维度。
5. 依据交付规范选择与风险相称的测试；没有真实 PVE 时使用 Fake PVE 或隔离实验环境。
6. 若真实部署信息未知，保留适配器与能力检测边界，并用 ADR 记录后续决策，不硬编码环境假设。

## 与其他 skill 配合

- 管理端 UI 设计或审查时，若 `emil-design-eng` 可用则同时使用；不得为了动效牺牲运维效率。
- 只有明确进行前端库选型时才使用 `pick-ui-library`，且先检查 `web/package.json`。
- 手势、拖拽、抽屉、弹簧或物理感交互确有价值时才考虑 `apple-design`。
- 动效命名、机会分析、全局审计和实现审查分别使用对应 animation skill；这些 skill 不构成添加装饰性动画的理由。
