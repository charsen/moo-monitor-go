# 第14步：队列最终失败与调度完成边界

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

13已提交67cef24，Host43已提交52e8491。本步只monitor，沿真实framework1.18公开构造，不修改Host全局queue/Artisan，不连接真实Cloud或外部消息服务。用户要求本轮14收尾后暂停，不开启后续Host采用块。

## 队列接口与状态机

adapters/goravel/queue.go：NewQueueWorker(WorkerDependencies{Config,Cache,DB,Jobs,JSON,Log},WorkerConfig{Connection,Queue,Concurrent,Tries,Collector,Context,CaptureTimeout,MaxEvents,OnError})→*QueueWorker，提供真实Run/Shutdown及安全Status。

先原NewDriverCreator.Create得到成熟后端唯一实例，再用嵌Config只覆该connection的Driver/Via，将同一装饰实例交真实NewWorker。Tries只配置一处，同值用于原Worker和无策略时的tries<ownerTries；0/1均只尝试一次。原ShouldRetry委托恰一次，返回false才旁路Capture原error，bool/delay原样返回。DriverWithReceive有独立包装类型，纯Pop不假装Receive；Push原样委托，不改待序列化任务。

Reserved.Task只复制当前Task/chain及必要slice，不保存/输出Args或payload。每reservation一个有界EventScope，chain-index ID区分；同UUID跨投递不永久去重。容量满只安全诊断，不拒绝业务任务。Delete无论成败defer关闭scope，原Delete返回不变。Job.Handle仍由框架JobStorer.Call执行，包装Handle不意味着传播ctx；默认业务日志无显式共享ID时不保证跨入口去重。

原Worker.call最终决策之后才TaskToJson/failedJobChan，故JSON失败时已观察到终态但原failed_jobs可能未写，这与PHP JobFailed事件时点有差异。Native Handle panic不属于可读终态error，不复制worker来补；关闭仍由原Worker等在途任务。

## 调度接口

schedule.go：NewScheduledArtisan(delegate,collector,ScheduleConfig{Signature resolver,CaptureTimeout,OnError})，保四公开方法与原返回，只Call旁路。signature由owner明确解析为已知安全名称，拒空白/参数串，不持久化带参数整命令；resolver失败诊断并省略不可信名称，仍保真实error采集，不阻断原Call。

提供callback包装和显式Complete(ctx,ScheduleReceipt{Signature,Err,ExitCode *int,Runtime *time.Duration,Scope,EventID})。Err优先，code只已知meta；只有Err=nil且明确非0才合成ScheduledTaskExit。cloud已知signature只排除合成退出，不吞真实error。成功不采，未知code不填1，不覆盖全局OsExiter；urfave ExitCoder若在Call返回前退出进程，本适配无法假称看到了回执。Callback panic采后原样repanic给原cron.Recover，不取代其日志。

每次自动Call/callback有独立有界scope；显式Complete依owner给明确scope/EventID或显式ctx关联，不按command/message推断同Throwable，不维护弱对象猜测表。来源和脱敏/hash/metadata通过现Collector/Builder/Buffer。

## 文件/复用/验证安排

新增completion.go仅共享派生采集超时与安全旁路诊断（queue/schedule两真实消费者），主ShouldRetry/Call流程就地可读，无通用pipeline。新增queue_test/schedule_test及queue_database_test：真实Worker+内存Driver验证retry→success不采、最终一次、原策略/默认0/1/N、chain停止、并发、Delete失败、采集/observer失败、shutdown；成熟Database Driver用本次私有SQLite验证采用，不造存储。调度真实框架Schedule/Artisan受控构造或明确delegate验证success/error/未知与明确code/cloud/重复ID/panic原样传播。只相关定向及一次实际并发race，不全量/E2E/真实服务。

以上为本块定向验证范围，实际执行结果见下方；未验证项不标通过。依赖沿已有framework/sqlite/GORM，不追版本。14结束写明确下一Host采用任务与剩余未知边界，用户练习待完成。

## 实际结果与暂停交接

2026-10-30，Go1.27.3/GOWORK=off：真实Worker首组PASS0.985s；Schedule/Artisan/cron四组PASS1.617s；成熟Database Driver自有SQLite PASS1.016s（真实jobs删除0/failed_jobs1/monitor末次1）。最终本块定向race PASS7.333s，含真实4入闸Receive并发、同UUID八投递分别累计、原policy false/Tries99一次、TaskToJson失败先观察终态、nil backend拒绝及已列主链。普通结果属早轮，不重复跑已过项。

首轮修正Driver嵌入字段遮蔽Driver()，测试metadata YAML整数类型/成功分支不应强求错误日志mock，DB fixture补真实GetDuration；schedule首次导入缺cron sum后按已核固定图get/tidy。依赖仅新增cron/v3 v3.0.1一require与两sum，无framework/GORM/xsys/cpuid漂移。无PHP执行/新fixture、全量/E2E/真实服务。

原failed日志可能含payload/Exception，本旁路不能撤回它或自动与原err同ID；Host需后续审真实双消费者。signature-only、安全名称而非完整参数命令是明确Go选择，跨SDKclass/栈/hash不保证相同。CallAndExit/真实os.Exit不可拦截，Job无ctx不自动传播业务日志scope；NativeWorker.Shutdown原限制未改。

本轮完成14必要实现/定向/文档/commit后暂停，用户练习仍待其完成。恢复入口唯一为后续Host显式QueueWorker/ScheduledArtisan owner接线：以本步queue.go/schedule.go公开工厂，落实独立配置/默认关闭、启动和原Shutdown/Buffer关闭顺序、真实业务消费，核原FailedJob日志敏感路径与显式同事件关联。默认Host Worker/全局Artisan当前仍未替换；不实施该下一块，不开始Cloud/MCP或新矩阵功能。

## 当前契约衔接（2026-11-03校准）

15/16与Host45/46已通过默认关闭的显式owner接入；这里的暂停和“未接Host”仅是14节点历史，当前owner生命周期见对应后续设计。全局Artisan并未被替换。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
