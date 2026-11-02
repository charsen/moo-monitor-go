# 第35课：读取有边界的Cloud摘要

[上一课：33](33-crash-output.md) · [课程目录](../README.md)

[计划](../plans/35-cloud-summary.md)交付SDK显式Summary接口，不创建管理页面、后台拉取任务或新Cloud字段。

## 从请求到typed结果

HTTPClient.Summary(ctx,limit)先检查1..10范围，非法输入零请求。它POST固定api/v1/summary，body只有token与limit；源Cloud SummaryController使用runtimes能力认证，并对limit作clamp。Go明确拒越界，不悄悄替调用者选择范围。

单次交互使用client浅副本，把timeout限制在1..4秒，两次连接级尝试间隔100ms；调用者更短的ctx仍有效。原client配置/transport所有权不变，默认TLS、拒重定向和调用者MaxResponseBytes全部保留；没有自动push/heartbeat。

summary.go只返回Project、Stats、Recent和GeneratedAt。统计使用int64，Todo ID始终string，避免经float64丢精度。每个必需对象/键必须存在；只有源migration/Clip允许的字段可null，例如project env/url/icon、异常摘要、慢SQL位置、时间。每族recent条数不超过本次limit，hash/状态/优先级及非负数值验证后才返回。

未知新增字段可被忽略，但不会成为动态map传给业务；坏JSON、重复键、错误类型、坏UTF8或超限响应返回静态错误，不泄漏URL/token/Cloud自由error。摘要里的普通描述仍是远端业务内容，此接口不是任意秘密全内容净化器。Host命令应有限输出，不能把任意对象完整dump成诊断。

## 证据与练习

新两项普通0.374s，新三项race1.521s，涵盖真实httptest POST投影、nullable、超过2^53的统计与字符串TodoID、畸形/重复键/负数、越界零请求、redirect第二host零请求、低响应预算、连接重试、交互timeout不改共享client及caller取消。只有本地假服务器；没有真实token、真实Cloud或生产权限/TLS部署证明。

自主练习待完成：说明int64与Todo string的不同用途；列出哪个null来自源schema；比较源clamp与Go拒绝；解释浅副本为何不应Close借用transport；设计只展示计数和有限ID的命令输出而不打印整响应。

Host62真实命令六模式race4.170s通过（默认/显式limit、非法输入、缺token、坏响应、位置参数）。显式命令在自动Cloud关闭时仍可执行，只发summary，不建本地Buffer。所有子进程仅自有空-env与假服务器。

副作用范围：命令自身不解析Buffer；“无Buffer创建”为本次MONITOR_ENABLED=false隔离配置的断言。普通Host若启用其他owners，其bootstrap仍可能初始化本地资源，不能把该结果推广为整个应用无副作用。
