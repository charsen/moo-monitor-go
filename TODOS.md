# TODOS

- [x] 当前Monitor本地业务代码与Host消费已交付；2026-11-03全包race、私有PostgreSQL/Redis及受影响Host业务验证已完成，首失败和复验见[全盘矩阵](../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。
- [ ] 按[执行队列](docs/execution-queue.md)保留真实MCP权限/Cloud剩余操作、MySQL/SQLServer服务、目标部署/跨OS、远端发布解析的外部或平台验证；本地合入不等于远端发布。

- [x] 单条runtime记录的Go/Goravel版本接收、持久化、读回、Markdown/UI已在Cloud代码层接通（`e9af916`，[兼容设计](docs/plans/cloud-go-compatibility.md)）；目标环境仍需执行`2026_11_03_000001`增量迁移并重启worker，历史记录不伪造版本。

以下仅为用户自主学习项；删除已被后续交付替代的旧执行指令，不自动勾选练习。

- [ ] 完成[第01课](docs/lessons/01-sensitive-masker.md)固定假值预测、三入口范围与初始化错误隔离复述；代理参考验证不代做用户练习。
- [ ] 完成[第02课](docs/lessons/02-sql-masker.md)固定假SQL预测、两阶段与语法盲区复述，代理fixture一致性不代做用户练习。
- [ ] 完成[第03课](docs/lessons/03-bounded-payload.md)共享预算/截断形状/map原序差异与类型边界练习，代理验证不代做学习验收。
- [ ] 完成[第04课](docs/lessons/04-runtime-records.md)hash归一顺序、count/daily与Snapshot采集边界练习，代理源fixture通过不代做学习验收。

- [ ] 完成[第05课](docs/lessons/05-runtime-buffer.md)cap/overflow、journal最终图像、结果未知与私有scope练习，代理定向恢复测试不代做用户验收。

- [ ] 完成[第06课](docs/lessons/06-incremental-sync.md)同毫秒版本、四结果/失败洞、至少一次与扫描边界练习；参考race不代做学习验收。

- [ ] 完成[第07课](docs/lessons/07-cloud-http-client.md)timeout/caller取消、body token重定向、wire失败与传输/确认边界练习，代理HTTP测试不代做学习验收。

- [ ] 完成[第08课](docs/lessons/08-runtime-capture.md)事件身份/unknown标记/来源升级/安全诊断练习；代理Writer与race验证不代做学习验收。

- [ ] 完成[第09课](docs/lessons/09-http-capture.md)请求scope/终态事件关联/Recover时序/已提交响应练习；代理内存路由测试不代做学习验收。

- [ ] 完成[第10课](docs/lessons/10-host-runtime.md)late identity/Host所有权与workspace验证差异练习；真实Cloud/部署解析仍待，代理验收不代做学习。

- [ ] 完成[第11课](docs/lessons/11-slow-query-pipeline.md)原raw身份/双mask安全差异、字节与码点、cap峰值/overflow、共享typed内核与同ms确认练习；代理参考验证不代做学习。

- [ ] 完成[第12课](docs/lessons/12-gorm-slow-query.md)回调窗口/安装回滚/原Trace观测开销、事务语义与连接覆盖练习，代理真实ORM验证不代做学习。

- [ ] 完成[第13课](docs/lessons/13-host-slow-query.md)请求快照/标准日期与任意Valuer边界、目标写链断言、原生DB启动baseline/预热及关闭重试练习；代理验证不代做学习。

- [ ] 完成[第14课](docs/lessons/14-queue-schedule.md)最终重试/原失败持久化、未知exit/真实panic、scope与原生日志边界练习；代理race不代做学习。

- [ ] 完成[第15课](docs/lessons/15-host-queue-owner.md)单Run/首轮屏障/在途关闭与DisableCapture练习；Host45参考已接，15/16已参考交付，当时暂停；当前按执行队列持续推进。

- [ ] 完成[第16课](docs/lessons/16-host-schedule.md)双开关/原callback元信息、未知exit与真实回执、Shutdown排空和安全内部诊断练习；代理验证不代做学习。

- [ ] 完成[第17课](docs/lessons/17-cloud-push-owner.md)双类partial/Busy/关闭与无变化练习；all/dry见21/22、确认resolved维护见23；继续完整维护余项、全面Cloud兼容及部署解析，不把当前增量采用当完整M13。

- [ ] 完成[第18课](docs/lessons/18-health-check.md)心跳与saved双证据、clamp/caller取消、重复样本max语义及显式自检副作用练习；继续真实Cloud字段/权限、summary/resolve及部署解析，不能把本机参考验收写为M14/M15全部完成。

- [ ] 完成[第19课](docs/lessons/19-pending-statistics.md)完整指纹、同ms更新、Known/Busy/Unknown与构造/读取副作用练习；local噪音协调已见20，当前后续仅21/22，不自动push。

- [ ] 完成[第20课](docs/lessons/20-local-noise-discard.md)remote project/local scope、历史ACK/抑制/cursor、Unknown与cap冻结练习；本机替身不代替真实Cloud或用户学习。后续S-Cloud页面/其它monitor矩阵需另定范围，19/20当时收口；21/22已参考交付，当前按执行队列持续推进。

- [ ] 完成[21课](docs/lessons/21-push-preview.md)只读/Unknown/版本选择练习；显式All已见22，确认resolved维护已见23，完整维护余项仍待，不自动push。

- [ ] 完成[第22课](docs/lessons/22-explicit-all-replay.md)模式选集/抑制重叠、Pending与Remaining、最终持久化error及Tick不变练习；用户学习待完成。确认resolved维护见23；全面Cloud/目标发布解析仍待，当前持续推进。


- [ ] 完成[第23课](docs/lessons/23-confirmed-resolved-maintenance.md)精确ACK与resolved回收、RetentionDays启用语义和未确认保护练习，未代做用户学习。

- [ ] 完成[第24课](docs/lessons/24-migration-inspection.md)浅envelope/完整兼容、cursor非ACK与源保留练习；代理参考验证不代做用户学习。继续[当前执行队列](docs/execution-queue.md)的真实转换导入设计，不读取真实业务记录。

- [ ] 完成[第26课](docs/lessons/26-mcp-stdio-loop.md)精确ID/生命周期、structured回退与owned输入取消练习；代理测试不代做学习。七Cloud工具与Host显式采用已见27/28，继续执行队列，不把本地协议验证当真实MCP全兼容。

- [ ] 完成[27课](docs/lessons/27-cloud-mcp-tools.md)七工具/分页闭合/图片与状态幂等练习；28隔离Host入口已交付，真实MCP外部验证仍需对应授权。

- [ ] 完成[28课](docs/lessons/28-host-mcp-entry.md)专用环境变量、stdout隔离与stdin/SIGTERM所有权练习；继续29过滤/payload及monitor余项，真实MCP不能借健康自检授权自动调用。

- [ ] 完成[25课](docs/lessons/25-legacy-record-import.md)来源固定副本、完整typed转换、源集合/摘要漂移、新scope归属及部分续作练习；真实迁移/legacy Cloud字段验证仍需对应授权，后续按持续队列执行。

- [ ] 完成[29课](docs/lessons/29-filtered-http-payload.md)单unwrap/Join过滤、入口脱敏副本与HTTP/Builder同cap练习；30源码Reader与Host58见对应课，真实token配置不进入本地测试。

- [ ] 完成[第31课](docs/lessons/31-postgres-slow-query.md)raw/hash与展示SQL、标准值边界、sqlmock与真实PG差异练习；32已补私有真实PG两路径与Host默认关闭接线；生产TLS/权限、更多类型/所有池仍待。

- [ ] 完成[第30课](docs/lessons/30-source-context.md)原identity/辅助失败/根所有权与有限mask练习；目标源码布局、其它平台与部署权限仍待验证。

- [ ] 完成[33课](docs/lessons/33-crash-output.md)全局slot/Owner与Manager、raw隐私/预算和MayBePartial练习；34已实现显式单ID与`--all`批量导入，自动startup仍默认不启用。

- [ ] 完成[32课](docs/lessons/32-db-facade-observation.md)同pool/context、原Query与Tx、关闭只停观察、真实PG与部署差异练习；32参考已交付，继续34/35。

- [ ] 完成[35课](docs/lessons/35-cloud-summary.md)精确计数/nullable/限额与有限输出练习；Cloud Go元信息按[兼容方案](docs/plans/cloud-go-compatibility.md)已本地交付，发送端字段继续37，不借本地Summary授权真实MCP。

- [ ] 完成[34课](docs/lessons/34-crash-diagnostic-import.md)永久receipt/Unknown/预算/源漂移与构造副作用练习；手动单ID与显式`--all`已交付，自动startup导入默认不启用，P1与36按当前执行队列推进。

- [ ] 完成[第36课](docs/lessons/36-log-capture-policy.md)来源与级别策略练习；参考SDK/Host63已定向验证，37真实构建元信息已交付。

- [ ] 完成[第37课](docs/lessons/37-go-build-metadata.md)BuildInfo/unknown/replace练习；本地wire已参考交付，38继续独立发布解析验证。

- [ ] 完成[第38课](docs/lessons/38-release-reproducibility.md)本地解析/MVS/floor与远端发布区别练习；新P1快照按另report验证，旧两报告不改。

- [ ] 完成[第39课](docs/lessons/39-self-test-resolution.md)strict saved与best-effort解决、同token能力和独立结果练习；参考实现已交付，MySQL已见40，SQLServer本地观察已交付，继续真实服务与平台验证，不自动push。

- [ ] 完成[第40课](docs/lessons/40-mysql-slow-query.md)原Raw与有界展示、同池双入口/事务/关闭边界练习；MySQL参考已实现，真实实例仍待授权，41 SQLServer已交付，真实服务另验。

- [ ] 完成[第41课](docs/lessons/41-sqlserver-slow-query.md)替换callback/OUTPUT/编号预算及双入口练习；四已支持driver参考不等生产所有类型/池覆盖，未知OUTPUT表达式仍有有限mask盲区。
