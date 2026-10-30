# 第14课：队列最终失败与调度完成

[上一课：13](13-host-slow-query.md) · [课程目录](../README.md)

本课使用真实 Goravel Worker、成熟 Database Driver 和原 cron/Artisan，接入队列最终失败与调度结果；不是每次 Handle 失败就上报。只显式 owner 构造会启用，Host 默认 Worker/全局 Artisan 尚未替换。[计划](../plans/14-queue-schedule.md)列明本轮收尾与暂停恢复入口。

## 先找到真正的最终失败

PHP源 `MooeenMonitorLaravelServiceProvider.php:185..197` 监听 JobFailed，带 connection/queue/job_name/attempts，再交统一Dispatcher；同Throwable可升级queue_failed来源而不重复计数。调度Finished/BackgroundFinished只把非0整数退出码合成ScheduledTaskExit，Laravel12后续精确普通Exception才关联抑制；本包cloud命令的合成退出排除，真实代码异常仍应可见。本课只读这些源码/既有源测试，没有运行PHP Host或新增PHP fixture。

Goravel1.18 `queue/worker.go:call` 在每次Handle错误后决策重试，末次才序列化Task并写failedJobChan。Job.Handle没有ctx/UUID；失败数据库中只剩err.Error字符串，不能从日志还原原error。公开JobWithShouldRetry却正好接收本次原error和attempt，因此采用已完成任务的策略装饰，不改成熟worker。

| 文件 | 职责 |
| --- | --- |
| [queue.go](../../adapters/goravel/queue.go) | 显式owner、单后端实例、Pop/Receive分型、reservation/末次策略 |
| [schedule.go](../../adapters/goravel/schedule.go) | Schedule专用Artisan、callback与显式完成回执 |
| [completion.go](../../adapters/goravel/completion.go) | 两入口共用的派生timeout与安全诊断 |
| [queue_test.go](../../adapters/goravel/queue_test.go) | 真实Worker配受控内存Driver、重试/chain/并发与失败 |
| [queue_database_test.go](../../adapters/goravel/queue_database_test.go) | 成熟Database Driver配自有SQLite，原删除/失败持久化 |
| [schedule_test.go](../../adapters/goravel/schedule_test.go) | 原Artisan返回、真实cron恢复、显式code/身份与错误边界 |

## 同一个重试配置，原 Worker 仍执行任务

NewQueueWorker接调用方原Config/cache/db/JobStorer/json/log。先原NewDriverCreator.Create创建一次实际后端，再嵌原Config，仅给这一个Worker的connection覆Driver/Via，将同一实例交真实NewWorker。没有复制worker/数据库队列，也不全局改配置。

Tries只由owner给一次。已有ShouldRetry精确委托一次，原bool/delay不变；没有策略就与原worker一样使用attempt < ownerTries，0/1都是一次。只有false才旁路Capture，监控/observer失败不替换此结果。Handle实际仍走原JobStorer.Call，不能据包装Job宣称业务日志已经得到task context。

Push直接委托原后端，待序列化Job不包装。只有Pop/Receive拿到的reservation被观察；支持Receive的原后端使用独立包装类型，不能给纯Pop虚构Receive能力。Task/chain/slice复制后装饰策略，owner不缓存/输出Args或payload。

每reservation一个有界EventScope，chain-index区分任务；Delete无论成功失败都defer关闭scope，原error保留。同UUID再次投递是新生命周期，会正常累加，不靠永久UUID账本抹掉重投。容量不足只诊断，不拒绝业务任务。

```go
worker, err := goravel.NewQueueWorker(goravel.WorkerDependencies{
    Config: queueConfig, Cache: cache, DB: db,
    Jobs: registeredJobs, JSON: json, Log: originalLog,
}, goravel.WorkerConfig{
    Connection: "jobs", Queue: "default", Concurrent: 4, Tries: 3,
    Collector: collector, Context: ownerContext,
})
if err != nil { return err }
// 由应用既有生命周期在确认启动后触发Shutdown，并等待Run结束。
// 这里只展示阻塞执行入口，不提供立即并发Shutdown的时序承诺。
return worker.Run()
```

这示例展示实际公开采用点，不是已替换Host。原Worker.Run会重置初始化状态，不能启动goroutine后立刻Shutdown来假设时序已同步；具体启动确认/退出同步留后续Host采用设计。Sync Driver的Worker.Run原本不消费任务，DispatchSync也未由这个入口观察。Shutdown保持原Worker限制，包括非幂等和任务/原重试delay不响应关闭时可能等待；借用池/cache/logger不归SDK关闭，派生采集timeout也不是原worker硬实时关闭保证。

## 采集时点与原失败日志不能混为一谈

旁路发生在“最终重试策略false”之后、原TaskToJson之前。序列化失败时，SDK可能已有终态记录，而原failed_jobs没有写入；这与PHP JobFailed具体事件时点有差异。测试确实让原payload含不支持序列化的假值，并验证末次采集与原处理继续，没有造新的失败存储。

原Worker在失败DB关闭或写失败时会把FailedJob（含payload/Exception）交原日志。SDK只保证自己Record的脱敏，不能撤回原日志，也不能把已字符串化日志与原error自动同ID合并。Host采用时必须单独审这条真实双消费者/敏感原日志路径，不应临时加全局过滤器冒充本块安全完成。

Go普通error也不携带PHP Throwable那样的创建栈。本课记录观察时可得调用栈，不伪造业务行；跨SDK类/位置/hash不保证相同。Job无ctx，业务日志未显式关联时可能独立计数，不能把message相同当同error。

## 调度的 error、退出码和 panic 是不同输入

NewScheduledArtisan只用于原Schedule构造所接的Artisan。四公开方法原样保留，仅Call在原delegate返回后旁路；成功不采。错误没有整数code就是unknown，不填1或0。代码内临时meta使用null表示未给出，现Builder的runtimeMeta会过滤null，因此持久Record并不保证保留exit_code键；缺失表示未知，不能读成0。实际返回的ExitCoder可读取code作为元信息，原error仍优先，不能因为cloud签名吞掉真实代码异常。

Signature resolver由owner把已知调用映射为安全command名称。false/失败不持久化不可信command，仍保真实error/安全诊断和原Call返回。不要把带参数完整命令串当安全标签；signature-only是减少敏感参数持久化的Go选择，与PHP完整summary的hash不宣称等价。

原console.Artisan.Call→Run(false)仍可能被urfave ExitCoder默认OsExiter在返回前终止进程，CallAndExit更不能当可拦截完成事件。本块不覆盖全局OsExiter。显式Complete供真正知道结果的owner调用：Err优先；只有Err=nil且明确非0 code才合成ScheduledTaskExit；已知moo:cloud:* signature只排除这种合成退出。

```go
scope, err := capture.NewEventScope(1)
if err != nil { return err }
defer scope.Close()
code := 2 // 由实际执行owner取得，不能把未知错误猜成2或1。
_, err = observed.Complete(ctx, goravel.ScheduleReceipt{
    Signature: "backup:run", ExitCode: &code,
    Scope: scope, EventID: "this-execution",
})
return err
```

scope/ID是调用方明确关联，不从command/message推断弱身份。同ID重复完成复用现Collector的计数/来源升级规则。原callback func()没有error返回；Callback包装只在panic时采集，随后原样repanic给原cron.Recover，监控失败/observer panic也不替换原值。error保原，string可作消息，其它值只用静态类型名，不调用任意Stringer。Command.Handle内部若已被原框架goroutine转换错误，SDK只消费实际返回对象，不声称恢复丢失的原panic。

## 本轮真实验证

2026-10-30，Go1.27.3，GOWORK=off：Worker首组0.985s；真实Schedule/Artisan/cron四组1.617s；成熟Database Driver私有SQLite1.016s。最终只本块TestQueue*/TestScheduled*/TestSchedule*/TestRealSchedule*定向race通过7.333s，包含4任务同时入闸、同UUID8次独立投递、默认0/1/N与原策略、chain停止、Delete失败、监控/observer失败、TaskToJson失败时点、nil backend、实际jobs删除与failed_jobs持久化、真实cron恢复。

首轮Driver字段名遮蔽接口方法、测试YAML整数动态类型/成功分支mock要求、成熟DB fixture缺GetDuration，以及schedule消费缺cron sum都已按真实输出修正。get/tidy只增加framework锁定cron/v3 v3.0.1与两条sum，framework/GORM/xsys/cpuid未升级；没有为依赖纳入扩大测试。

未跑全量/E2E、真实消息服务/Cloud、Host生产worker/全局scheduler接线。用户练习待完成：画出false决策→旁路→TaskToJson→failedJobChan→Delete的顺序，解释为何不能用每次Handle错误冒充JobFailed；区分未知code/真实error/合成退出/原panic；指出业务日志无ctx和原failed日志含payload的后续责任。

## 暂停与恢复入口

本轮14收口后暂停，不填时间开新块。下一步是Host显式QueueWorker/ScheduledArtisan owner采用设计：原Config/连接/并发/tries一处配置，监控默认关闭，启动/Shutdown与monitor Buffer先后顺序，真实业务job/command消费，以及原失败日志敏感信息和显式事件身份关联。当前默认Host Worker/全局Artisan未自动替换，真实Cloud、fatal/OOM与其它矩阵缺口保留。
