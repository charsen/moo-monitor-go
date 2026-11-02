# Moo Go Monitor 全业务复刻计划

## 定位和边界

用户于 2026-10-28 明确指定 `moo-go-monitor` 为必需项目，与 skeleton、scaffold 一起构成 Go 业务开发基础。

交付目标为 PHP monitor 全业务复刻，覆盖采集、过滤、去重、聚合、脱敏、缓冲、同步、调度、运维、自检、迁移、MCP 和工作台接入。功能、业务规则、输入输出、持久化及网络协议逐项对照；内部机制按 Go/Goravel 实现。

本包是 headless 监控 SDK：采集、脱敏、聚合、可恢复缓冲、增量同步和运维接口由它负责。Goravel 适配与通用记录/传输能力保持明确边界，是否拆 module 在实现时决定。业务 Host 可以不安装 scaffold 而单独使用 monitor；monitor 不依赖研发 UI。

PHP 复核基线为本机 `moo-monitor-laravel` 提交 `28d424e`。现阶段已盘点入口、阅读关键采集/存储/同步契约，已实现M07纯脱敏与M02显式快照构建部分，并运行受控PHP纯组件probe；M08已有私有scope本地运行时缓冲与intent恢复；06已有显式Client的增量快照/严格回执/持久确认，07已有runtime intake/discard HTTP传输，HTTP/日志及已安装SQLite ORM已有真实Host消费，14提供显式队列/调度owner；默认Host采用与其余驱动/事件、真实Cloud兼容仍待。以上为初次盘点的历史边界；当前状态以[执行队列](execution-queue.md)与后续课程为准。

依据：[PHP README](../../moo-monitor-laravel/README.md)、[Cloud 契约](../../moo-monitor-laravel/docs/cloud-api-contract.md)、[Provider](../../moo-monitor-laravel/src/MooeenMonitorLaravelServiceProvider.php)、[源码与测试索引](php-surface-inventory.md)。

## 完整能力对照

以下编号是能力分类，复核发现的遗漏继续补入。M07-A已有[基础脱敏](lessons/01-sensitive-masker.md)，M07-B已有[SQL值侧有限算法](lessons/02-sql-masker.md)，M07-C已有[递归payload与截断](lessons/03-bounded-payload.md)及各自源fixture；M02已有[运行时记录构建与刷新](lessons/04-runtime-records.md)，覆盖显式输入、脱敏/hash/count/daily/source优先级，该课未自动采集/落盘；M08已在[第05课](lessons/05-runtime-buffer.md)接本地缓冲/cap/持久overflow/重开与恢复。M11/12已有[第06课](lessons/06-incremental-sync.md)指纹确认账本与替身传输验证，07已补[显式HTTP传输](lessons/07-cloud-http-client.md)，仅本机假服务验证，未接真实Cloud。Host已接本地HTTP/日志与显式预热SQLite ORM；未知请求表示/其它driver、queue/schedule默认Host采用与其余条目仍待逐项完成。PHP 特有路径需核实 Go 对应能力；平台限制记为待解决缺口，提出替代方案并说明业务影响，不能自行删项或视为完成，也不宣称捕获所有错误。

| 编号 | PHP 能力与现码入口 | Go 目标及验证重点 |
| --- | --- | --- |
| M01 | Provider/config、主链和旁路独立开关 | 显式 Goravel 适配与配置，无 scaffold 依赖，禁用后不改变 Host 行为 |
| M02 | ExceptionDispatcher、RuntimeErrorRecorder、显式 report | 错误记录与请求恢复观察、业务错误过滤、稳定指纹、堆栈与源码上下文 |
| M03 | 日志 exception/context/字符串化兜底、HTTP 5xx | 日志与 HTTP 可用接入点、去重、来源标记，不改变原响应或覆盖框架恢复逻辑 |
| M04 | 队列失败、重试、调度异常和非零退出 | Goravel queue/schedule 对应事件与退出行为，避免多条旁路重复计数 |
| M05 | PHP fatal/OOM 轻量路径和采集盲区 | 记录 Go panic/recover 的边界、同 goroutine 约束、进程终止等不可可靠采集路径，不类比宣称全覆盖 |
| M06 | SqlSlowListener/Recorder、bindings、连接与应用栈 | 查询耗时、阈值、SQL/binding 脱敏、来源和连接标记，不改变查询或凭空制造身份 |
| M07 | SensitiveMasker、请求/用户/console 语境、payload 截断 | Token/密码/URL/SQL/堆栈的有界脱敏，显式上下文，不把请求对象跨生命周期保存 |
| M08 | BucketedYamlRecorder、三桶、复发、累计 count、cap/overflow | 05已有私有scope本地缓冲/累计/cap/持久overflow/journal恢复；06已接显式Client回执，真实Cloud/缓冲维护与跨格式迁移待接 |
| M09 | 原子写、坏记录/写失败、并发、路径限制 | 适合 Go 常驻并发/多进程的明确存储方案，容量和竞争不会拖垮业务 |
| M10 | StorageScope、cursor、partial ack、锁与 prune 范围 | 同一 Host 多项目/环境的所有状态统一隔离，scope 不串数据 |
| M11 | CloudClient/CloudSync、逐条回执、重试、拒绝隔离 | 06已验严格receipt/失败洞/同指纹隔离与账本；07已有runtime intake/discard传输/有限重试，真实Cloud兼容与自动协调仍待接 |
| M12 | 累计快照、微秒时间、旧回执兼容、复发与 resolve | 与 Cloud 同源 fixture；既有字段语义不变，不把累计 count 当增量求和 |
| M13 | CloudPushCommand、调度、锁、恢复、prune | 正常/无数据推送、幂等/并发/失败恢复；未确认数据不清除 |
| M14 | HeartbeatMeta、summary、discard-local | 心跳和上报结果分开表述，只清获准环境范围，Go 元信息不伪造 PHP/Laravel 版本 |
| M15 | CloudTestCommand、自检回执和后续处置 | 自检必须真正 saved 才成功，fixture 验证与真实云端写入验证分别记录 |
| M16 | MigrateCommand、旧格式/旧目录/环境变量体检 | 评估 PHP 缓冲导入或 Go 版本迁移边界，预览与可恢复迁移，不直接改 PHP 持久化目录 |
| M17 | CloudMcpCommand、McpLoop、CloudToolset | JSON-RPC/stdio、协议协商、结构化/文本兼容、分页和七项工具的等价能力 |
| M18 | SafelyLogs、NeverThrowsIntoHost、CaptureHookSafety | SDK 故障不改变业务响应，不递归采集自身日志/推送失败，内部保留有界诊断与退出码 |
| M19 | scaffold 状态/推送/云端入口及云摘要 | 通过公开接口供 scaffold 接入，不向 monitor 放 UI 或让工作台复制同步逻辑 |

## 需要保留的持久化与网络语义

- 先落可恢复缓冲，再由受控推送发往 Cloud；网络失败不能使业务请求等待完整重试流程。
- hash、累计计数、状态、cursor、partial ack、锁和 scope 是持久化契约。改变格式或指纹时设计版本和迁移，不在两个版本间反复制造同类记录。
- Cloud `results` 必须逐条核对 index、hash、status、retryable 和汇总闭合；保存确认失败不能触发批量清理。
- open 记录承担累计计数锚点；回收策略不能因为“已上传”就丢掉聚合基线。
- PHP 当前记录热路径有 best-effort 近似计数语义。Go 并发实现需明确内存同步、磁盘和多进程边界；既不声称精确计数，也不默认复制 PHP 的无锁策略。
- 监控自身的推送失败不反向形成 runtime 风暴，但 SDK 内部真正的代码错误要有可诊断证据。

## Cloud 和 MCP 接入

复用既有 Cloud 服务的 runtime/slow-query intake、summary、heartbeat、local discard 和 MCP 相关接口；先用当前双方 fixture 校验，再在获准环境做真实联调。

PHP 当前 `CloudToolset` 是七个工具：`list_open_runtimes`、`get_runtime`、`resolve_runtime`、`list_open_todos`、`get_todo`、`get_todo_image`、`update_todo_status`。旧 scaffold 总览仍有“六工具”描述，Go 范围按现码核实，不遗漏现场图片能力。

MCP 需要独立 ability；runtime、slow-query、Todo intake 与 MCP 权限不能互相替代。列表保留 status/limit/offset 与分页回执校验；现场图按项目和附件授权获取，不能生成公开附件地址代替受控读取。

已确认一个 Cloud 兼容缺口：当前 [HeartbeatController](../../moo-scaffold-cloud/app/Http/Controllers/Api/HeartbeatController.php) 只接收 PHP/Laravel 等既有版本字段，没有 Go/Goravel 版本字段；[IntegrationHealthAdvisor](../../moo-scaffold-cloud/app/Services/Monitoring/IntegrationHealthAdvisor.php) 的提示和最低 SDK 版本也按 PHP SDK 编写。需要增量、多 SDK 兼容方案，不能直接把 Go 元信息填进 PHP 字段。本轮仅记录，不修改 Cloud。

PHP 包当前 TODO 中的容量恢复、记录版本/坏 YAML、心跳健康语义、SQL binding 展示和 Cloud 单条坏输入隔离，是行为复核风险线索。Go 设计应验证并解决对应问题或明确边界，不能把现有缺陷当作兼容要求。

## 分阶段实现与学习

| 子阶段 | 范围 | 接入课程 |
| --- | --- | --- |
| M-A | 明确错误事件、脱敏、手动记录与覆盖矩阵 | 第 08 监控支线 |
| M-B | Goravel HTTP/日志/SQL/队列/调度适配，记录覆盖与盲区 | 第 08 监控支线 |
| M-C | 聚合、缓冲、scope、并发、容量、原子写与恢复 | 第 08 监控支线 |
| M-D | Cloud intake、逐条 ack、重试、调度、心跳与自检 | 第 08 至 11G |
| M-E | MCP 七工具、分页、图片、协议协商、迁移兼容 | 第 11G |
| M-F | scaffold 工作台接入、真实 Host、Cloud 适配及多项目验收 | 第 11G 至 12 |

## 代码结构与可读性

遵守[业务代码与结构设计标准](../../go-moo-engine-skeleton/docs/code-design.md)。采集适配、规范化/脱敏、聚合/缓冲、同步和回执推进按责任组织，状态归属与 scope 明确；命令、MCP 和工作台消费同一业务能力。后台工作说明启动、取消、容量和退出责任，错误隔离保留可诊断证据。

各能力设计附实际调用链、文件职责、状态转移与失败处理位置，使维护者能够判断数据何时可重试、何时已确认、何时允许清理。

## 完成标准

每项保留 PHP 行为依据、Go 方案、实际实现、定向测试和 Host/Cloud 验证状态。至少验证独立 Host、skeleton 接入、scaffold 消费、云端不可用、磁盘失败、重复记录、多环境隔离与并发恢复。

按相同业务场景对照两端的记录结果、计数/状态、过滤/脱敏、scope、逐条回执、失败恢复及 MCP/Cloud 行为。Go 等价适配须保留完整业务能力；任何取消、收缩或业务含义变化由用户明确确认，不能借性能、语言差异或阶段结束自行降级交付要求。

验证选择相关单元测试及定向接口/集成；跨系统完整流程、浏览器/E2E、真实云端写入、真实消息与生产部署按明确授权执行。协议替身通过不代表真实 Cloud 兼容已经完成。

## 08局部进展

[显式采集与日志旁路](lessons/08-runtime-capture.md)已有Capture/LogMessage→Builder/Buffer，有限EventScope/EventID与原子TagSource，真实Goravel1.18.0 Writer消费和安全OnError/Status。低rank不改、不同ID正常累加、unknown不盲重试；不声称等价于所有PHP Throwable对象自动识别。未接Host全局Recover/HTTP500/queue/schedule，不自动联网；M02/M18其余条目仍待完成。

## 09局部进展

[HTTP生命周期](lessons/09-http-capture.md)已有独立请求scope、有限method/url/ip/显式身份快照、明确终态ID关联及5xx合成观察；显式owner Recover组合保证原callback执行，未配置时交原框架。真实内存Gin定向含已提交响应、旁路故障与并发隔离。未改Host、未注册全局Recover、不主动读body、不自动Cloud；payload/完整过滤及queue/schedule/slowSQL矩阵仍待后续实现。

## 10局部进展

[Host本地运行时](lessons/10-host-runtime.md)消费SDK，默认关闭、不自动Cloud；实际认证要求Identity(context.Context)在事件时刻复制当前身份，SDK不保存原请求。Host41的Provider/日志stack/私有Buffer与响应证据单列，未把workspace接入当发布解析。自动payload/完整采集矩阵与真实Cloud仍待完成。

## 11局部进展

[显式慢SQL链](lessons/11-slow-query-pipeline.md)已有typed ReportSlowQuery、源hash/绑定/舍入fixture、批准的双SQL持久脱敏/真UTF8 byte长度/有效截断、共享私有store聚合恢复及SQL typed Cloud同步/HTTP。Runtime外壳与TagSource保留，SQL不伪装异常；新旧必要消费者已定向验证。没有自动数据库hook或真实Cloud/SQL执行，语法脱敏盲区/云端提示边界及发布解析仍待。

## 12 SDK局部进展

[真实GORM慢SQL回调](lessons/12-gorm-slow-query.md)可显式安装到已存在SQLite Goravel Query，六core后同步复制SQL/Vars，不改业务结果/原Trace调用次数；启动失败回滚/有限Close/安全诊断/真实并发已定向验证。DB façade sqlx、未安装连接、Fresh、其它driver和Rows后续迭代未自动覆盖。Host独立显式业务预热与allowlist接线续见13，不以测试Attach冒充全连接覆盖；不连接真实Cloud。

## 13局部进展

[Host慢SQL消费边界](lessons/13-host-slow-query.md)新增现生命周期只读标量入口与原生数值/锁定Carbon/DeletedAt适配，不调用任意Valuer。真实Host认证Food逐动作INSERT/UPDATE/SELECT目标验证，与SDK单module/race证据分列。Host独立业务预热默认空，按源原生默认DB基线解释，不把监控开关当连接创建器；关闭可重试与真实Cloud/发布解析仍分界。

## 14局部进展与暂停恢复

[队列最终失败与调度完成](lessons/14-queue-schedule.md)已有真实Worker末次策略旁路、原成熟Database Driver私有SQLite采用、Schedule专用Artisan/回调/显式code回执，实际定向race7.333。保原重试/删除/失败落盘/cron恢复，不解析日志。终态观察先于TaskToJson，原failed日志仍可能含payload，Job无ctx不自动贯通日志同ID；未知exit不造1，真实os.Exit不可拦。

第14步暂停后，用户已恢复并授权15/16两个Host采用功能：显式owner默认关闭，原失败日志窄保护与真实生命周期分别验收，全局Artisan仍不替换。该历史交接由下文当前进展接续；用户练习、真实Cloud/MCP与其余矩阵仍未完成。

## 15：Host队列Owner生命周期

显式DisableCapture、单Run与Shutdown、首N Pop/单Receive屏障已见[计划15](plans/15-host-queue-owner.md)。最终TestQueue范围race9.924s，Host四模式真实Bootstrap/官方SQLite队列表6.600s；queue-only日志代理不覆盖SQL日志。随后仅schedule Host采用，完成两功能暂停；其余事件/Cloud/目标部署仍待。

## 16：Host定时任务采用与本轮暂停

[计划16](plans/16-host-schedule.md)对应Host46原Schedule单实例、专用观察Artisan/具名Event与原生日志窄保护；关闭必须原生job排空和Run done同时成立。无SDK生产API或依赖变化，Host开发workspace证据与SDK独立发布解析分开。两功能完成后暂停，不启动第三块；自动payload过滤、跨无ctx业务日志事件身份、真实os.Exit、其它DB路径/真实Cloud与发布解析仍是明确未验边界。

## 17：显式与周期增量采用

见[计划17](plans/17-cloud-push-owner.md)：复用原逐version ACK/type gate+flock，借Buffer、独立HTTP所有权与首tick后推送。SDK四项race1.657，Host两项race1.645/六模式9.873；无真实Cloud。完整all/dry-run/retention及目标兼容仍待，本轮第二功能仅健康检查后暂停。

## 18：健康心跳与saved级自检

见[计划18](plans/18-health-check.md)：心跳typed白名单及每轮独立结果、现Builder固定样本、精确逐条saved与实际命令退出。SDK健康早轮race1.487、实际推送心跳补项1.317、稳定身份/时间补断言1.409；Host10模式真实CLI race5.459，受影响旧推送五项race8.950/metadata配置0.437另列计划。只本机httptest，无真实Cloud；Go版本consumer兼容、summary、resolve/MCP与完整M14/M15余项仍待。两功能完成后暂停，下一范围由用户决定。

## 19：精确pending读取

[计划19](plans/19-pending-statistics.md)为S-Cloud一期提供client-free类型统计，复用ACK/锁/快照，不推进状态；构造可能建锁/恢复另列。Stats/Pusher/选定Sync race3.430、旧Snapshot2.102、Host49五模式7.597。local确认后清理协调仍由下一独立功能完成，不提前冒称交付。

## 20：确认后的local噪音抑制与本轮暂停

[计划20](plans/20-local-noise-discard.md)采用独立suppression而非删除record或伪造ACK，Stats/Sync共同选版本，保累计/overflow；真实远端仍是project-wide local unresolved。SDK新增race2.582，Hostgate1.549/11模式14.731；旧共享SDK2.977/Host49五模式7.517分栏。只本机HTTP与自有Root，无真实Cloud/全量/E2E。19/20后暂停，S-Cloud页面/真实Cloud兼容与余下monitor能力待未来授权。

## 21：只读推送预览

[计划21](plans/21-push-preview.md)：无Client Preview复用严格Stats，零HTTP/heartbeat/状态写。SDKrace2.098s，Host51五模式7.637s；下一项All独立交付。

## 22：显式All重推与本轮暂停

[计划22](plans/22-explicit-all-replay.md)复用原主链，手动忽略历史ACK选集但保exact suppression；Remaining只表示本次余项，error/Failed仍是最终判据，自动Tick继续增量。新SDK真HTTP首测1.717/race2.787；Host八模式race11.254、旧SDK2.973/HostPreview7.636/invalid0.847分栏。只当前live版本，无恢复已删或retention，本轮21/22后暂停；真实Cloud/发布兼容和后续M矩阵仍待。

## 23：确认resolved维护

[计划23](plans/23-confirmed-resolved-maintenance.md)提供store-only本地维护，正数非天龄，完整ACK resolved才候选，实际删除拒journal不恢复；open等保护。新SDKrace1.940、旧buffer1.600、Host10模式13.427分栏。当前按用户要求[持续执行](execution-queue.md)，历史两项后暂停是当时安排；不自动push/真实Cloud/全量/E2E。

## 24：只读迁移体检

[计划24](plans/24-migration-inspection.md)提供显式源路径、固定桶与浅envelope/严格cursor/旧配置键参考映射，只报安全分类不导入。SDK最终0.348s、Host54五模式1.255s；ImportSupported=false，后续真实转换导入按[执行队列](execution-queue.md)持续设计，不把体检当M16完成。

## 26：MCP stdio协议循环

[计划26](plans/26-mcp-stdio-loop.md)实现独立Toolset消费/三固定版本、精确JSON数值ID、限长/安全诊断与取消。普通0.316s、补强前race2.359s，最后stderr短写与structured文本回退两项0.683s。无Host桥或真实Cloud工具；25转换导入并行，继续[队列](execution-queue.md)。

## 27：七个Cloud MCP工具

[计划27](plans/27-cloud-mcp-tools.md)复用26loop，typed七操作、分页闭合、私有图片与精确状态回执；新race mcpcloud1.744/cloudsync1.479，补强状态/布尔单项0.084另列（race为补强前）。仅假HTTP/stdio，无真实MCP；28独立Host桥继续，25导入同时实施。

## 28：Host独立MCP入口

[计划28](plans/28-host-mcp-entry.md)采用独立cmd/monitor-mcp，不import业务bootstrap/config/facades/owner，不读.env。首轮六子例完成但包因signal超时failed；pollable stdin修后signal/flags0.402、Run资源错误0.407分栏，初配置0.727；实际binary/依赖闭包已核，未真实MCP。25导入仍进行，29过滤/payload按设计继续。

## 25：受控转换与新scope导入

[计划25](plans/25-legacy-record-import.md)保hash/累计/时间/桶及真实PHP上下文，未知类型整Plan拒绝；SQL共享双mask，显式键无静默默认。离线Plan封印源集合/全文和目标镜像；Apply只新scope，同plan同bytes可续，不冒领活动ACK状态，源不删/cursor不转ACK。Apply首测1.043；必要race migration2.734/buffer1.751/records1.406/slow1.513，末tag/UTF8路径守卫仅0.071分栏。Host55十模式12.771，未真实迁移或legacy Cloud接入。

## 29：明确过滤与可选HTTP payload

[计划29](plans/29-filtered-http-payload.md)用能力接口/显式CLI标记过滤，不猜消息或Join整链；payload默认关闭，只取已解析All、5000预预算、上传静态类型和既有mask。修前race capture4.844/adapters5.278；终读小修0.630/0.338。Host七模式首包1.634因bad-json夹具失败，唯一修正子例0.900；统一HTTP/Builder cap后两新子例0.933，不写成七模式最后整组重跑通过。后续30源码上下文独立推进。

## 31：Postgres展示SQL

[计划31](plans/31-postgres-slow-query.md)用成熟dialector和sqlmock分层验证，固定依赖与原SQLite/Slow消费者定向证据见课文；未连接真实PG。

## 30：有界源码上下文

[计划30](plans/30-source-context.md)保存原身份后附加显示，Reader/Capture新race1.271/1.419、旧scope2.124；失败保原Frames修正单项0.815（race修前）。Host58 Runtime1.715/真实bootstrap5.422，均自有fixture且无网络。

## 33：M05第一阶段私有崩溃诊断

[计划33](plans/33-crash-output.md)使用公开SetCrashOutput、单Owner、私有raw与有界Inspect，SDK新范围race2.495/Host配置1.508/Bootstrap4.335。原stderr保留，raw未脱敏且无写quota；M05后续显式诊断导入/幂等/cap/count仍待，不自动Cloud或删非空报告。

## 32：DB façade同池观察与真实PG补证

[计划32](plans/32-db-facade-observation.md)保原CommonBuilder/Query/Tx与lazy DB解析，不预建第二池；SDK新race6.540/旧PrepareSQL6.781、Hostenabled8.042与负例、root私有真实PG两路径0.944分栏。实际表已清/池关/服务停，非生产部署或所有池验证。

## 35：typed摘要只读接口

[计划35](plans/35-cloud-summary.md)显式summary，保原安全client/limit1..10、typed计数/nullable与有限recent；SDK新race1.521、Host62六模式4.170，无真实Cloud/首页UI。Cloud三消费者兼容仅[待授权方案](plans/cloud-go-compatibility.md)，不改第四仓。
