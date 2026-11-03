# 第10步：Host本地运行时接入与late identity修正

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

主交付位于[Host第41步计划](../../../go-moo-engine-skeleton/docs/plans/41-monitor-runtime.md)与[详课](../../../go-moo-engine-skeleton/docs/lessons/41-monitor-runtime.md)。Host代码由其Provider拥有，SDK不import Host/scaffold。

本仓必要公共修正：HTTPConfig.Identity改接context.Context，HTTP生命周期只保resolver函数与基础标量，不捕获原HTTP Context。身份在首条日志/最终observe时同步读取当前context并复制ID/Name/Guard；resolver error/panic仍保基础request且安全诊断，不再次递归resolver。匿名不伪造。实际Host路由认证在全局HTTP middleware之后，这正是09 eager身份无法满足的新消费者事实，09当时已提交测试证据不改写为新行为。

SDK自身GOWORK=off，隔离Go1.27.3：14项受影响HTTP/Writer定向race实际PASS3.072s，包括late身份已在首日志存在、最终5xx只tag，身份resolver在beforeNext零调用、匿名，以及原响应/owner/诊断/并发边界。Host workspace证据独立见41计划，不能互相替代。

没有新增SDK依赖、Cloud或全局Recover；Host默认关闭且本地启用不联网。SDK部署解析、请求自动payload、queue/schedule/slowSQL与真实Cloud兼容继续待完成。用户练习待完成。
