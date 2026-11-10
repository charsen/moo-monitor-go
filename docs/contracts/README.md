# Monitor 公共契约

本目录维护协议与测试证据；学习资料由 skeleton 统一保存。

- [显式单次 HTTP](explicit-once-http.md)：调用所有权与超时，不隐式重试未知结果。
- [GORM generation identity](gorm-generation-identity.md)：连接/插件身份与代际管理。
- [`capture/`](../../capture)、[`buffer/`](../../buffer)、[`cloudsync/`](../../cloudsync)：公开 Go API 与持久/同步状态。
- [`adapters/`](../../adapters)：Goravel/GORM 适配，与独立核心分离。
- [`fixtures/`](fixtures)：协议对照、固定假数据与历史发布证据。
- [`跨SDK脱敏样例`](fixtures/cross-sdk-redaction.json)：与其他Monitor SDK逐字一致的纯假数据；覆盖多词/转义引号密码、JWT和正常源码标识。仅共享测试输入与安全断言，不统一不同运行时的采集能力，也不替代历史PHP来源fixture。

记录、scope、游标与 ACK 隔离；严格逐条确认才推进水位。Host 提供采集配置、存储、身份快照及 Owner 启停。Scaffold 通过消费方窄接口使用状态和显式操作，由 Host 连接两个包；SDK 不反向依赖 scaffold 或 Host。

API、HTTP/Cloud/MCP wire、缓冲格式及资源关闭都属于兼容边界。公共变更核对实现、Host 调用、适配/绑定、测试与文档；本地协议替身不等于真实 Cloud/部署验证。

同步内核在主函数完成整批回执验证后，才进入 `applyValidatedReceipt` 的逐条 CAS/确认规则；该内部职责拆分不改变公开接口或持久格式。精确版本未成功应用时保持 pending，永久拒收仅隔离发送时的版本；游标重算与账本持久化仍由原主流程按批控制。
