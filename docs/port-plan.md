# Moo Go Monitor 全业务复刻计划

## 定位和边界

用户于 2026-10-28 明确指定 `moo-go-monitor` 为必需项目，与 skeleton、scaffold 一起构成 Go 业务开发基础。

交付目标为 PHP monitor 全业务复刻，覆盖采集、过滤、去重、聚合、脱敏、缓冲、同步、调度、运维、自检、迁移、MCP 和工作台接入。功能、业务规则、输入输出、持久化及网络协议逐项对照；内部机制按 Go/Goravel 实现。

本包是 headless 监控 SDK：采集、脱敏、聚合、可恢复缓冲、增量同步和运维接口由它负责。Goravel 适配与通用记录/传输能力保持明确边界，是否拆 module 在实现时决定。业务 Host 可以不安装 scaffold 而单独使用 monitor；monitor 不依赖研发 UI。

PHP 复核基线为本机 `moo-monitor-laravel` 提交 `28d424e`。现阶段已盘点入口、阅读关键采集/存储/同步契约，尚未运行 PHP 行为验证或实现 Go 功能。

依据：[PHP README](../../moo-monitor-laravel/README.md)、[Cloud 契约](../../moo-monitor-laravel/docs/cloud-api-contract.md)、[Provider](../../moo-monitor-laravel/src/MooeenMonitorLaravelServiceProvider.php)、[源码与测试索引](php-surface-inventory.md)。

## 完整能力对照

以下编号是能力分类，复核发现的遗漏继续补入。当前M07-A已有[基础脱敏纯函数与源fixture](lessons/01-sensitive-masker.md)，M07-B已有[SQL值侧有限算法与源fixture](lessons/02-sql-masker.md)；递归payload及其余条目的实现、Host/Cloud验证仍待逐项完成。PHP 特有路径需核实 Go 对应能力；平台限制记为待解决缺口，提出替代方案并说明业务影响，不能自行删项或视为完成，也不宣称捕获所有错误。

| 编号 | PHP 能力与现码入口 | Go 目标及验证重点 |
| --- | --- | --- |
| M01 | Provider/config、主链和旁路独立开关 | 显式 Goravel 适配与配置，无 scaffold 依赖，禁用后不改变 Host 行为 |
| M02 | ExceptionDispatcher、RuntimeErrorRecorder、显式 report | 错误记录与请求恢复观察、业务错误过滤、稳定指纹、堆栈与源码上下文 |
| M03 | 日志 exception/context/字符串化兜底、HTTP 5xx | 日志与 HTTP 可用接入点、去重、来源标记，不改变原响应或覆盖框架恢复逻辑 |
| M04 | 队列失败、重试、调度异常和非零退出 | Goravel queue/schedule 对应事件与退出行为，避免多条旁路重复计数 |
| M05 | PHP fatal/OOM 轻量路径和采集盲区 | 记录 Go panic/recover 的边界、同 goroutine 约束、进程终止等不可可靠采集路径，不类比宣称全覆盖 |
| M06 | SqlSlowListener/Recorder、bindings、连接与应用栈 | 查询耗时、阈值、SQL/binding 脱敏、来源和连接标记，不改变查询或凭空制造身份 |
| M07 | SensitiveMasker、请求/用户/console 语境、payload 截断 | Token/密码/URL/SQL/堆栈的有界脱敏，显式上下文，不把请求对象跨生命周期保存 |
| M08 | BucketedYamlRecorder、三桶、复发、累计 count、cap/overflow | 可恢复状态与稳定聚合，容量、频控、重启恢复与计数语义明确 |
| M09 | 原子写、坏记录/写失败、并发、路径限制 | 适合 Go 常驻并发/多进程的明确存储方案，容量和竞争不会拖垮业务 |
| M10 | StorageScope、cursor、partial ack、锁与 prune 范围 | 同一 Host 多项目/环境的所有状态统一隔离，scope 不串数据 |
| M11 | CloudClient/CloudSync、逐条回执、重试、拒绝隔离 | saved/filtered 最终确认，retryable skipped 单项保留，non-retryable 隔离；畸形回执不推进水位 |
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
