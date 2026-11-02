# 35 Typed Cloud Summary

实现显式只读HTTP Summary，不建Scaffold页面/自动任务。固定POST api/v1/summary，body token+limit；Go limit1..10越界拒且零HTTP（源clamp差异）。复用交互post短timeout1..4s/连接级2次100ms、默认TLS/拒redirect/调用方响应cap。

响应只投影SummaryController现有project/stats/recent/generated_at，严格必需组/类型/非负精确整数/有限耗时/每族<=limit；未知新增字段忽略但不返回自由error。TodoID保string，nullable时间/url/icon沿源。Host62真实make命令moo:cloud:summary，默认5，只输出有限安全摘要，不自动heartbeat/sync，不用真实token。新测试只httptest+自有空env命令子进程，不全量/E2E/真实Cloud。

实际定向结果见下节；用户练习待完成。

## 实测

SDK首两项0.374s；补连接重试/timeout浅副本/caller取消后，新三项race1.521s。请求投影/精确整数和ID/nullable/坏响应/低cap/重定向零泄露均有定向断言。Host62真实make命令后，六模式CLI唯一race4.170s通过：默认5/显式1、非法limit/缺token/位置参数零请求、坏响应原CLI非0；只一次summary无intake/heartbeat、不建Buffer，白名单env+自有空-env/假token。未真实Cloud。

副作用范围：命令自身不解析Buffer；“无Buffer创建”为本次MONITOR_ENABLED=false隔离配置的断言。普通Host若启用其他owners，其bootstrap仍可能初始化本地资源，不能把该结果推广为整个应用无副作用。
