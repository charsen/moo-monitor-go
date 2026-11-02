# TODOS

- [ ] 按 [复现计划](docs/port-plan.md)核对 PHP 每项采集来源、配置、缓冲格式、Cloud 与 MCP 契约，补齐对应 Go 行为设计。
- [ ] 按当前已支持HTTP/日志/显式安装SQLite ORM与14队列/调度owner边界继续补未覆盖驱动、真实默认Host采用及其它事件；不把已实现入口笼统记为全部待接。
- [ ] 核对 Cloud 心跳字段白名单、SDK 版本诊断和 runtime/慢 SQL 数据格式对 Go 的兼容缺口；先制定增量兼容方案，不伪装成 PHP SDK。

- [ ] 在首个实现阶段落实[代码设计标准](../go-moo-engine-skeleton/docs/code-design.md)：审查参考业务调用链、文件职责、抽象必要性及生成/手写协作；文档制定不等于代码验收。

- [ ] 完成[第01课](docs/lessons/01-sensitive-masker.md)固定假值预测、三入口范围与初始化错误隔离复述；代理参考验证不代做用户练习。
- [ ] 完成[第02课](docs/lessons/02-sql-masker.md)固定假SQL预测、两阶段与语法盲区复述，代理fixture一致性不代做用户练习。
- [ ] 完成[第03课](docs/lessons/03-bounded-payload.md)共享预算/截断形状/map原序差异与类型边界练习，代理验证不代做学习验收。
- [ ] 完成[第04课](docs/lessons/04-runtime-records.md)hash归一顺序、count/daily与Snapshot采集边界练习，代理源fixture通过不代做学习验收。
- [ ] 继续M02采集/Dispatcher、M08缓冲维护策略及后续真实Cloud接线与M07未知请求表示/Host适配；坏JSON原日志预防与M18接入另行设计，有限Masker不等于全面脱敏。

- [ ] 完成[第05课](docs/lessons/05-runtime-buffer.md)cap/overflow、journal最终图像、结果未知与私有scope练习，代理定向恢复测试不代做用户验收。

- [ ] 完成[第06课](docs/lessons/06-incremental-sync.md)同毫秒版本、四结果/失败洞、至少一次与扫描边界练习；参考race不代做学习验收。
- [ ] 在获准范围验证07 HTTPClient与真实Cloud Go字段/回执兼容，并设计Discard成功后的本地pending清理协调；本机httptest不等于远端兼容。

- [ ] 完成[第07课](docs/lessons/07-cloud-http-client.md)timeout/caller取消、body token重定向、wire失败与传输/确认边界练习，代理HTTP测试不代做学习验收。

- [ ] 完成[第08课](docs/lessons/08-runtime-capture.md)事件身份/unknown标记/来源升级/安全诊断练习；代理Writer与race验证不代做学习验收。
- [ ] 后续接真实Host生命周期、panic/HTTP500/queue/schedule钩子与完整过滤策略；08仅显式Capture和日志适配，不替现GinLogSafety。

- [ ] 完成[第09课](docs/lessons/09-http-capture.md)请求scope/终态事件关联/Recover时序/已提交响应练习；代理内存路由测试不代做学习验收。
- [ ] 在后续Host接线保留现GinLogSafety/认证与原Recover行为，核请求自动payload过滤、queue最终失败/schedule退出及慢SQL真实扩展点；09未覆盖这些能力。

- [ ] 完成[第10课](docs/lessons/10-host-runtime.md)late identity/Host所有权与workspace验证差异练习；真实Cloud/部署解析仍待，代理验收不代做学习。

- [ ] 完成[第11课](docs/lessons/11-slow-query-pipeline.md)原raw身份/双mask安全差异、字节与码点、cap峰值/overflow、共享typed内核与同ms确认练习；代理参考验证不代做学习。
- [ ] 继续DB façade、其它驱动/占位符、未安装/Fresh连接的采集方案；当前已安装SQLite GORM实例有自动观察，不能概括全数据库覆盖。

- [ ] 完成[第12课](docs/lessons/12-gorm-slow-query.md)回调窗口/安装回滚/原Trace观测开销、事务语义与连接覆盖练习，代理真实ORM验证不代做学习。

- [ ] 完成[第13课](docs/lessons/13-host-slow-query.md)请求快照/标准日期与任意Valuer边界、目标写链断言、原生DB启动baseline/预热及关闭重试练习；代理验证不代做学习。

- [ ] 完成[第14课](docs/lessons/14-queue-schedule.md)最终重试/原失败持久化、未知exit/真实panic、scope与原生日志边界练习；代理race不代做学习。
- [ ] **当前持续推进**：21只读预览与22显式All重推参考已交付，按[执行队列](docs/execution-queue.md)继续23以后功能；此前暂停保留历史证据。其余按矩阵确认剩余范围：无ctx业务日志身份、自动payload过滤、真实os.Exit/Cloud与目标部署解析仍未完成，全局Artisan仍不替换。

- [ ] 完成[第15课](docs/lessons/15-host-queue-owner.md)单Run/首轮屏障/在途关闭与DisableCapture练习；Host45参考已接，15/16已参考交付，当时暂停；当前按执行队列持续推进。

- [ ] 完成[第16课](docs/lessons/16-host-schedule.md)双开关/原callback元信息、未知exit与真实回执、Shutdown排空和安全内部诊断练习；代理验证不代做学习。

- [ ] 完成[第17课](docs/lessons/17-cloud-push-owner.md)双类partial/Busy/关闭与无变化练习；all/dry见21/22、确认resolved维护见23；继续完整维护余项、全面Cloud兼容及部署解析，不把当前增量采用当完整M13。

- [ ] 完成[第18课](docs/lessons/18-health-check.md)心跳与saved双证据、clamp/caller取消、重复样本max语义及显式自检副作用练习；继续真实Cloud字段/权限、summary/resolve及部署解析，不能把本机参考验收写为M14/M15全部完成。

- [ ] 完成[第19课](docs/lessons/19-pending-statistics.md)完整指纹、同ms更新、Known/Busy/Unknown与构造/读取副作用练习；local噪音协调已见20，当前后续仅21/22，不自动push。

- [ ] 完成[第20课](docs/lessons/20-local-noise-discard.md)remote project/local scope、历史ACK/抑制/cursor、Unknown与cap冻结练习；本机替身不代替真实Cloud或用户学习。后续S-Cloud页面/其它monitor矩阵需另定范围，19/20当时收口；21/22已参考交付，当前按执行队列持续推进。

- [ ] 完成[21课](docs/lessons/21-push-preview.md)只读/Unknown/版本选择练习；显式All已见22，确认resolved维护已见23，完整维护余项仍待，不自动push。

- [ ] 完成[第22课](docs/lessons/22-explicit-all-replay.md)模式选集/抑制重叠、Pending与Remaining、最终持久化error及Tick不变练习；用户学习待完成。确认resolved维护见23；全面Cloud/目标发布解析仍待，当前持续推进。

- [ ] 当前持续推进：23维护→24只读迁移体检→后续设计按[执行队列](docs/execution-queue.md)；每功能验收commit，不再两项后自动暂停，不自动push。用户23自主练习待完成。

- [ ] 完成[第24课](docs/lessons/24-migration-inspection.md)浅envelope/完整兼容、cursor非ACK与源保留练习；代理参考验证不代做用户学习。继续[当前执行队列](docs/execution-queue.md)的真实转换导入设计，不读取真实业务记录。

- [ ] 完成[第26课](docs/lessons/26-mcp-stdio-loop.md)精确ID/生命周期、structured回退与owned输入取消练习；代理测试不代做学习。继续七Cloud工具与Host显式采用，不把协议loop当业务闭环完成。

- [ ] 完成[27课](docs/lessons/27-cloud-mcp-tools.md)七工具/分页闭合/图片与状态幂等练习；28隔离Host入口继续，真实MCP外部验证仍需授权。

- [ ] 完成[28课](docs/lessons/28-host-mcp-entry.md)专用环境变量、stdout隔离与stdin/SIGTERM所有权练习；继续29过滤/payload及monitor余项，真实MCP不能借健康自检授权自动调用。

- [ ] 完成[25课](docs/lessons/25-legacy-record-import.md)来源固定副本、完整typed转换、源集合/摘要漂移、新scope归属及部分续作练习；真实迁移/legacy Cloud字段验证仍需对应授权，后续按持续队列执行。

- [ ] 完成[29课](docs/lessons/29-filtered-http-payload.md)单unwrap/Join过滤、入口脱敏副本与HTTP/Builder同cap练习；30源码Reader按设计继续，真实token配置不进入本地测试。
