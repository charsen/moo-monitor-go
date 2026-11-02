# 第22课：显式重推，不用旧ACK掩盖本次失败

[上一课：21](21-push-preview.md) · [下一课](23-confirmed-resolved-maintenance.md) · [课程目录](../README.md)

本课让已有Sync主链支持显式All模式，不复制第二套发送器。[计划](../plans/22-explicit-all-replay.md)记录范围与实际验证，Host采用对应模板第52课。本轮两功能完成后暂停，不做retention或真实Cloud写入。

## All忽略历史ACK，但保留用户抑制

PHP CloudSync的all选集忽略cursor和partial ACK，读取当前open/resolved。Go沿用显式重推，同时保留SDK20新增的exact suppression：它是用户discard意图，不等同已上传状态。

| 当前版本 | Incremental | All |
| --- | --- | --- |
| exact confirmed、无exact suppression | 不发 | 重推 |
| exact suppression | 不发 | 不发 |
| confirmed与suppressed同版本重叠 | 不发 | 保守不发 |
| 没有有效ACK或抑制 | 待推 | 重推 |

Stats仍按confirmed优先分类；All预览不能把Scanned减去分类中的SuppressedOpen就当选集。重叠时，分类可能记confirmed，但operation仍尊重exact suppression。`selectionReason`是Preview和Sync的共同候选判据；All的excluded_confirmed为0，不能把历史ACK排除数照抄上去。

All只重推**当前仍在open/resolved的版本**。已正常确认后即时清掉的resolved、deleted、cloud-rejected不会被复原；抑制账本也不被暗中清空。

## 明确API，默认与自动路径不变

| 文件 | 职责 |
| --- | --- |
| [replay.go](../../cloudsync/replay.go) | All/SyncOptions与精确模式选择 |
| [sync.go](../../cloudsync/sync.go) | 同一锁/快照/receipt/Compare/持久化主链，分开本次余项 |
| [preview.go](../../cloudsync/preview.go) | Mode驱动的只读选集，不网络或推进状态 |
| [pusher.go](../../cloudsync/pusher.go) | 手动选项、每类型继续、现owner取消与独立心跳 |
| [replay_test.go](../../cloudsync/replay_test.go) | 真本机HTTP、历史ACK/抑制/失败与Tick消费者 |

`Sync(ctx)`仍是增量；`SyncWithOptions(ctx, SyncOptions{Mode: All})`显式选All。手动Pusher使用`PushWithOptions`，普通Push和自动Tick固定Incremental。选项不写进全局配置，不会让下一轮定时任务也重推。

主链顺序不变：type gate/flock → 严格ACK/抑制与快照 → mode选集 → 分批HTTP → 全批receipt校验 → 精确Compare → 逐条确认/隔离 → 原子账本。不会为了重发而清空匹配历史ACK或伪造cursor；既有失配/孤立ACK剪枝规则保持。

## Pending与Remaining不是同一个问题

Pending沿既有快照规则，表示当前选读快照中尚未确认且未抑制的实际待推。Remaining是**本次选中版本**尚未得到合法处置的余项。

例如某open版本以前saved，普通Pending=0。显式All这次得到retryable，历史ACK仍保留，但Remaining=1，返回失败；不能用旧ACK让本次重推“成功0条”。saved/filtered只有新回执合法且Compare仍匹配才完成本次项；同毫秒新落盘版本不被旧回执覆盖。永久拒收继续原隔离规则。

Remaining也不是跨重启的持久任务进度。即使Remaining=0，最后保存ACK失败仍须返回error/Failed；磁盘重开可能仍待推。下次显式All会再选全部eligible版本，不只上一次失败项。正常Sync/Tick继续增量，不自动把失败All变成新的全量任务。

cursor仍基于真实ACK历史，不代表本次所有重推都成功；Result也只描述本次读取的快照边界，不保证请求结束时磁盘没有更晚事件。

## 与dry-run组合

Host `moo:cloud:push --all`显式重推，`--all=false`仍增量。`--all --dry-run`使用All选集，只读预览，零intake、heartbeat、ACK/抑制写；不检验Cloud接收或权限。`--all --dry-run=false`则是真实All操作，应按实际副作用理解。

操作发送的是已有累计快照，count/时间不因重推生成新事件；Cloud按既有max累计契约处理，不能把重复发送次数当新增异常数。本轮所有传输只对本机httptest，真实Cloud版本/字段/权限仍未验。

## 实际证据与自主练习

2026-11-02，实际Go1.27.3。新增四组SDK真HTTP定向首测1.717s，最终race2.787s；首编译只有测试unused import已修，未改业务绕过。覆盖真实先Sync取得ACK后重推、exact suppression和重叠、预览、retryable/HTTP失败/坏receipt且旧ACK字节不变、partial filtered、same-ms新版、末ACK保存失败Remaining0仍error、runtime/slow与Tick。

Host八模式实际Call唯一race11.254s，由writer并行执行；旧共享SDK选定race2.973s、旧HostPreview五模式race7.636s、旧invalid子例0.847s分栏。无全量/E2E、真实Cloud、push/merge/tag。

用户自主练习待完成：比较Pending0/Remaining1和Remaining0/error两种结果；解释All为何保suppression；说明全量预览与正常分类的差异；画出一次All后自动Tick仍增量的调用链。
