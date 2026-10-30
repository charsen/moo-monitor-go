# TODOS

- [ ] 按 [复现计划](docs/port-plan.md)核对 PHP 每项采集来源、配置、缓冲格式、Cloud 与 MCP 契约，补齐对应 Go 行为设计。
- [ ] 接入 Goravel 前核实所选版本的异常、日志、SQL、队列和调度接入点，记录覆盖与盲区。
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
