# 23：确认resolved安全维护

2026-11-02。独立store-only维护，沿PHP CloudSync.pruneLocal语义：RetentionDays<=0不读写；正数只启用confirmed resolved兜底，不是N天龄策略。open累计锚点、未确认resolved、deleted、cloud-rejected全部保留。Go比源cursor时间比较更严格：完整digest/timestamp/bucket ACK才授权候选。

PruneOptions{RetentionDays,DryRun}；StatusReader.Prune(ctx,target,options)与Syncer同type方法复用type gate/flock，严格读取ACK/suppression与ReadOnlySnapshot。DryRun零HTTP/heartbeat/save/Compare/recover。实际Buffer.PruneResolved固定resolved，热锁下intent存在即ErrRecoveryRequired；正常Compare仍可恢复，二者只共享compareLocked精确匹配/删除内核，不复制规则。失败不当0，Busy/Unknown/OutcomeUnknown分别呈现。ACK/cursor不剪枝，默认Sync即时resolved回收不变；常态候选0合理。

## 实际结果

SDK首3组TestPrune0.782s；补slow/typebusy与保护桶后最终4组race1.940s。exact ACK/未来cursor不授权/同ms新版/二次核race/失败/journal保原/Days<=0坏状态无IO/dry字节不变及独立slow均有断言。共享Compare提取的既有buffer选定Compare/ResolvedFailure/ReadOnlySnapshot race1.600s。Host53十模式race13.427s：无token/Cloud关闭/production可做本地exact维护，confirm/readonly仍门控，dry/apply/zero/非法days/Busy/journal/无源明确。

全部私有目录/本地HTTP spy0，无真实Cloud/生产/全量/E2E。不重复已过范围。用户已要求持续推进，每功能commit/HEAD回读后接24；见[持续队列](../execution-queue.md)，历史暂停不删除。
