# 第09课：HTTP生命周期、明确事件关联与Recover委托

[上一课：08](08-runtime-capture.md) · [课程目录](../README.md) · [下一课：10](10-host-runtime.md)

08能显式Capture及接收日志，但没有请求生命周期。本课在monitor内提供Goravel HTTP中间件，以真实内存Gin路由验证scope、请求快照、5xx和panic委托。没有修改Host、注册全局Recover或连接Cloud；[计划](../plans/09-http-capture.md)列出接口和范围。

## 从一次请求读完整流程

[HTTP.Handle](../../adapters/goravel/http.go)先创建有界EventScope，把私有生命周期state放进下游context，然后复制有限请求标量，执行Next。正常返回后读取Response.Origin().Status，只有>=500才观察。整个动作的defer最后清除标记错误与请求快照、关闭scope；显式Recover和普通返回使用同一清理边界。

| 文件 | 本课职责 |
| --- | --- |
| [http.go](../../adapters/goravel/http.go) | NewHTTP/Handle、EventContext/MarkError、显式Recover与有限请求快照 |
| [logger.go](../../adapters/goravel/logger.go) | 消费已复制的HTTP快照，使首次日志落盘有真实请求信息 |
| [http_test.go](../../adapters/goravel/http_test.go) | 真实Goravel/Gin内存路由、原响应、关联与生命周期边界 |
| [go.mod](../../go.mod) | 固定Gin与实际必要模块图 |

Builder脱敏、hash与来源优先级、Buffer累计和intent不复制。中间件借用Collector，Collector借用Buffer，退出请求只关闭本次scope，不关闭共享store。

## 构造与owner接线

```go
observer, err := goraveladapter.NewHTTP(collector, goraveladapter.HTTPConfig{
    MaxEvents: 64,
    Recover: ownerRecover,
    OnError: observeSafeDiagnostic,
})
if err != nil { return err }
// owner把observer作为普通Goravel Middleware注册。
// 这里只配置对象，不注册全局Recover，也不执行Cloud请求。
```

示例中的collector、ownerRecover和诊断函数由owner提供。MaxEvents默认64，允许1..4096，满时沿用08明确错误，不淘汰旧ID后再次计数。HTTP实现Signature/Handle，Status返回有界安全诊断。

本课测试使用真实goravel/gin.NewRoute及SetGlobalMiddleware，仅装本课middleware并保留框架内置recover；不启动监听端口或Host容器。替换测试路由的middleware集合不代表生产Host应清空原中间件。未来Host接线必须保认证、原响应和GinLogSafety，不能照抄测试装配。

## 请求快照只取哪些值

默认只取Method、FullUrl和Ip；可选Identity resolver同步返回ID/Name/Guard字符串指针，SDK立即复制值。身份必须来自owner可信认证边界，不能由本课从业务body或任意header猜测。IP沿用Goravel/Gin解析结果，可信代理策略由Host配置；不是授权依据。

SDK不主动读取、关闭或解析body，不枚举headers/cookies，不抓User-Agent，不保存原Request对象。需要精确区分：ctx.Request懒初始化仍可能触发Gin内部getHttpBody，框架会继续按原机制处理输入；不能承诺“整条框架链完全不解析body”。本课JSON测试在真实handler完整读到原body，这是当前样本证据，不泛化为所有表单/上传流完全等价。

URL与身份进入既有Builder脱敏；输入URL含token查询的测试保存为***。body即使含token也不会自动进入Payload；正常5xx记录为空payload。自动payload提取、业务字段过滤和上传策略仍待后续设计，不能把“不自动采集body”称为已完整复刻PHP请求all。

resolver返回error时基础method/url/ip仍保留；解析或resolver自身panic时按采集失败诊断，不承诺该次快照完整。两者都不改响应。08 Handler现在在锁内复制私有state中的请求标量，结束后不读取。因此首次错误由日志Record时已有HTTP Request，末尾只TagSource也不会留下CLI信息；核心capture仍无框架import。

## 同一请求不等于同一错误

HTTP没有把request ID当所有error的身份。需要跨来源关联时，调用方明确派生EventContext，并明确MarkError指定终态错误：

```go
eventCtx, err := observer.EventContext(ctx.Context(), "terminal-1")
if err != nil { return err }
// 用eventCtx在08日志Handler中记录明确exception与frames。
// 同一生命周期还可以用另一个ID记录第二个真实错误。
if err := observer.MarkError(ctx.Context(), "terminal-1", event, frames); err != nil {
    return err
}
```

这段是错误关联步骤，不是完整Controller；真实业务不得因为旁路标记失败改写原HTTP响应，应交安全诊断边界处理。MarkError只保留显式终态关联，不执行Record。EventContext只绑定调用方指定事件，base context没有scope+空ID，普通日志无需事件关联也能正常采集。

若最终5xx有MarkError，复用同error/ID并以http_5xx采集：首次已有log_context会仅升级来源，不增加count。不同ID即使hash相同仍正常累加。实际测试先两个ID落同hash，再把其中一个标记终态，最终count=2而非3。

没有明确MarkError的5xx合成HTTPStatus事件，使用固定合成位置moo-monitor/http，不从响应body拼错误；它不是源PHP的真实Throwable。内部ID使用保留前缀__moo_http:，公开EventContext/MarkError拒该前缀、空ID、超128字节或非法UTF8。不要按message猜同一个error，也不要把多个真实错误全部覆盖成请求级一条。

Handle结束清除mark并Close。保存派生context在请求结束后再采，scope会明确报关闭；不能把它携带原请求身份进入异步后台。独立任务应创建自己的生命周期scope和明确快照。

## Recover为何必须由owner明确提供

真实[PHP Provider](../../../moo-monitor-laravel/src/MooeenMonitorLaravelServiceProvider.php)的renderable观察HttpException>=500，返回null让原渲染继续。Go普通500响应可能完全没有error对象，所以本课明确区分状态合成事件与panic/error。

真实Gin的内置recover位于用户middleware外层，Route.Recover又是单一全局替换且没有公开getter。不能先退出内层scope，再宣称外层回调日志仍可正常共享事件身份；也不能猜原callback后擅自覆盖。SDK不调用Route.Recover。

配置了原始owner Recover时，Handle的defer捕获当前goroutine panic：建立新内部事件ID，Collector采集一次，随后始终执行owner callback。回调日志临时使用同一panic身份；回调后若实际响应仍>=500，仅升级http_5xx。最后清理scope。panic是新事件，不复用此前MarkError的业务错误ID。

采集失败仍调用owner；owner callback自己panic不吞，最后Close仍执行，再由框架原recover处理。已经提交的响应遵循owner原行为：测试先提交202和前缀，再panic，owner尝试502追加后缀，实际仍202且body为原前缀+owner后缀。SDK不强行改成500，也不会因已捕获panic伪造最终5xx。

没有配置Recover时，SDK原样repanic交框架、关闭scope，不接管原默认日志/500响应。本课实际验证框架原recover得到原panic，返回500空body且本SDK未记录该panic；若owner另有日志适配，其日志仍按原配置处理，不能将本模式说成SDK已完整覆盖panic。

WrapRecover是显式委托包装，不注册全局。组合模式在HTTPConfig.Recover只传原始owner callback，Handle内部包装一次；不要再传已经WrapRecover的结果造成双重采集。独立使用WrapRecover时，生命周期由owner负责，不能期待不存在的HTTP.Handle替它创建/关闭scope或自动观察最终响应。

## 故障、诊断与所有权

HTTP复用08安全Diagnostic：只静态错误类别，不保存resolver/Recorder错误原文、payload或panic文本；OnError自己panic留下ObserverFailed。回调要使用传入internal ctx，不能另起Background日志再宣称防回环有效。

Collector错误、resolver错误、诊断panic均不改变原status/body。显式EventContext/MarkError调用会返回错误供调用方观察，业务接线必须维持旁路原则。SDK不自动网络flush、不接Cloud、不替Host全局Recover或35原日志安全层；queue/schedule/慢SQL与自动payload仍是后续块。

## 依赖与验证证据

本课固定goravel/gin1.18.0，与framework1.18.0对齐；真实图含gin-gonic1.12、timeout1.2.1、解析/QUIC与框架配置/telemetry相关模块。导入这些包用于真实内存路由，不代表启动telemetry或数据库。核心capture不import Gin。

cpuid2.4.0明确对齐31已审生态兼容基线，不宣称2.3在本路径存在可利用漏洞；x/sys0.49、yaml3.0.5、testify1.12.1与项目Go1.26保持，实际执行Go1.27.3。无依赖replace复制，不改Host workspace，未实跑Go1.26或干净发布解析。

首轮六项HTTP实际PASS1.583s。最终结果在计划和下方记录补齐；仅本课HTTP和必要08 Writer定向验证，无全量、E2E、Host或真实Cloud。

## 自主练习（待用户完成）

1. 画出Next正常返回、显式Recover、无delegate三条scope关闭顺序，解释不能随意替全局Recover。
2. 预测两个ID同hash再标记一个终态5xx的count；解释无MarkError合成事件为何可能另计。
3. 复述body、身份resolver与IP代理的边界，指出仍未实现的payload能力。
4. 解释已提交202再panic时为何不能由监控改成500，以及采集失败应由谁观察。

代理测试不代做学习验收。


最终实际验证（2026-10-30）：补强后仅本课8项HTTP及必要5项Writer执行定向race，PASS3.399s。命令与实际覆盖见[计划最终记录](../plans/09-http-capture.md)。此前6项HTTP普通测试1.583s是早轮证据。所有记录使用私有临时Buffer和真实内存路由，没有监听外部服务或连接Cloud。

后续演进：第10步实际Host认证消费者要求Identity(context.Context)在事件时刻解析，09首次实现的eager身份模型已被替换；原始测试记录保留，当前API/接线见[第10课](10-host-runtime.md)。
