# Moo Go Monitor

Go 业务系统必需的独立监控采集 SDK，全业务复刻 PHP 监控包的能力、规则和公共契约，并提供 Goravel 接入。内部实现按 Go 适配，最终业务范围保持完整。

本机 PHP 包的实际目录和 Composer 名称是 `moo-monitor-laravel` / `charsen/moo-monitor-laravel`；Go 项目按用户指定命名为 `moo-go-monitor`。

本包负责运行时错误、慢 SQL、聚合脱敏、可恢复本地缓冲、云端同步、心跳、自检及 MCP 接入；不提供业务页面或研发工作台。业务 Host 可以独立安装它，`go-moo-scaffold` 只消费其状态与公开接口。

已建立独立Go module。M07脱敏、M02显式记录构建与M08私有本地缓冲已有真实PHP纯组件fixture及定向恢复验证；06已有显式Client的增量快照、严格回执、部分确认和持久指纹账本。07已有runtime intake/discard显式HTTP传输；HTTP/日志及显式安装SQLite ORM已有真实Host消费；队列/调度现有显式owner入口，默认Host采用仍待。未知请求表示/其它事件与驱动、真实Cloud兼容继续逐项完成。最低Go1.26.0，本机实测Go1.27.3；SDK可GOWORK=off独立运行，Host开发以忽略的workspace联调；不依赖scaffold。

- [有序课程入口](docs/README.md)

- [完整复核与复现计划](docs/port-plan.md)
- [PHP 源码入口清单](docs/php-surface-inventory.md)
- [配套监控课程](../go-moo-engine-skeleton/docs/stages/08-monitor.md)

本地缓冲仅使用调用方显式新建的私有Root：os.MkdirTemp后检查error并os.Chmod(root,0700)，再传buffer.Config.Root；scope子目录由SDK创建。不要传PHP已有runtimes或含open/resolved/deleted的桶根，不自动沿用Host目录。配置、调用链和真实恢复证据见[第05课](docs/lessons/05-runtime-buffer.md)，独立GOWORK=off使用。

[第06课](docs/lessons/06-incremental-sync.md)解释发送中同毫秒更新、确认账本与永久拒收旁车；仅显式fake Client/httptest验证，不含远端鉴权或真实推送。

[第07课](docs/lessons/07-cloud-http-client.md)提供显式HTTPClient，默认验证TLS并拒重定向，坏UTF8编码前失败。仅在调用时联网；当前验证只本机httptest假服务，不含真实Cloud、自动采集或本地discard协调器。

[第08课](docs/lessons/08-runtime-capture.md)接显式Capture与Goravel日志Handler，复用Builder/Buffer及有界EventScope；只本地落盘，不自动推送或替换Host Recover。原delegate不改写，旁路故障以安全OnError/Status观察。

[第09课](docs/lessons/09-http-capture.md)提供HTTP请求scope、有限标量快照与5xx观察，以及显式owner Recover委托；SDK不主动读取body、不注册全局Recover，采集失败不改响应。已通过真实内存Gin路由定向验证，Host本地runtime接线已见10/Host41，自动payload仍待完成。

[第11课](docs/lessons/11-slow-query-pipeline.md)提供显式ReportSlowQuery与独立SQL记录，复用同一durable store和确认协调器；固定SQL intake/discard传输仅本机替身验证。该课当时只显式输入；后续12/13已补安装SQLite实例与真实Host SQL消费，真实Cloud仍未验证。

[第12课](docs/lessons/12-gorm-slow-query.md)提供已存在SQLite Goravel实例的显式GORM插件，复用慢SQL落盘；原日志调用保留，同步采集会增加原Trace观测耗时。未自动发现连接，DB façade不在覆盖内，Host生产预热接线单独验收。

[第13课](docs/lessons/13-host-slow-query.md)补真实Host所需的当前请求标量与原生数值/已知日期类型，分别验证Food目标INSERT/UPDATE；原框架默认DB启动与显式预热动作分清，未连接真实Cloud。

[第14课](docs/lessons/14-queue-schedule.md)提供真实Worker末次策略旁路、成熟Database Driver与专用调度完成入口，保原重试/删除/失败持久化/cron恢复。该课当时的Host采用待办已由15/16接续：显式owner开关默认关闭，全局Artisan仍不替换；原失败日志仅在专用边界静态保护，不概括所有日志安全。

第15步已补显式QueueWorker单次Run/幂等关闭、Started屏障与DisableCapture，Host45已在本地四模式采用；不是所有Host默认worker都被SDK替换。见[Owner讲义](docs/lessons/15-host-queue-owner.md)。

[第16课](docs/lessons/16-host-schedule.md)在Host采用原Schedule/Artisan/cron单实例，业务owner与观察开关独立，事件默认空；原Shutdown与Run双条件排空，不拦真实os.Exit。Host46最终bootstrap证据见该课计划；本轮两功能收口后暂停，不自动开启后续功能。

[第17课](docs/lessons/17-cloud-push-owner.md)复用原Sync/SlowSync进行显式与周期增量推送，默认关闭，不因采集联网；每类失败继续、关闭先join。完整M13/all/dry/retention与真实Cloud仍待。

[第18课](docs/lessons/18-health-check.md)增加当前Cloud接受字段的心跳与显式saved级自检。每轮推送后健康结果独立；样本不落业务Buffer/ACK，旧无逐条results不算自检成功。Host48提供显式命令，本机假服务验证不代表真实Cloud兼容、resolve/MCP、summary或完整M14/M15已完成。17/18完成后暂停，不开第三块。

[第19课](docs/lessons/19-pending-statistics.md)新增client-free精确待推统计，Stats不HTTP/恢复/写ACK；构造副作用明确分开，Busy/坏状态不当零。S-Cloud一期页面仍待消费。

[第20课](docs/lessons/20-local-noise-discard.md)显式Cloud确认后抑制当前scope精确local pending-open版本，保record/count/overflow与真ACK，不推进cursor；远端仍是token项目/type全部local unresolved范围，历史ACK不证明远端当前存在。Stats/Sync共同排除exact抑制，新落盘版本重新待推。与19完成后暂停，未做真实Cloud、S-Cloud页面或第三功能。

当前新增：[第21课只读推送预览](docs/lessons/21-push-preview.md)，本地候选不验证Cloud配置。

[第22课](docs/lessons/22-explicit-all-replay.md)给同一同步链显式All模式，仅当前open/resolved非exact-suppressed版本；历史ACK不能掩盖本次失败，Remaining与Pending/最终error分开。自动Tick仍增量，--all+--dry-run只读；不做retention、恢复已删记录或真实Cloud。本轮21/22后暂停。

[第23课](docs/lessons/23-confirmed-resolved-maintenance.md)增加本地exactACK resolved维护，不是按天龄删open。当前按[持续执行队列](docs/execution-queue.md)推进，不自动push。

[第24课](docs/lessons/24-migration-inspection.md)只读体检显式旧PHP目录/游标/env，不创建Buffer或写入/导入；下一步转换导入按执行队列单独设计。

[第26课](docs/lessons/26-mcp-stdio-loop.md)提供独立MCP stdio协议循环，三版本协商、精确RPC ID、旧版text/image兼容和拥有输入的取消；仅假Toolset验证，不代表七Cloud工具或Host命令完成。25转换导入并行建设中，编号入口将在其提交后补齐。

[第27课](docs/lessons/27-cloud-mcp-tools.md)接七个typed Cloud工具、分页/私有图片与状态回执，仅假服务验证；Host隔离入口由28继续，25转换/导入并行实施。

[第28课](docs/lessons/28-host-mcp-entry.md)记录Host独立stdio进程采用，不加载业务bootstrap/dotenv，默认关闭，仅显式MONITOR_MCP_*；真实MCP仍未授权验证。

[第25课](docs/lessons/25-legacy-record-import.md)完成显式旧记录转换/封印Plan/冷scope幂等导入；源不删除、cursor不转ACK，SQL按显式敏感键重mask。此前26–28并行先交付，现课程导航按25补齐。

[第29课](docs/lessons/29-filtered-http-payload.md)新增能力化事件过滤和默认关闭的HTTP payload快照；不读原body/上传内容，使用既有mask预算，Host57明确启用，预算不等于请求防火墙。
