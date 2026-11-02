# 第23课：只回收完整ACK匹配的resolved

[上一课：22](22-explicit-all-replay.md) · [下一课](24-migration-inspection.md) · [课程目录](../README.md)

retention的正数在源实现里不是按天龄淘汰，而是开启已确认resolved兜底清理。Go保持这个含义，并把授权从cursor时间收紧为完整版本ACK。它不是容量大扫除，不会清open累计锚点。范围与结果见[计划](../plans/23-confirmed-resolved-maintenance.md)。

## 职责与动作顺序

`cloudsync/prune.go`提供PruneOptions/typed结果和无Client StatusReader.Prune。type gate/flock → 严格ACK/suppression → ReadOnlySnapshot → resolved且exact ACK候选 → dry计数或Buffer.PruneResolved。RetentionDays<=0在访问store前返回Disabled；nil context仍为非法调用。构造reader可能准备锁，不能把整个进程称零写入。

`buffer/prune.go`在热锁内拒绝intent，不自动recover；`snapshot.go`把原匹配/删除代码窄抽compareLocked供正常回执Compare与维护双消费。只有hash/bucket/digest/time都匹配才删除resolved；新版本保留。普通Compare的恢复契约保持，不把它误当维护API。

维护没有网络、心跳或ACK/suppression保存；实际只删除匹配记录，留下历史ACK供下一次正常Sync按既有规则剪孤立项。DryRun不Compare/恢复。已有Sync成功即时回收resolved不变，所以常态Candidates=0正常。

Known表示成功读取候选，Busy/坏状态不能显示为有效0。删除失败增加Failed并保守标OutcomeUnknown，需重新检查本地事实，不能声称回滚。ChangedOrPreserved表示二次核已不匹配/不存在，不能继续删新版。

## 验证与练习

2026-11-02，SDK首测0.782s、最终4组race1.940s；Compare提取受影响旧buffer定向race1.600s；Host十模式13.427s分栏。私有目录、HTTP spy零请求，无真实Cloud/全量/E2E。

自主练习待用户完成：解释为何未来cursor不授权删未确认resolved；制造同ms新版并观察保留；对比DryRun和真实操作的journal边界；解释0候选为何不是故障。后续按[持续队列](../execution-queue.md)推进，不再使用历史两项后暂停安排。
