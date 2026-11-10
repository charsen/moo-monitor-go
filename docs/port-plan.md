# Moo Go Monitor 全业务复刻计划

## 定位与当前基线

本包是独立headless SDK，负责错误/慢SQL采集、过滤脱敏、聚合缓冲、确认同步、运维、自检、迁移与MCP。Goravel适配在边界，命令与工作台消费同一公开能力；Monitor不导入Host或scaffold，Host可单独采用。PHP参考基线仍为moo-monitor-laravel `28d424e`，原始fixture及其源SHA不因后续实现而重写。

当前module最低Go1.26.0，锁定Goravel framework/gin1.18.0、GORM1.31.2；具体driver版本以[go.mod](../go.mod)为准。15个包的本地全量race、Host真实消费、独占PostgreSQL17.10两路径及受控浏览器验证见[全盘矩阵](../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。本地fake Cloud、sqlmock和私有PG各自证明的层次不同，不能合并成所有外部服务均已验证。固定Go1.26.8 app/MCP构建仅证明[报告固定SHA](contracts/fixtures/release-reproducibility-final-go126.json)，不把后续源码或文档HEAD倒写进旧报告。

本文件描述产品范围、当前契约与验证边界；剩余动作以[执行队列](execution-queue.md)和根 TODOS 为准。历史课程记录由 skeleton 保存，不作为当前待办或授权依据。

用户确认的全业务复刻目标不变：功能、规则、输入输出、持久化与协议逐项对照，内部按Go/Goravel适配。平台差异保留缺口和替代方案，任何取消或实质收缩仍须用户明确决定；不能因本地测试完成自行删项。原PHP入口与行为线索见[源码清单](php-surface-inventory.md)及[PHP Cloud契约](../../moo-monitor-laravel/docs/cloud-api-contract.md)。PHP已知坏记录/容量/回执问题是验证线索，不作为必须照搬的缺陷。

## 当前能力与明确边界

| 编号 | 当前实现与实际消费 | 仍需保留的限制或外部验证 |
| --- | --- | --- |
| M01 配置/生命周期 | SDK显式构造，Host默认关闭的Runtime/HTTP/log/Slow及独立Queue/Schedule/Cloud owners；原业务资源所有权保留 | 不自动发现所有业务实例，不替未知第三方owner作关闭保证 |
| M02 错误/来源 | [capture](../capture/capture.go)复用Builder/Buffer，有限unwrap、显式EventScope/EventID与source升级；30可选源码展示 | 无ID不猜同一Throwable，跨scope无持久exactly-once；不识别所有业务异常类型 |
| M03 日志/5xx | [HTTP](../adapters/goravel/http.go)独立请求scope与晚身份；36 typed context/message/levels，Host原日志stack仍透传 | Host未指定Recover时正常返回5xx可观察，panic仍交框架；任意原生日志不保证全面脱敏 |
| M04 Queue/Schedule | 真实Worker最终策略旁路、原Database Driver、专用Artisan/回执/cron；Host45/46默认关闭采用 | Job无ctx不自动贯通日志身份，真实os.Exit不能截取为完成回执，外部队列后端未普遍实测 |
| M05 进程诊断 | [crashoutput](../crashoutput/manager.go)显式SetCrashOutput owner、私有raw/有界Inspect，34单ID永久receipt导入 | 仅原runtime实际输出可保存；raw未mask/无写quota，不保证OOM、SIGKILL或所有fatal；startup导入默认关闭，可显式开启 |
| M06 慢SQL | [GORM Plugin](../adapters/gorm/plugin.go)与[DB Factory](../adapters/db/factory.go)双入口，支持SQLite/Postgres/MySQL/SQLServer；Host allowlist/preload及静止Fresh代际观察已接 | 直接database/sql、未安装/未选实例与Rows后续迭代不自动覆盖；MySQL已用隔离与本机26.7双入口验通（目标版本/TLS/生产池仍待）；SQLServer实服务、生产TLS/所有池和并发Fresh未验 |
| M07 脱敏/请求 | [mask](../mask/masker.go)、SQL有限语法与递归预算共用；Host默认pwd/api_key等键、可选payload、31/40/41安全展示；工作台prefix在All前排除 | 预算不等于输入/网络防火墙；未知SQL/源码自由文本仍有盲区；不能保存任意请求对象或声称所有秘密都可识别 |
| M08 三桶/聚合 | [Buffer](../buffer/buffer.go)与SlowBuffer私有typed store，open累计/cap/持久overflow/复发、固定intent恢复 | Go独立目录/格式，不接管活动PHP桶；结果未知不能盲重试，不声称无事件ID的提交恰一次 |
| M09 文件并发 | os.Root、0700/0600、NOFOLLOW与inode复核、gate/flock、原子替换/file及目录sync | 已验合作进程模型；非设备断电实验，非同UID恶意改目录防护；跨OS/网络FS仍单列 |
| M10 scope | family/scope的记录、锁、ACK、suppression、receipt和导入归属隔离，未知状态失败关闭 | 调用方仍须给正确的项目/环境根；Cloud远端discard不是本地scope事务 |
| M11 同步/回执 | [Sync](../cloudsync/sync.go)严格逐项回执、累计快照、失败洞/隔离、有限HTTP重试；17显式/周期owner、22 All | 未确认不清除；传输与本地确认不构成跨Cloud事务；UIonce策略不提供远端exactly-once |
| M12 版本/确认 | digest+timestamp+bucket精确ACK，cursor仅安全前缀；同毫秒新内容不旧确认，open保累计锚点 | 不把count当增量求和，不把旧cursor迁成ACK；真实legacy字段/线上并发兼容不从fake推断 |
| M13 推送/维护 | Push/PushWithOptions/Tick、Stats/Preview、All与exact suppression、23确认resolved维护；Host现成CLI同一内核 | RetentionDays正数是启用维护而非天龄；All不恢复已删版本，不自动解除用户抑制 |
| M14 健康/摘要/discard | Heartbeat独立于intake deadline/result；35 Summary typed投影；20严格远端确认后local-open抑制 | 远端discard作用于token项目/type全部local unresolved，deleted=0仍可有效；Summary/discard真实权限未整体验证 |
| M15 自检/解决 | [SelfTest](../cloudsync/selftest.go)固定两类样本严格saved；39可选解决刚生成的固定runtime，失败只独立警告 | 自动resolve默认false，不操作任意hash；真实resolve权限未验，心跳成功不能代替saved |
| M16 迁移 | Inspect显式只读源；[Convert/Plan/Apply](../migration/plan.go)固定副本/完整seal、新冷scope幂等导入、保真实PHP上下文 | Inspect本身ImportSupported=false不等于独立converter不存在；不接管活动native/PHP目录、overflow无法从旧记录复原 |
| M17 MCP | [loop](../mcp/loop.go)、[七工具](../mcpcloud/toolset.go)、Host独立cmd/monitor-mcp已接；精确ID/分页/私有图片/状态回执 | 专用MONITOR_MCP_*、独立mcp ability；真实客户端/权限/分页图片和写操作仍外部待验 |
| M18 旁路安全 | 原delegate/SQL/响应保留，静态诊断、internal标记防递归、可取消/有界等待与明确结果未知 | 未承诺任意用户回调/文件syscall可硬中断，不将监控错误静默丢弃或全部当业务成功 |
| M19 工作台消费 | S-Cloud借注册Runtime，状态/预览、显式push/health/discard/prune与安全站点链接已接，无复制同步账本 | UI本身不提供远端权限；首页聚合/完整云后台仍后置，未验证的外部动作不由本地页面测试授权 |

## 实际调用链与所有权

Host MonitorServiceProvider先按配置构造本地Runtime，日志stack保原native sink；HTTP只加原中间件，Collector不自建网络任务。Slow只观察业务原实例，DB_PRELOAD_CONNECTIONS是独立业务预热，不能因监控启用而额外造池。框架原Schema/Seeder/默认连接生命周期仍可能初始化默认DB，不能宣称整个Host零连接。

已知Host shutdown顺序先停业务HTTP/queue/schedule等，再WorkBench自有reader、CloudPushOwner，最后Monitor Buffer/Source/Crash；优先级属于当前已知owners，不扩为任意第三方契约。Goravel HTTP包装对象不跨后台I/O保存；HTTP调用者取原生ctx.Context()，其中values仅浅复制，可变数据及资源生命周期仍由调用方负责。

Cloud `NewPusher` 的类型flags控制传输，不禁止本地读。健康disabled源继续支持Stats/Preview；disabled初始化失败只在明确读取该type时给initialization_failed/Unknown，不能阻断另一健康推送；enabled源失败仍fatal。Host combined reader失败时只为Status/Preview保分类型fallback，Prune仍需原combined成功并保写门禁，GET不重建/恢复。见[故障隔离](plans/cloud-source-initialization-isolation.md)。

## 持久化与网络的权威语义

- 网络前释放Buffer热锁；按family/type独立同步锁处理ACK、suppression和当前版本，旁路网络失败不令业务请求等待完整推送重试。
- `results`逐项核index/hash/status/retryable及汇总闭合；普通Sync允许已核旧成功回执，自检必须逐条严格saved。畸形回执不制造确认。
- Confirm保open累计锚点，仅exact已确认resolved可回收；cursor不是“早于它即已上传”的授权。All选当前eligible版本，仍保exact suppression。
- 默认intake最多3次，discard/heartbeat为2次连接级尝试；确定HTTP拒绝不即时重试。S-Cloud显式按钮设[DisableRetries](contracts/explicit-once-http.md)，未知POST结果不自动重发；两类、多批次和独立心跳仍可产生多次正常HTTP。
- discard远端成功但本地记账失败必须保CloudConfirmed/LocalStateUnknown，不能称回滚；本地只抑制仍匹配的local pending-open版本，保record/count/ACK/resolved。
- 迁移、诊断导入和恢复各有独立归属/intent，不能把跨文件原子性或至少一次协议写成跨Cloud事务、通用幂等或硬件耐久证明。

## Cloud 平台兼容与当前信息缺口

Go心跳发送sdk=moo-monitor-go和可选go_version/goravel_version；Cloud HeartbeatController仅对该SDK接收这些受限字段，IntegrationHealthAdvisor分Go/PHP/unknown，接入视图显示对应平台且development不套PHP最低版本。2026-11-02 21:24用户部署后的真实页面、heartbeat和runtime/slow逐项saved已记录在[Host接入证据](../../go-moo-engine-skeleton/docs/integration/cloud-connection.md)；14:59旧UI观察原样保留，不再把心跳兼容写成“本轮只记录不改Cloud”。

**每条runtime的Go/Goravel版本已贯通（代码，待部署迁移）。** SDK发送Record.context.go/goravel；Cloud Intake写入独立go_version/goravel_version，Runtime模型、读回接口、AI Markdown与详情页按平台展示；历史PHP字段不被覆盖。迁移`2026_11_03_000001_add_go_versions_to_runtimes`须在目标环境执行后才对生产记录生效。四层证据与测试见[兼容方案](plans/cloud-go-compatibility.md)。

七个MCP工具为list_open_runtimes/get_runtime/resolve_runtime/list_open_todos/get_todo/get_todo_image/update_todo_status。intake、summary/discard、mcp ability不能相互替代；当前真实健康证据只覆盖获准心跳与两类saved，不替其它字段、resolve/discard/summary/MCP权限或所有生产部署验收。

## 当前后置、平台差异与验证准则

执行队列保留外部MySQL/MariaDB/SQLServer服务、生产连接/TLS/权限及所有池、跨OS/网络FS、实际OOM/强杀、真实legacy迁移、完整MCP/Cloud剩余动作与远端tag/discovery/私有认证。原生Go不能可靠接管所有fatal/os.Exit，必须保业务影响与替代诊断说明；不是把未支持路径删出范围。

每项用源码、manifest、真实调用者与已验报告核对；固定历史报告/source fixture/失败记录不重写，用户练习不勾选。此次为文档静态校准，不重跑测试、E2E、PHP probes、真实Cloud或部署。后续验证及外部操作仍按用户当时授权执行，不能从文档状态自行推导权限。

逐课实施与验证历史统一保存于 [skeleton 历史记录](../../go-moo-engine-skeleton/docs/history/monitor-progress.md)。
