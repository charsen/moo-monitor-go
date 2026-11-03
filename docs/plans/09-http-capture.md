# 第09步：HTTP请求生命周期、5xx观察与显式Recover委托

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

仅monitor，08验收后开发；不改Host、不替GinLogSafety、不注册或替换全局Recover，不接真实Cloud。

## 接口与输入

在adapters/goravel新增HTTP中间件，复用capture.Collector/EventScope和安全Diagnostic。HTTPConfig显式MaxEvents（默认64）、OnError、可选Recover(owner callback)及身份resolver；HTTP实现contracts/http.Middleware的Handle/Signature并提供有界Status。

每次Handle建立新scope，经ctx.WithContext把私有生命周期state传下游。默认只采method/url/ip，身份resolver同步给ID/Name/Guard标量并复制；不保存原request/Entry。SDK不主动读body、不取headers/cookies、不解析业务输入；ctx.Request懒初始化仍会按框架原机制处理body，不承诺整条框架链完全不解析。payload自动采集与过滤属于明确待接范围。现Builder继续做统一脱敏，不另建HTTP masker。

EventContext(ctx,id)为同一明确事件派生capture绑定；MarkError(ctx,id,error,frames)只标记调用方明确指定的终态错误供5xx关联，不自动Record。普通日志和错误没有同ID就不合并，同一请求多个错误各自保留。请求base context不直接绑定scope+空ID，避免把普通日志误判为不完整身份。最终5xx有明确标记则用原error/ID升级source=http_5xx；否则合成独立HTTPStatus事件，不能假装是源Throwable。panic使用新内部事件ID。

## 生命周期与Recover顺序

真实Gin内置recover位于用户middleware外层；不能先Close scope再声称外层Recover日志仍可共享身份。因此owner显式配置Recover时，Handle同一defer内处理当前goroutine panic，采集先行，任何采集故障仍调用原owner callback，观察其最终响应后Close。WrapRecover只包装owner显式给的callback，不读取或猜全局默认回调、不自动注册。

没有delegate时不接管panic，defer Close后原样repanic交框架；本SDK此模式只保证正常返回的5xx观察，不能声称已接管所有panic。owner callback自己panic不吞，scope仍defer关闭；已提交响应后panic按owner原行为处理。正常status/body一律不修改，采集/身份resolver/诊断失败只能安全OnError/Status旁路观察，不抢业务结果。

scope在Handle结束必Close，context不跨请求缓存；显式EventContext在结束后再用返回scope关闭错误，不把原请求带进后台。HTTP不拥有Collector/Buffer，不关借用store，无worker或自动flush。

## 真实依赖与验证范围

只读已核github.com/goravel/gin1.18.0与framework1.18匹配，最低go1.25.0/toolchain1.26.4；其gin-gonic1.12、timeout1.2.1、cors1.11.1、cast1.10、secure1.17及解析/QUIC模块进入真实图。08已提交a671955后，真实按批准版本get/tidy审MVS，不复制依赖replace、不另建submodule。项目仍最低Go1.26，本机实跑1.27.3。

定向真实内存Gin路由覆盖200/4xx/正常5xx、显式delegate panic/无delegate原框架、原status/body、同ID日志/5xx只tag与不同ID累加、scope关闭、collector/identity/observer失败、并发请求隔离与body未消费。只受影响HTTP adapter及必要原Handler回归，实际并发再定向race。未执行结果不填通过；没有Host启动、E2E、全量或真实服务。用户练习待完成。


## 最终实现与实际验证（2026-10-30）

实现见[第09课](../lessons/09-http-capture.md)。HTTP.Handle顺序可直接读到scope/context→有限快照→Next→正常5xx或显式Recover→必清理；公开WrapRecover只委托原owner，不自动注册。Handler消费私有已复制快照，闭合时不读；正常首事件不再丢HTTP Request。EventContext/MarkError严格显式ID，内部前缀保留；没有终态mark才合成HTTPStatus。无delegate模式原样repanic，不能声称SDK已接管。

真实get/tidy后图固定framework1.18.0/gin1.18.0/gin-gonic1.12.0；按设计明确固定cpuid2.4.0对齐已审生态基线，不作漏洞可利用性判断。x/sys0.49.0、yaml3.0.5、testify1.12.1保留，根无replace，项目go1.26.0不变。Gin引入的解析/QUIC、框架配置/telemetry模块如实进入go.mod/sum，测试未启动telemetry/DB/Host。没有追latest或扩大安全扫描。

首轮6个HTTP项普通定向PASS1.583s；补Handler HTTP快照消费、owner自身panic/采集失败仍执行与已提交响应后panic后，最终13顶层项（8 HTTP+5必要Writer）race PASS3.399s。命令在monitor根执行，go使用隔离1.27.3：

```sh
GOWORK=off GOTOOLCHAIN=local go test -mod=readonly -race ./adapters/goravel -run '^(TestHTTPLifecycle|TestHTTPExplicitEvent|TestHTTPRecoverDelegate|TestHTTPNoDelegateFrameworkRecover|TestHTTPFailureIsolation|TestHTTPConcurrentRequests|TestHTTPDelegatePanicAndCaptureFailure|TestHTTPCommittedResponsePanic|TestWriterCaptureAndDelegate|TestWriterEventIdentity|TestWriterFailuresAndReentry|TestWriterCancellationAndConcurrentEvents|TestWriterSafeDiagnosticBoundary)$' -count=1 -timeout=90s
```

实际路由为goravel/gin.NewRoute+SetGlobalMiddleware，本机httptest Recorder直接ServeHTTP，无监听器/真实外联。覆盖200/422/503原body、handler完整读固定JSON body、身份复制/query脱敏、同ID来源升级/不同ID累计、原默认recover、显式delegate只采一次、scope结束关闭、旁路失败不改响应、6并发请求隔离。已提交202后panic由owner追加body且原202保留，SDK不伪改状态。用户练习仍待完成；未跑全量/E2E/真实Cloud/Host，未自动payload/queue/schedule/slowSQL。
