# 第08课：显式错误采集、事件身份与Goravel日志旁路

[上一课：07](07-cloud-http-client.md) · [课程目录](../README.md)

本课把显式error和真实Goravel日志送入已有Builder与Buffer。采集结果落本地，构造和Capture都不自动推送Cloud；07的HTTP传输与06的Sync仍由owner显式调用。范围见[计划](../plans/08-runtime-capture.md)。

## 从调用到落盘

调用链是Collector.Capture → 读取有限错误信息/明确上下文 → Snapshot → Buffer.Record → Builder.Build或Refresh → 已有intent提交。日志链是框架Writer → HandlerToSlogHandler → 本课Handler → 原delegate → Collector → Buffer。这里实际消费公开接口，未启动Goravel应用、Provider、数据库或telemetry。

| 文件 | 职责 |
| --- | --- |
| [capture.go](../../capture/capture.go) | 显式error或字符串日志转换、有限previous与caller位置、事件去重编排 |
| [scope.go](../../capture/scope.go) | 有界生命周期身份、可取消gate、内部context标记 |
| [tag.go](../../buffer/tag.go) | 锁内来源元信息升级，保留累计与桶状态 |
| [records.go](../../records/records.go) | NormalizeSource、来源优先级和实际共享TagMetadata |
| [recovery.go](../../buffer/recovery.go) | 三个已知目标桶的最终图像恢复 |
| [logger.go](../../adapters/goravel/logger.go) | 可选框架Logger/Handler、白名单转换和安全诊断 |

核心capture不import框架或scaffold；可选adapter与核心同module，固定framework1.18.0。安装整个module仍会解析框架依赖，不能把“可选import”说成独立无依赖发布。go.mod保留项目Go1.26.0，本机实跑1.27.3，未实跑1.26。依赖方replace没有复制进本仓。

## 显式Capture的输入和边界

New消费窄Recorder接口：Record与TagSource，正常传真实*buffer.Buffer。Config显式给Env、Project、GoVersion、GoravelVersion和Clock；空GoVersion使用runtime.Version，空Clock用time.Now。Collector借用store，不拥有关闭权。测试替换Recorder只用于模拟“已提交但返回unknown”，不是另一套持久化实现。

```go
collector, err := capture.New(store, capture.Config{
    Env: "testing", Project: "example", GoravelVersion: "1.18.0",
})
if err != nil { return err }
scope, err := capture.NewEventScope(64)
if err != nil { return err }
defer scope.Close()
_, err = collector.Capture(ctx, businessError, capture.Options{
    Scope: scope, EventID: "task-event-1", Source: "reportable",
})
return err
```

例子依赖调用方已有的store、ctx和businessError，不创建服务或读取环境。Request、Payload、Meta与Frames由调用方明确提供；空Request.Method按CLI处理，payload方法白名单及脱敏继续由Builder统一执行。不能传入原请求对象期望SDK自动遍历。

error只读取Error、明确类型名和Unwrap链；最多三个previous，单错误和errors.Join均有界，循环链也不会无限走。类型名来自reflect.TypeOf，不反射任意字段；不会打开源码文件。Frames最多20个，未提供时只读runtime caller位置；不假装这是错误最初发生位置。Snippet未采集。

无error对象的LogMessage使用LoggedErrorMessage/log_message，字符串不会变成伪Throwable。显式Capture提取panic返回ErrExtraction；若已开始可能写入，则附结果未知类别，不把panic原值带入诊断。非法UTF8保留既有Builder/存储字节语义，07发送前仍可能ErrWire，落盘通过不等于JSON可发送。

## EventScope不是全局错误缓存

PHP Dispatcher使用Throwable对象身份；Go error可能不可比较，日志又常只有字符串。因此调用者显式给同一生命周期EventScope与稳定EventID。ID是有效UTF8的1..128字节，scope容量1..4096，满时ErrScopeFull，不淘汰旧ID重新计数。

同ID首成功计数；后来更高SourceRank只调用TagSource，低rank不改。不同ID即便得到相同hash，仍是两个事件，正常累加。没有共同ID的字符串日志与显式Capture不能自动关联；不同scope或Close后的新scope也没有持久exactly-once保证。不要用message推测身份。

scope的gate可按ctx取消，Close等待正在处理者、清空状态并禁止再用；owner应在请求或任务结束调用。普通确定失败移除占位，可以重试；ErrOutcomeUnknown保uncertain tombstone，重复ID返回ErrEventUncertain而不盲目Record，避免“已提交但回执丢失”再次加count。unknown包装测试只证明该保护，实际文件恢复由Buffer故障注入测试证明。

## 来源升级为何不重新Record

Buffer.TagSource先加锁并恢复intent，读真实桶，然后调用Builder.TagMetadata。元信息无实质变化先返回，不刷updated_at；当天daily达到cap也不写。真正变化才写meta.updated_at，count、daily、last_seen及原业务字段不变，overflow保留。

目标可以是open/resolved/deleted，升级不重开状态。intent恢复写目标并删除其它桶时必须排除target；没有新增本地Resolve/Delete生命周期接口。三个桶和tag-intent、tag-target、tag-old-removed阶段都有实际恢复断言。

一个恢复例子：目标tag文件已写但旧副本未清就失败。调用返回结果未知；下一次锁内恢复重放相同最终Record，清其它桶、保overflow并删intent，不再执行Record累计。这证明进程重新打开后的图像恢复，不是设备断电耐久测试。

## 日志先传原delegate，再旁路采集

NewDriver返回满足框架Logger的Driver，其Handle给出真正Handler。Handler.Enabled保留delegate所需等级并启用Error及以上；Handle先让原delegate处理，再仅采Error及以上。Info不会落监控记录。SDK不会替原日志做脱敏或改写；原delegate看到原消息，因此Host仍必须落实原日志敏感信息策略。

适配器只读取level/message/code/domain及明确With键：moo_monitor_event_id、moo_monitor_exception、moo_monitor_frames。scope来自capture.WithEvent的context；With的ID仍需要配套scope。其它With对象、Request/Response/User不遍历、不序列化；Handler返回前复制明确frames，不保存池化Entry或请求对象。error对象键才走log_context，否则是log_message。

框架Writer真实实现会忽略Handler返回error：本课提供OnError及可查询Status，不能声称Writer会把delegate失败返回业务调用者。直接Handler调用仍原样返回delegate error；采集失败不覆盖它、不改变HTTP业务结果。显式Collector.Capture则返回原错误原因，由调用方处理。

## 诊断也要防止泄露与回环

公开Recorder可能返回含payload的任意error。自动旁路的Diagnostic.Failure只保静态ErrCapture及有限已知类别，不保存任意cause或调用其Error文本；Status只累计失败数和最后一个安全Diagnostic，不长期保存Entry/native error。测试用固定假token错误核对OnError与Status文本均不含该串；显式Capture仍保留原cause。

OnError收到内部标记ctx，使用该ctx记录诊断时原delegate仍执行，但监控跳过回采。回调必须传递此ctx；丢掉它另起Background日志属于调用方违约，不能声称全局递归万能防护。标记只影响本次调用，不以全局busy丢其它并发事件。OnError自己panic被隔离并留下ObserverFailed，panic文本不保存；owner应查询Status，不能只依赖同一个失败回调。

## 资源、验证与剩余覆盖

owner负责EventScope.Close、Buffer.Close，以及未来显式Syncer.Close/HTTPClient空闲连接退出。Collector/Driver不接管借用资源，没有后台flush，也没有新全局Recover。真实panic/HTTP500/queue/schedule、Host接线、完整过滤策略和Cloud字段兼容仍未完成；不能把本课称为M02/M18全部交付。

验证结果见下方最终记录。全部使用自有t.TempDir、固定假消息和真实Goravel公开Writer；没有Host启动、真实Cloud、E2E或全量测试。

## 自主练习（待用户完成）

1. 预测同ID reportable→log_context与不同ID同hash各自count变化，解释为何不能按字符串去重。
2. 画出已提交但unknown后的scope状态，说明关闭scope后为何不能承诺exactly-once。
3. 比较显式Capture error、delegate error和安全Diagnostic，指出各自由谁观察。
4. 用“tag目标写完、旧桶未清”的例子说明恢复为何不增加count；解释daily cap与无变化为何不能刷时间。

代理测试通过不代表这些练习完成。

## 实际验证（2026-10-30）

隔离Go1.27.3、GOWORK=off、GOTOOLCHAIN=local，使用`-mod=readonly`。恢复本步后重新取得Writer五项可信输出0.807s；未凭丢失的上轮输出标通过。

最终受影响定向命令（在monitor根执行；go指向上述隔离工具链）：

```sh
GOWORK=off GOTOOLCHAIN=local go test -mod=readonly -race ./records ./buffer ./capture ./adapters/goravel -run '^(TestRuntimeSourceFixture|TestTagSourceBuckets|TestTagRecovery|TestBufferRecovery|TestCaptureScope|TestCaptureUnknown|TestCaptureErrorExtraction|TestCaptureCancellation|TestWriterCaptureAndDelegate|TestWriterEventIdentity|TestWriterFailuresAndReentry|TestWriterCancellationAndConcurrentEvents|TestWriterSafeDiagnosticBoundary)$' -count=1 -timeout=90s
```

实际通过：records1.435s、buffer4.392s、capture2.021s、adapter1.900s。包括三桶tag/关键intent阶段恢复、scope容量/关闭/取消/unknown、有限unwrap、真实Writer delegate/Info/Error、6并发事件及安全诊断。提交结果未知包装与实际Buffer故障恢复是两类证据；没有设备断电、全量测试、E2E、Host或真实Cloud验证。


最终journal补强：恢复要求Record.Status与intent.Target严格相等。新定向TestTagIntentStatusMismatch验证合法但不一致的状态/目标仍拒绝，原intent、记录及overflow保留。首次新增测试有`**Record`编译误用，修正后相等守卫暴露旧seedBucket刻意保留raw status=open的六个tag恢复失败；TagSource现先按真实桶设置最终Status，再写intent。桶仍是真值，不把破损intent当迁移机会。

补强后仅执行buffer四项：TestTagIntentStatusMismatch、TestTagSourceBuckets、TestTagRecovery、TestBufferRecovery，实际PASS2.817s（2026-10-30）。此前race是此补强前证据，未重新执行Writer或race。
