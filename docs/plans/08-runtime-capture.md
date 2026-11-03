# 第08步：显式runtime采集与Goravel日志适配

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

仅Monitor。先落实Capture(error)/LogMessage→records.Builder/Buffer，及真实Goravel公开Logger/Handler/Writer消费；不改Host，不替Recover/GinLogSafety，不自动Cloud/Flush网络，panic/HTTP500/queue/schedule另块。

## 去重与资源API

capture.Collector消费现Buffer，Config显式env/project/go/goravel/Clock；Options携带Source、显式request/payload/meta/frames、EventScope/EventID。error只读取Error()、明确类型名和有限unwrap previous，不读任意源码文件；日志无error对象只记LoggedErrorMessage/log_message，不从字符串伪造Throwable。所有mask/预算交Builder。

EventScope由调用方按请求/任务生命周期创建，容量有界，不map[error]或字符串message猜身份。每scope可取消gate串行同ID，首成功缓存hash/规范source rank，高rank只原子TagSource，低rank不改；不同ID同hash正常累加。满容量明确error，不淘汰再双计；Close清空/禁止复用。无scope/ID时不声称跨来源自动去重。

普通确定失败可重试同ID；ErrOutcomeUnknown保uncertain tombstone，重复ID仍返回未知而不盲目Record。跨scope/Close后无持久EventID去重，不声称exactly-once。

Tag路径复用records.NormalizeSource和Builder.TagMetadata/rank/runtimeMeta；Buffer.TagSource在scope锁内恢复/读取，先判断metadata实质无变化不写，再daily cap冻结不写，只改meta/updated_at且保count/daily/last_seen。复用05固定最终图像intent，目标明确三已知桶，清其它桶排除target；不是恢复已移除的本地生命周期动作。

## 日志旁路与失败边界

可选adapters/goravel同module固定framework1.18.0，核心capture不import框架/scaffold。Handler先传原delegate，捕获只error及以上；delegate自身error原样返回，监控失败只安全OnError，不抢原业务结果。框架Writer会忽略Handle返回error，必须说明并提供可观测诊断。

在Handle内只读白名单level/message/time/code及显式scope/EventID/frames上下文值，不遍历任意With对象、Request/Response/User或保留Entry/请求对象。context内部标记防本次采集诊断回环，不全局忙就丢其它并发事件。OnError本身panic也隔离并留下有界可查询失败状态，不把实现bug静默消失。

依赖批准前已核framework go.mod=go1.25.0/toolchain1.26.4；真实Writer/log contracts带HTTP/ORM/Carbon/eris/slog-multi/OTel/gRPC等module图。只固定根require framework1.18.0，不抄依赖replace、不造submodule。既有x/sys0.49/yaml3.0.5/testify1.12.1应MVS保留；get/tidy后以实际图回读。测试只NewWriter+HandlerToSlogHandler，不bootstrap app/Provider/DB/telemetry。

## 验证安排

新增capture与adapter定向、受影响records/Buffer tag三桶与intent关键阶段：同ID跨来源首计数/高rank只升级、不同ID累加，cap/no-change不刷updated_at、容量/Close/取消/unknown禁止重复计数；显式error有限unwrap与UTF8按Builder；真实Writer→custom Handler→自有Buffer、Info不采/Error采/delegate保留、采集失败安全OnError/重入/诊断panic可观测、关闭资源。实际并发才定向race，不全量/E2E/Host/真实服务。用户练习待完成，实际结果见下方。


## 最终实现边界

API已落capture.New/Collector.Capture/LogMessage、NewEventScope/WithEvent/Close及Buffer.TagSource。Recorder是Record/TagSource的真实持久化消费边界；Collector不关闭借用Buffer。新增records.NormalizeSource/TagMetadata供实际消费者共享，没有框架级pipeline。

自动诊断只保静态ErrCapture及有限已知错误类别，不长期保存公开替换Recorder的任意cause；显式Capture仍原样返回原因。OnError必须用传入internal ctx记录诊断，忽略ctx另起日志不在回环保证内。回调panic由ObserverFailed可查，原delegate行为不被采集失败替换。

实际go get/tidy固定framework1.18.0；新增Carbon/eris/slog-multi、HTTP/ORM/OTel/gRPC等真实框架图，核心capture没有框架import，不复制依赖replace、不另建submodule。既有x/sys0.49.0、yaml3.0.5与testify1.12.1保留；Go下限仍1.26.0，测试实际1.27.3。没有bootstrap/DB/telemetry运行或Host依赖接线。

## 实际验证（2026-10-30）

隔离Go1.27.3、GOWORK=off、GOTOOLCHAIN=local，使用`-mod=readonly`。恢复本步后重新取得Writer五项可信输出0.807s；未凭丢失的上轮输出标通过。

最终受影响定向命令（在monitor根执行；go指向上述隔离工具链）：

```sh
GOWORK=off GOTOOLCHAIN=local go test -mod=readonly -race ./records ./buffer ./capture ./adapters/goravel -run '^(TestRuntimeSourceFixture|TestTagSourceBuckets|TestTagRecovery|TestBufferRecovery|TestCaptureScope|TestCaptureUnknown|TestCaptureErrorExtraction|TestCaptureCancellation|TestWriterCaptureAndDelegate|TestWriterEventIdentity|TestWriterFailuresAndReentry|TestWriterCancellationAndConcurrentEvents|TestWriterSafeDiagnosticBoundary)$' -count=1 -timeout=90s
```

实际通过：records1.435s、buffer4.392s、capture2.021s、adapter1.900s。包括三桶tag/关键intent阶段恢复、scope容量/关闭/取消/unknown、有限unwrap、真实Writer delegate/Info/Error、6并发事件及安全诊断。提交结果未知包装与实际Buffer故障恢复是两类证据；没有设备断电、全量测试、E2E、Host或真实Cloud验证。


最终journal补强：恢复要求Record.Status与intent.Target严格相等。新定向TestTagIntentStatusMismatch验证合法但不一致的状态/目标仍拒绝，原intent、记录及overflow保留。首次新增测试有`**Record`编译误用，修正后相等守卫暴露旧seedBucket刻意保留raw status=open的六个tag恢复失败；TagSource现先按真实桶设置最终Status，再写intent。桶仍是真值，不把破损intent当迁移机会。

补强后仅执行buffer四项：TestTagIntentStatusMismatch、TestTagSourceBuckets、TestTagRecovery、TestBufferRecovery，实际PASS2.817s（2026-10-30）。此前race是此补强前证据，未重新执行Writer或race。
