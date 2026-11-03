# Cloud 两类源的初始化故障隔离

本次复盘从实际 `Host NewCloudPush → NewPusher → Syncer/SlowSyncer` 发现：即使某类型的推送已关闭，只要传入非nil Buffer，其不安全同步锁也会使整个Pusher构造失败，阻断另一健康类型。不是网络失败或已执行的Cloud请求。

## 兼容边界

不能直接跳过disabled源构造：现有公开Pusher.Stats/Preview明确允许读取推送已关闭的本地源，`TestPendingStatsSlowAndNoTransport`就是实际消费者。类型flags是传输开关，不是本地读取禁令。

Pusher仍在构造阶段尝试所有非nil源句柄。启用源失败仍fatal并关闭已创建的自有句柄；禁用源失败只记录private initError，继续提供另一健康类型。Push/Tick按原flags给disabled，不发送该类型，也不把该初始化错误变成推送失败。无lazy初始化，Stats/Preview不新建锁或恢复intent。

只读方法仅在明确选择坏类型时给Known=false、Counts=nil、Reason=initialization_failed、Skipped空，并返回ErrPushSource及原初始化错误；健康类型保持Known，单独选择健康类型不收到无关错误。nil源保原source_unavailable。成功构造的disabled源仍可读原统计/预览。所有操作仍参与原enterOperation取消与Close；Close仅关闭Pusher拥有的句柄，不关闭借用Buffer/client。

## 实际验证

SDK GOWORK=off、Go1.27.3、readonly且离线缓存，四个named test定向race **PASS1.912s**：新TestPusherDisabledSourceInitializationIsolation用owned runtime/slow Buffer，两方向将禁用源同步锁设为0644，先确认两类启用时该锁确实拒绝；随后核健康type推送确认、坏type传输disabled、只读错误归属/unknown、健康target无污染、锁原文不变与Close保留借用Buffer。新healthy-disabled Preview及原disabled Stats/缺源/explicit target语义一起验证，未修改旧fixture。

没有运行Cloud、MCP、HTTP服务、用户数据库、全量或E2E；未重复既有push/heartbeat成熟组。Host S-Cloud只读构造隔离另见对应Host计划，公开StatusReader constructor与Prune/CLI语义不改。
