# 15：Host队列Owner生命周期

已实现等待终验提交；[详课](../lessons/15-host-queue-owner.md)。只深化SDK14明确工厂与生命周期，不复制原worker或自动开Host业务。

WorkerConfig.DisableCapture仅显式true允许nil collector且不建observer/scope；原nil仍拒。Managed Run一次、preRun关闭、首N Pop/单Receive屏障、sync done早返、native Shutdown至多一次与重试等待均由queue_lifecycle.go协调。Started与Running含义不同；取消仅停止等待，不承诺中断原Pop/Job/retry sleep。借用资源不Close。

实际消费者Host45显式Provider替原Runners，观察关闭照常运行；原default sync与JobStorer不改。queue-only日志保护在Host，SDK不猜任意业务日志内容。Owner失败状态公开，不把Error void伪造成持久化确认。

2026-10-30，实际Go1.27.3，GOWORK=off：最终`go test -race ./adapters/goravel -run '^TestQueue' -count=1` PASS9.924s。旧14真实DB消费者与新增6 managed用例同范围：preRun、重复、N=1/4/16、Receive取消、sync早返、原不可取消Pop、inflight取消后重试。先阶段managed0.652/0.596s不代替最终。Host真实Bootstrap四模式/原官方SQLite队列表PASS6.600s，配置/日志2项0.995s；无外部队列/全量/E2E。随后仅schedule Host采用，完成两功能暂停。
