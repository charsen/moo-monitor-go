# Moo Go Monitor

Go 业务系统必需的独立监控采集 SDK，全业务复刻 PHP 监控包的能力、规则和公共契约，并提供 Goravel 接入。内部实现按 Go 适配，最终业务范围保持完整。

本机 PHP 包的实际目录和 Composer 名称是 `moo-monitor-laravel` / `charsen/moo-monitor-laravel`；Go 项目按用户指定命名为 `moo-go-monitor`。

本包负责运行时错误、慢 SQL、聚合脱敏、可恢复本地缓冲、云端同步、心跳、自检及 MCP 接入；不提供业务页面或研发工作台。业务 Host 可以独立安装它，`go-moo-scaffold` 只消费其状态与公开接口。

已建立独立Go module。M07脱敏、M02显式记录构建与M08私有本地缓冲已有真实PHP纯组件fixture及定向恢复验证；06已有显式Client的增量快照、严格回执、部分确认和持久指纹账本。07已有runtime intake/discard显式HTTP传输；未知请求表示/Host适配、自动采集与真实Cloud兼容仍待完成。最低Go1.26.0，本机实测Go1.27.3；不加入Host workspace、不依赖scaffold。

- [有序课程入口](docs/README.md)

- [完整复核与复现计划](docs/port-plan.md)
- [PHP 源码入口清单](docs/php-surface-inventory.md)
- [配套监控课程](../go-moo-engine-skeleton/docs/stages/08-monitor.md)

本地缓冲仅使用调用方显式新建的私有Root：os.MkdirTemp后检查error并os.Chmod(root,0700)，再传buffer.Config.Root；scope子目录由SDK创建。不要传PHP已有runtimes或含open/resolved/deleted的桶根，不自动沿用Host目录。配置、调用链和真实恢复证据见[第05课](docs/lessons/05-runtime-buffer.md)，独立GOWORK=off使用。

[第06课](docs/lessons/06-incremental-sync.md)解释发送中同毫秒更新、确认账本与永久拒收旁车；仅显式fake Client/httptest验证，不含远端鉴权或真实推送。

[第07课](docs/lessons/07-cloud-http-client.md)提供显式HTTPClient，默认验证TLS并拒重定向，坏UTF8编码前失败。仅在调用时联网；当前验证只本机httptest假服务，不含真实Cloud、自动采集或本地discard协调器。

[第08课](docs/lessons/08-runtime-capture.md)接显式Capture与Goravel日志Handler，复用Builder/Buffer及有界EventScope；只本地落盘，不自动推送或替换Host Recover。原delegate不改写，旁路故障以安全OnError/Status观察。

[第09课](docs/lessons/09-http-capture.md)提供HTTP请求scope、有限标量快照与5xx观察，以及显式owner Recover委托；SDK不主动读取body、不注册全局Recover，采集失败不改响应。已通过真实内存Gin路由定向验证，Host接线和自动payload仍待完成。

[第11课](docs/lessons/11-slow-query-pipeline.md)提供显式ReportSlowQuery与独立SQL记录，复用同一durable store和确认协调器；固定SQL intake/discard传输仅本机替身验证。未自动安装数据库hook，未执行SQL或连接真实Cloud。

[第12课](docs/lessons/12-gorm-slow-query.md)提供已存在SQLite Goravel实例的显式GORM插件，复用慢SQL落盘；原日志调用保留，同步采集会增加原Trace观测耗时。未自动发现连接，DB façade不在覆盖内，Host生产预热接线单独验收。
