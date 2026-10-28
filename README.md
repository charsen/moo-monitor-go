# Moo Go Monitor

Go 业务系统必需的独立监控采集 SDK，全业务复刻 PHP 监控包的能力、规则和公共契约，并提供 Goravel 接入。内部实现按 Go 适配，最终业务范围保持完整。

本机 PHP 包的实际目录和 Composer 名称是 `moo-monitor-laravel` / `charsen/moo-monitor-laravel`；Go 项目按用户指定命名为 `moo-go-monitor`。

本包负责运行时错误、慢 SQL、聚合脱敏、可恢复本地缓冲、云端同步、心跳、自检及 MCP 接入；不提供业务页面或研发工作台。业务 Host 可以独立安装它，`go-moo-scaffold` 只消费其状态与公开接口。

已建立独立Go module，M07-A基础脱敏、M07-B SQL值侧有限算法与M07-C递归payload已有真实PHP假输入fixture对照；M02显式Snapshot构建/刷新/hash/source已有纯源fixture对照，未知请求表示/Host适配、真实采集和Cloud尚未实现；M08已有私有scope本地运行时缓冲、cap/overflow与固定intent恢复，尚无Cloud回执接线。最低Go1.26.0，本机实测Go1.27.3；不加入Host workspace、不依赖scaffold。

- [有序课程入口](docs/README.md)

- [完整复核与复现计划](docs/port-plan.md)
- [PHP 源码入口清单](docs/php-surface-inventory.md)
- [配套监控课程](../go-moo-engine-skeleton/docs/stages/08-monitor.md)

本地缓冲仅使用调用方显式新建的私有Root：os.MkdirTemp后检查error并os.Chmod(root,0700)，再传buffer.Config.Root；scope子目录由SDK创建。不要传PHP已有runtimes或含open/resolved/deleted的桶根，不自动沿用Host目录。配置、调用链和真实恢复证据见[第05课](docs/lessons/05-runtime-buffer.md)，独立GOWORK=off使用。
