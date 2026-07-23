# 参与开发

修改代码前请先阅读 [AGENTS.md](AGENTS.md)。其中定义了产品边界、安全规则、架构、UI 要求、路线图和 Definition of Done。

## 开发流程

1. 每次变更聚焦一个领域或纵切片。
2. 对外 API 变化必须先更新或同时更新 OpenAPI。
3. 数据库结构变化必须包含迁移。
4. 状态机、幂等、权限和失败恢复必须有测试。
5. 提交前运行相关本地检查。
6. 在 Pull Request 中说明部署、安全和兼容性影响。

## 检查命令

~~~bash
make test
make lint
make openapi-lint
make build
~~~

## 提交信息

使用简洁英文提交信息，可采用以下前缀：

- feat：新增用户能力
- fix：缺陷修复
- docs：纯文档变更
- test：测试覆盖
- refactor：不改变行为的结构调整
- build：依赖、CI 或打包

## 安全约束

- 不在未纳管或生产 PVE 集群测试破坏性操作。
- 不把跳过 TLS 校验作为默认配置。
- 不向浏览器或瘦客户机暴露 PVE Token。
- 不把“异步任务已受理”描述为“操作成功”。
- 不自动删除未知或未纳管的 PVE 资源。
