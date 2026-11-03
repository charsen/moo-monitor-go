# Moo Go Monitor 全业务复刻计划

## 定位与当前基线

本包是独立headless SDK，负责错误/慢SQL采集、过滤脱敏、聚合缓冲、确认同步、运维、自检、迁移与MCP。Goravel适配在边界，命令与工作台消费同一公开能力；Monitor不导入Host或scaffold，Host可单独采用。PHP参考基线仍为moo-monitor-laravel `28d424e`，原始fixture及其源SHA不因后续实现而重写。

当前module最低Go1.26.0，锁定Goravel framework/gin1.18.0、GORM1.31.2；具体driver版本以[go.mod](../go.mod)为准。15个包的本地全量race、Host真实消费、独占PostgreSQL17.10两路径及受控浏览器验证见[全盘矩阵](../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。本地fake Cloud、sqlmock和私有PG各自证明的层次不同，不能合并成所有外部服务均已验证。固定Go1.26.8 app/MCP构建仅证明[报告固定SHA](contracts/fixtures/release-reproducibility-final-go126.json)，不把后续源码或文档HEAD倒写进旧报告。

本文件的下表描述当前契约；末尾逐课节点保留当时设计、失败和分批验证。课程中的“未接”“暂停”不再作为当前待办或授权门禁。当前剩余动作以[执行队列](execution-queue.md)和根TODOS为准，用户自主练习不自动完成。

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

Go心跳发送sdk=moo-go-monitor和可选go_version/goravel_version；Cloud HeartbeatController仅对该SDK接收这些受限字段，IntegrationHealthAdvisor分Go/PHP/unknown，接入视图显示对应平台且development不套PHP最低版本。2026-11-02 21:24用户部署后的真实页面、heartbeat和runtime/slow逐项saved已记录在[Host接入证据](../../go-moo-engine-skeleton/docs/integration/cloud-connection.md)；14:59旧UI观察原样保留，不再把心跳兼容写成“本轮只记录不改Cloud”。

**每条runtime的Go/Goravel版本已贯通（代码，待部署迁移）。** SDK发送Record.context.go/goravel；Cloud Intake写入独立go_version/goravel_version，Runtime模型、读回接口、AI Markdown与详情页按平台展示；历史PHP字段不被覆盖。迁移`2026_11_03_000001_add_go_versions_to_runtimes`须在目标环境执行后才对生产记录生效。四层证据与测试见[兼容方案](plans/cloud-go-compatibility.md)。

七个MCP工具为list_open_runtimes/get_runtime/resolve_runtime/list_open_todos/get_todo/get_todo_image/update_todo_status。intake、summary/discard、mcp ability不能相互替代；当前真实健康证据只覆盖获准心跳与两类saved，不替其它字段、resolve/discard/summary/MCP权限或所有生产部署验收。

## 当前后置、平台差异与验证准则

执行队列保留外部MySQL/MariaDB/SQLServer服务、生产连接/TLS/权限及所有池、跨OS/网络FS、实际OOM/强杀、真实legacy迁移、完整MCP/Cloud剩余动作与远端tag/discovery/私有认证。原生Go不能可靠接管所有fatal/os.Exit，必须保业务影响与替代诊断说明；不是把未支持路径删出范围。

每项用源码、manifest、真实调用者与已验报告核对；固定历史报告/source fixture/失败记录不重写，用户练习不勾选。此次为文档静态校准，不重跑测试、E2E、PHP probes、真实Cloud或部署。后续验证及外部操作仍按用户当时授权执行，不能从文档状态自行推导权限。

## 历史逐课交付与验证分栏

下文保留各节点当时范围、交接和失败；“后续”“未接”“暂停”描述该节点时间点，不覆盖上面的当前契约。比如12当时只有SQLite，当前四driver均已接；18当时拒resolve，39已接可选动作；31当时sqlmock-only，32及本轮全盘已有独占PG实证。数字和原失败不补造为后来的整轮重跑。

### 08局部进展

[显式采集与日志旁路](lessons/08-runtime-capture.md)已有Capture/LogMessage→Builder/Buffer，有限EventScope/EventID与原子TagSource，真实Goravel1.18.0 Writer消费和安全OnError/Status。低rank不改、不同ID正常累加、unknown不盲重试；不声称等价于所有PHP Throwable对象自动识别。未接Host全局Recover/HTTP500/queue/schedule，不自动联网；M02/M18其余条目仍待完成。

### 09局部进展

[HTTP生命周期](lessons/09-http-capture.md)已有独立请求scope、有限method/url/ip/显式身份快照、明确终态ID关联及5xx合成观察；显式owner Recover组合保证原callback执行，未配置时交原框架。真实内存Gin定向含已提交响应、旁路故障与并发隔离。未改Host、未注册全局Recover、不主动读body、不自动Cloud；payload/完整过滤及queue/schedule/slowSQL矩阵仍待后续实现。

### 10局部进展

[Host本地运行时](lessons/10-host-runtime.md)消费SDK，默认关闭、不自动Cloud；实际认证要求Identity(context.Context)在事件时刻复制当前身份，SDK不保存原请求。Host41的Provider/日志stack/私有Buffer与响应证据单列，未把workspace接入当发布解析。自动payload/完整采集矩阵与真实Cloud仍待完成。

### 11局部进展

[显式慢SQL链](lessons/11-slow-query-pipeline.md)已有typed ReportSlowQuery、源hash/绑定/舍入fixture、批准的双SQL持久脱敏/真UTF8 byte长度/有效截断、共享私有store聚合恢复及SQL typed Cloud同步/HTTP。Runtime外壳与TagSource保留，SQL不伪装异常；新旧必要消费者已定向验证。没有自动数据库hook或真实Cloud/SQL执行，语法脱敏盲区/云端提示边界及发布解析仍待。

### 12 SDK局部进展

[真实GORM慢SQL回调](lessons/12-gorm-slow-query.md)可显式安装到已存在SQLite Goravel Query，六core后同步复制SQL/Vars，不改业务结果/原Trace调用次数；启动失败回滚/有限Close/安全诊断/真实并发已定向验证。DB façade sqlx、未安装连接、Fresh、其它driver和Rows后续迭代未自动覆盖。Host独立显式业务预热与allowlist接线续见13，不以测试Attach冒充全连接覆盖；不连接真实Cloud。

### 13局部进展

[Host慢SQL消费边界](lessons/13-host-slow-query.md)新增现生命周期只读标量入口与原生数值/锁定Carbon/DeletedAt适配，不调用任意Valuer。真实Host认证Food逐动作INSERT/UPDATE/SELECT目标验证，与SDK单module/race证据分列。Host独立业务预热默认空，按源原生默认DB基线解释，不把监控开关当连接创建器；关闭可重试与真实Cloud/发布解析仍分界。

### 14局部进展与暂停恢复

[队列最终失败与调度完成](lessons/14-queue-schedule.md)已有真实Worker末次策略旁路、原成熟Database Driver私有SQLite采用、Schedule专用Artisan/回调/显式code回执，实际定向race7.333。保原重试/删除/失败落盘/cron恢复，不解析日志。终态观察先于TaskToJson，原failed日志仍可能含payload，Job无ctx不自动贯通日志同ID；未知exit不造1，真实os.Exit不可拦。

第14步暂停后，用户已恢复并授权15/16两个Host采用功能：显式owner默认关闭，原失败日志窄保护与真实生命周期分别验收，全局Artisan仍不替换。该历史交接由下文当前进展接续；用户练习、真实Cloud/MCP与其余矩阵仍未完成。

### 15：Host队列Owner生命周期

显式DisableCapture、单Run与Shutdown、首N Pop/单Receive屏障已见[计划15](plans/15-host-queue-owner.md)。最终TestQueue范围race9.924s，Host四模式真实Bootstrap/官方SQLite队列表6.600s；queue-only日志代理不覆盖SQL日志。随后仅schedule Host采用，完成两功能暂停；其余事件/Cloud/目标部署仍待。

### 16：Host定时任务采用与本轮暂停

[计划16](plans/16-host-schedule.md)对应Host46原Schedule单实例、专用观察Artisan/具名Event与原生日志窄保护；关闭必须原生job排空和Run done同时成立。无SDK生产API或依赖变化，Host开发workspace证据与SDK独立发布解析分开。两功能完成后暂停，不启动第三块；自动payload过滤、跨无ctx业务日志事件身份、真实os.Exit、其它DB路径/真实Cloud与发布解析仍是明确未验边界。

### 17：显式与周期增量采用

见[计划17](plans/17-cloud-push-owner.md)：复用原逐version ACK/type gate+flock，借Buffer、独立HTTP所有权与首tick后推送。SDK四项race1.657，Host两项race1.645/六模式9.873；无真实Cloud。完整all/dry-run/retention及目标兼容仍待，本轮第二功能仅健康检查后暂停。

### 18：健康心跳与saved级自检

见[计划18](plans/18-health-check.md)：心跳typed白名单及每轮独立结果、现Builder固定样本、精确逐条saved与实际命令退出。SDK健康早轮race1.487、实际推送心跳补项1.317、稳定身份/时间补断言1.409；Host10模式真实CLI race5.459，受影响旧推送五项race8.950/metadata配置0.437另列计划。只本机httptest，无真实Cloud；Go版本consumer兼容、summary、resolve/MCP与完整M14/M15余项仍待。两功能完成后暂停，下一范围由用户决定。

### 19：精确pending读取

[计划19](plans/19-pending-statistics.md)为S-Cloud一期提供client-free类型统计，复用ACK/锁/快照，不推进状态；构造可能建锁/恢复另列。Stats/Pusher/选定Sync race3.430、旧Snapshot2.102、Host49五模式7.597。local确认后清理协调仍由下一独立功能完成，不提前冒称交付。

### 20：确认后的local噪音抑制与本轮暂停

[计划20](plans/20-local-noise-discard.md)采用独立suppression而非删除record或伪造ACK，Stats/Sync共同选版本，保累计/overflow；真实远端仍是project-wide local unresolved。SDK新增race2.582，Hostgate1.549/11模式14.731；旧共享SDK2.977/Host49五模式7.517分栏。只本机HTTP与自有Root，无真实Cloud/全量/E2E。19/20后暂停，S-Cloud页面/真实Cloud兼容与余下monitor能力待未来授权。

### 21：只读推送预览

[计划21](plans/21-push-preview.md)：无Client Preview复用严格Stats，零HTTP/heartbeat/状态写。SDKrace2.098s，Host51五模式7.637s；下一项All独立交付。

### 22：显式All重推与本轮暂停

[计划22](plans/22-explicit-all-replay.md)复用原主链，手动忽略历史ACK选集但保exact suppression；Remaining只表示本次余项，error/Failed仍是最终判据，自动Tick继续增量。新SDK真HTTP首测1.717/race2.787；Host八模式race11.254、旧SDK2.973/HostPreview7.636/invalid0.847分栏。只当前live版本，无恢复已删或retention，本轮21/22后暂停；真实Cloud/发布兼容和后续M矩阵仍待。

### 23：确认resolved维护

[计划23](plans/23-confirmed-resolved-maintenance.md)提供store-only本地维护，正数非天龄，完整ACK resolved才候选，实际删除拒journal不恢复；open等保护。新SDKrace1.940、旧buffer1.600、Host10模式13.427分栏。当前按用户要求[持续执行](execution-queue.md)，历史两项后暂停是当时安排；不自动push/真实Cloud/全量/E2E。

### 24：只读迁移体检

[计划24](plans/24-migration-inspection.md)提供显式源路径、固定桶与浅envelope/严格cursor/旧配置键参考映射，只报安全分类不导入。SDK最终0.348s、Host54五模式1.255s；ImportSupported=false，后续真实转换导入按[执行队列](execution-queue.md)持续设计，不把体检当M16完成。

### 26：MCP stdio协议循环

[计划26](plans/26-mcp-stdio-loop.md)实现独立Toolset消费/三固定版本、精确JSON数值ID、限长/安全诊断与取消。普通0.316s、补强前race2.359s，最后stderr短写与structured文本回退两项0.683s。无Host桥或真实Cloud工具；25转换导入并行，继续[队列](execution-queue.md)。

### 27：七个Cloud MCP工具

[计划27](plans/27-cloud-mcp-tools.md)复用26loop，typed七操作、分页闭合、私有图片与精确状态回执；新race mcpcloud1.744/cloudsync1.479，补强状态/布尔单项0.084另列（race为补强前）。仅假HTTP/stdio，无真实MCP；28独立Host桥继续，25导入同时实施。

### 28：Host独立MCP入口

[计划28](plans/28-host-mcp-entry.md)采用独立cmd/monitor-mcp，不import业务bootstrap/config/facades/owner，不读.env。首轮六子例完成但包因signal超时failed；pollable stdin修后signal/flags0.402、Run资源错误0.407分栏，初配置0.727；实际binary/依赖闭包已核，未真实MCP。25导入仍进行，29过滤/payload按设计继续。

### 25：受控转换与新scope导入

[计划25](plans/25-legacy-record-import.md)保hash/累计/时间/桶及真实PHP上下文，未知类型整Plan拒绝；SQL共享双mask，显式键无静默默认。离线Plan封印源集合/全文和目标镜像；Apply只新scope，同plan同bytes可续，不冒领活动ACK状态，源不删/cursor不转ACK。Apply首测1.043；必要race migration2.734/buffer1.751/records1.406/slow1.513，末tag/UTF8路径守卫仅0.071分栏。Host55十模式12.771，未真实迁移或legacy Cloud接入。

### 29：明确过滤与可选HTTP payload

[计划29](plans/29-filtered-http-payload.md)用能力接口/显式CLI标记过滤，不猜消息或Join整链；payload默认关闭，只取已解析All、5000预预算、上传静态类型和既有mask。修前race capture4.844/adapters5.278；终读小修0.630/0.338。Host七模式首包1.634因bad-json夹具失败，唯一修正子例0.900；统一HTTP/Builder cap后两新子例0.933，不写成七模式最后整组重跑通过。后续30源码上下文独立推进。

### 31：Postgres展示SQL

[计划31](plans/31-postgres-slow-query.md)用成熟dialector和sqlmock分层验证，固定依赖与原SQLite/Slow消费者定向证据见课文；未连接真实PG。

### 30：有界源码上下文

[计划30](plans/30-source-context.md)保存原身份后附加显示，Reader/Capture新race1.271/1.419、旧scope2.124；失败保原Frames修正单项0.815（race修前）。Host58 Runtime1.715/真实bootstrap5.422，均自有fixture且无网络。

### 33：M05第一阶段私有崩溃诊断

[计划33](plans/33-crash-output.md)使用公开SetCrashOutput、单Owner、私有raw与有界Inspect，SDK新范围race2.495/Host配置1.508/Bootstrap4.335。原stderr保留，raw未脱敏且无写quota；M05后续显式诊断导入/幂等/cap/count仍待，不自动Cloud或删非空报告。

### 32：DB façade同池观察与真实PG补证

[计划32](plans/32-db-facade-observation.md)保原CommonBuilder/Query/Tx与lazy DB解析，不预建第二池；SDK新race6.540/旧PrepareSQL6.781、Hostenabled8.042与负例、root私有真实PG两路径0.944分栏。实际表已清/池关/服务停，非生产部署或所有池验证。

### 35：typed摘要只读接口

[计划35](plans/35-cloud-summary.md)显式summary，保原安全client/limit1..10、typed计数/nullable与有限recent；SDK新race1.521、Host62六模式4.170，无真实Cloud/首页UI。Cloud三消费者兼容仅[兼容方案](plans/cloud-go-compatibility.md)，第四仓已本地提交兼容修改，未部署。

### 34：显式单ID诊断导入

[计划34](plans/34-crash-diagnostic-import.md)复用Builder与同buffer锁、v2固定图像/永久receipt、有限预算，不再用Capture+done或冷Import冒幂等。SDK必要race3.215/1.683，Host61十模式12.637/旧配置1.546；无真实raw/Cloud，原材料保留，后续普通pending可独立同步。

### 36：日志采集策略

[计划36](plans/36-log-capture-policy.md)保旧NewDriver兼容，显式policy分别控制typed异常与普通消息，过滤先于采集预算；SDK新2+旧Writer3 race2.637，Host63七模式race9.520。原生sink不受监控选择策略替换。

### 37：真实构建元信息

[计划37](plans/37-go-build-metadata.md)保真实版本/unknown/development，SDK新旧wire race1.381、Host64 producer2.966/MCP1.367与旧push0.924分栏。不暴露VCS/Path，不把BuildInfo当发布/部署证明。

### 38：独立生产依赖构建

[计划38](plans/38-release-reproducibility.md)固定三个HEAD归档、成熟module zip/file proxy与两target readonly build，Go1.27.3/1.26.8同源同图，完整临时MVS差异保留。快照早于P1账户，后续新SHA另报；没有执行产物、远端发布或真实Cloud。

### 39：自检saved后可选解决

[计划39](plans/39-self-test-resolution.md)保Run兼容，显式解决仅固定sample；SDK新普通3.002/旧Run0.419、Host68九模式race9.974分栏。真实Cloud resolve未执行。当前剩余分为已实现、待实现、平台差异、外部待验，见[执行队列](execution-queue.md)；39时M06 MySQL/SQLServer观察仍待实现；MySQL后见40，SQLServer后见[41](plans/41-sqlserver-slow-query.md)，当前本地代码与外部验证分栏见执行队列。

### 40：MySQL分层双入口观察

[计划40](plans/40-mysql-slow-query.md)采用官方goravel/mysql1.18、gormmysql1.6及同池Factory；新GORM普通1.130/Factory0.759，必要新旧race7.137/6.858，Host69新consumer race2.365。真实MySQL服务/版本探测/TLS未验；该40节点当时M06 SQLServer仍待实现；后续[41](plans/41-sqlserver-slow-query.md)已接本地代码，真实服务继续外部待验。

### 41：SQLServer双入口与有限OUTPUT掩码

[计划41](plans/41-sqlserver-slow-query.md)核真实create/update替换和同池Factory；新callback首次发现OUTPUT间隔令原INSERT mask漏值，窄扩INSERTED标识符列表后原fixture+新mask0.344/六callback0.732，Factory1.213、Host70新consumer race2.317分栏。没有真实SQLServer服务；未知OUTPUT表达式仍是有限pattern盲区，不能称全面脱敏。紧接固定三SHA最低线生产build与P1页面，不重复旧组。
