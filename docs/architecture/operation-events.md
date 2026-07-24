# Operation SSE 事件流

`GET /api/v1/operations/{operation_id}/events` 为任务详情提供可重连的 Server-Sent Events 增量，同时保留 `GET /api/v1/operations/{operation_id}` 轮询降级。

## 当前实现

- API 从 Repository 周期读取 Operation，因此独立 Worker 写入 PostgreSQL 后也能被观察到，不依赖同进程通知。
- 首次连接发送 `operation.snapshot`，包含任务状态、汇总进度和全部 OperationItem 状态。
- 后续发送 `operation.updated` 与 `operation.item.updated`，序列在单个 Operation 内严格递增。
- 客户端使用 `Last-Event-ID` 重连；API 进程内为每个任务保留最近 256 个事件，并最多跟踪 512 个任务。
- 历史缺口、API 重启、实例切换或内存淘汰会返回新的 `operation.snapshot`，并设置 `reset=true`。客户端此时必须用快照替换本地状态。
- 心跳是无 ID 的 SSE 注释，不改变事件序列。任务进入终态且最终事件写完后，连接正常结束。
- 每个连接只使用请求处理 goroutine 和两个可停止的 ticker；请求取消后立即退出，不创建后台订阅 goroutine。

## 限制与后续演进

事件历史当前不持久化，也不在多个 API 实例间共享，因此它保证“可恢复到最新状态”，不保证跨实例重放每一个中间状态。浏览器应在收到 `reset=true`、解析失败或 SSE 不可用时采用快照替换或轮询。

如果后续需要审计级完整事件或大规模多实例推送，应增加 PostgreSQL append-only OperationEvent 表（或等价持久事件总线），以数据库分配的序列作为 SSE ID；在此之前不能把进程内历史描述为永久事件日志。
