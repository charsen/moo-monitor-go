# 第19课：准确知道哪些版本仍待推

[上一课：18](18-health-check.md) · [课程目录](../README.md)

本课为S-Cloud一期提供真实只读统计，Host消费对应第49课。[计划](../plans/19-pending-statistics.md)保留实际结果和边界，下一功能另做local清理协调。

[StatusReader](../../cloudsync/status_reader.go)只接已有runtime/slow Buffer，不接Client、URL或token。现Syncer.Stats与Pusher.Stats共用[读取实现](../../cloudsync/pending.go)：每类型gate/flock → 严格load ledger → ReadOnlySnapshot → 完整bucket/digest/timestamp匹配 → 分类计数。Cursor仅展示已存值，不推进或剪ACK。类型锁Busy、坏ledger/记录、closed/取消都不能当作有效pending0。

每类结果Known=true时才有Counts/Cursor。Counts区分Scanned、Open/Resolved、ConfirmedOpen/ConfirmedResolved、PendingOpen/PendingResolved；Known=false时指针缺席并给Reason/Busy。无源是Unavailable，不是空缓冲。记录条数不等于累计异常次数，open锚点的Count不能拿来充待推条数。

同毫秒刷新即使timestamp没变，只要文件bytes改变，digest就不同，旧ACK不能隐藏新版本。跨桶迁移也要重比bucket。旧cursor不作为单独过滤条件，因此不能越过失败洞。

[ReadOnlySnapshot](../../buffer/readonly_snapshot.go)持原热锁，intent存在就报ErrRecoveryRequired，不恢复。它和普通Snapshot复用snapshotLocked枚举/解码/排序，只有普通Snapshot维持原recover行为。Stats不HTTP、不AfterPush、不save、不GC，也不调用恢复。

严格只读指Stats操作本身。Reader构造会开scope并可能prepareLock；借用Buffer初始化也可能恢复。调用方必须把初始化和读取分开，不能宣称整个新进程零文件变化。Pusher构造现在保留所有传入buffer句柄供Stats读取，push开关只门控发送；关闭仍一并关闭句柄，不关借用Buffer。独立本地读取优先StatusReader，不需要假Client。

2026-11-02实际Go1.27.3：首轮Stats四项0.829s，补坏记录/实际Sync ACK与禁推仍统计后0.528s；最终Stats+现Pusher关闭/取消/partial及必要Sync类型消费者race3.430s，选定旧Snapshot消费者race2.102s。Host49五模式真实Bootstrap/Artisan race7.597s，Cloud关闭/无凭据也读，busy/坏状态明确unknown。文件列表/内容SHA前后不变、零transport/heartbeat与未决journal保留均有定向断言。无全量/E2E/真实Cloud。

自主练习：解释一个旧ACK为何在同毫秒更新后失效；区别Unknown、Busy、有效零；标出构造与Stats的副作用边界，并画出类型锁与缓冲热锁顺序。用户练习仍待完成。
