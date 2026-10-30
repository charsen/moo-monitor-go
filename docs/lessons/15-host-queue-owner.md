# 第15课：队列Owner怎样安全启动和关闭

[上一课：14](14-queue-schedule.md) · [课程目录](../README.md)

SDK14已有真实Worker工厂，本课增加它被Host长期拥有时必需的窄生命周期。设计与实际验证见[计划](../plans/15-host-queue-owner.md)。

[queue.go](../../adapters/goravel/queue.go)只显式DisableCapture=true才允许nil collector，不构造采集scope，原任务仍执行。Host借同一个Queue.JobStorer/cache/db/JSON/log与唯一Driver，未复制worker循环。

[queue_lifecycle.go](../../adapters/goravel/queue_lifecycle.go)负责Run至多一次、Shutdown-before-Run阻止启动、重复关闭不重复close原channel。原Run重置取消状态，因此Run请求发生后必须等Started或done；N个原Pop首轮全部入闸，才证明全部jobWg.Add已经发生。Receive只有一个接收器，sync直接done，不虚构Started。

Running表示已请求且未返回，Started才是poll屏障。ShutdownContext取消仅让调用者停止等待；原Handle、不可中断Pop和retry sleep仍可能在途，Closed不能提前true，重试Shutdown等待收尾。借用资源由外层Owner关闭，当前Host queue priority5先于monitor200结束，不保证任意第三方runner。

[生命周期测试](../../adapters/goravel/queue_lifecycle_test.go)覆盖上述顺序、N=1/4/16、idle Receive取消、sync早返与blocked Pop。2026-10-30实际独立SDK图TestQueue命名项race PASS9.924s，含既有真实Database Driver消费。Host45四隔离Bootstrap模式PASS6.600s（业务不随监控关闭、retry成功/最终失败/原failed_jobs、在途关闭与固定假payload日志边界），Host queue-only日志代理另测0.995s。无生产队列/全量/E2E。

自主练习：画出Run/preRun Shutdown、Started后Shutdown与context取消三条顺序，指出Closed何时才可true；比较DisableCapture与业务connection/default的独立性。用户练习仍待完成。
