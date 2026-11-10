# Moo Monitor Go

独立 headless Go 监控 SDK，提供运行时错误与慢 SQL 采集、脱敏聚合、持久缓冲、Cloud 同步、健康检查和 MCP。Goravel/GORM 适配与核心分离，不导入具体 Host 或 scaffold。

本包保持独立 SDK 布局，区别于 Goravel 业务扩展包：调用方显式选择采集源、私有存储、网络配置及 Owner 生命周期，监控故障不能改变业务行为，未确认回执不能推进同步状态。

## 维护入口

- [全业务范围与复现计划](docs/port-plan.md)、[执行队列](docs/execution-queue.md)。
- [PHP 源码清单](docs/php-surface-inventory.md)。
- [同步、HTTP 与 GORM 契约](docs/contracts/README.md)。

学习课程、练习和逐课实施记录统一保存在 skeleton；本包只维护 SDK 使用说明、产品计划、协议、测试夹具与发布证据。

2026-11-09 仓库与 module 从 `moo-go-monitor` 更名为 `moo-monitor-go`。历史证据中的旧名指同一项目。心跳 `meta.sdk` 使用 `moo-monitor-go`，Cloud 升级期间兼容旧名。

## 许可证与远端

MIT，见 [LICENSE](LICENSE)。现有 Gitee 主仓与 GitHub 镜像保持不变，module path 为 `gitee.com/charsen/moo-monitor-go`。
