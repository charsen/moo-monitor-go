# 35 Typed Cloud Summary

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

实现显式只读HTTP Summary，不建Scaffold页面/自动任务。固定POST api/v1/summary，body token+limit；Go limit1..10越界拒且零HTTP（源clamp差异）。复用交互post短timeout1..4s/连接级2次100ms、默认TLS/拒redirect/调用方响应cap。

响应只投影SummaryController现有project/stats/recent/generated_at，严格必需组/类型/非负精确整数/有限耗时/每族<=limit；未知新增字段忽略但不返回自由error。TodoID保string，nullable时间/url/icon沿源。Host62真实make命令moo:cloud:summary，默认5，只输出有限安全摘要，不自动heartbeat/sync，不用真实token。新测试只httptest+自有空env命令子进程，不全量/E2E/真实Cloud。

实际定向结果见下节；用户练习待完成。

## 实测

SDK首两项0.374s；补连接重试/timeout浅副本/caller取消后，新三项race1.521s。请求投影/精确整数和ID/nullable/坏响应/低cap/重定向零泄露均有定向断言。Host62真实make命令后，六模式CLI唯一race4.170s通过：默认5/显式1、非法limit/缺token/位置参数零请求、坏响应原CLI非0；只一次summary无intake/heartbeat、不建Buffer，白名单env+自有空-env/假token。未真实Cloud。

副作用范围：命令自身不解析Buffer；“无Buffer创建”为本次MONITOR_ENABLED=false隔离配置的断言。普通Host若启用其他owners，其bootstrap仍可能初始化本地资源，不能把该结果推广为整个应用无副作用。

## 当前契约衔接（2026-11-03校准）

typed Summary命令已实现；S-Cloud一期本地Status/Preview/四动作不是完整首页云摘要，后者后置。Summary真实权限与生产响应尚未由本地严格fixture证明。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
