# TODOS

- [ ] 按 [复现计划](docs/port-plan.md)核对 PHP 每项采集来源、配置、缓冲格式、Cloud 与 MCP 契约，补齐对应 Go 行为设计。
- [ ] 接入 Goravel 前核实所选版本的异常、日志、SQL、队列和调度接入点，记录覆盖与盲区。
- [ ] 核对 Cloud 心跳字段白名单、SDK 版本诊断和 runtime/慢 SQL 数据格式对 Go 的兼容缺口；先制定增量兼容方案，不伪装成 PHP SDK。

- [ ] 在首个实现阶段落实[代码设计标准](../go-moo-engine-skeleton/docs/code-design.md)：审查参考业务调用链、文件职责、抽象必要性及生成/手写协作；文档制定不等于代码验收。

- [ ] 完成[第01课](docs/lessons/01-sensitive-masker.md)固定假值预测、三入口范围与初始化错误隔离复述；代理参考验证不代做用户练习。
- [ ] 继续M07的SQL值侧/递归payload与有界截断，分别核源fixture；Host坏JSON原日志预防与M18接入另行设计，基础Masker不等于全面脱敏。
