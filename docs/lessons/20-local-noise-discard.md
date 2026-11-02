# 第20课：Cloud确认后，只抑制精确的local噪音版本

[上一课：19](19-pending-statistics.md) · [下一课](21-push-preview.md) · [课程目录](../README.md)

本课为S-Cloud一期补一个显式管理动作，复用既有HTTP、type锁、快照和原子状态写入。[计划](../plans/20-local-noise-discard.md)列契约、实际验证与安全差异；Host采用对应模板第50课。它不物理删除记录，不把“用户不想再发这个版本”伪装成“Cloud已经保存这个版本”。

## 先把远端和本地范围分开

Cloud既有discard-local接口只接受`{token, env:"local"}`。它影响该token项目下、选中类型的**全部远端local unresolved**（open/in_progress），不是当前本地scope候选hash列表。其它环境、其它项目和远端resolved按当前服务端规则保留。响应`ok=true, deleted>=0`才是有效操作确认；deleted=0也有效，不必等于本地候选数。

本地Confirmed只表示某版本曾取得合法intake ACK，不证明远端现在仍存在。远端这次操作也可能把其它scope或本地已confirmed对应的remote local open置deleted；本地仍保留历史ACK和累计锚点。SDK不能把这描述为“已确认数据在远端全部不动”。没有额外远端查询或跨云事务。

## 为什么不直接unlink YAML

PHP CloudSync当前在Cloud确认后，按cursor/partial ack判断pending，随后删除本地open/resolved文件。本地选择没有额外检查record.context.env。Go本步采取更窄安全边界：候选必须未exact-confirmed、未exact-suppressed、bucket=open、Context.Env精确local；resolved全部保留。

直接删除同hash的更新版本可能连累计基线一起丢失。下一次计数从1开始，而Cloud按max合并旧累计，会在追平前漏累计。因此Go保留所有record、count、first_seen和overflow，用独立suppression账本阻止重复发送这一个版本。

| 状态 | 本次动作 |
| --- | --- |
| exact confirmed open | 保ACK与原记录，不新增抑制 |
| 未确认local open | Cloud确认后复核精确版本，持久抑制 |
| nonlocal open、所有resolved | 保留，不新增抑制 |
| deleted/cloud-rejected | 不进入快照候选，不移动或删除 |
| HTTP期间新增/变化版本 | 保留新版，当前旧候选不抑制 |

## 一条主链就能看见副作用

[discard.go](../../cloudsync/discard.go)的顺序是：同实例gate → 同type非阻塞flock → 严格读ACK/抑制账本与只读快照 → 固定候选 → 请求Cloud → 一次重取只读快照 → map比较hash/digest/timestamp/bucket/local → 原子保存独立抑制账本 → 返回结果。

网络期间没有buffer热锁。真实HTTP测试在服务端回调中同时Record同毫秒的新发生，写入能完成；新digest未被旧候选抑制。type锁覆盖整个协调，防同类型Sync/Stats/其它discard交错；不同scope和runtime/slow类型分别隔离。锁忙不请求Cloud，不把busy写成“成功清理0条”。

本步复用[严格suppression协议](../../cloudsync/suppression.go)及[已有原子状态边界](../../cloudsync/state.go)，没有第二套写入引擎。固定文件为`suppression-runtimes.json`和`suppression-slow-queries.json`，每hash最多一项。必填、非null、版本、未知/重复键、UTF8、hash/digest/open桶和时间严格校验；坏文件报错并保原。

```json
{"version":1,"entries":{"aaaaaaaaaaaa":{"digest":"64位SHA256十六进制","timestamp":"2026-11-02T12:00:00+08:00","bucket":"open"}}}
```

上面是字段说明，不是可直接通过校验的fixture；真实测试使用实际YAML字节SHA256。

## 抑制不等于ACK，统计和游标各守职责

Stats、Sync和Discard共享`pendingSnapshot`。分类先看exact confirmed，再看exact suppression：

```text
Open = ConfirmedOpen + SuppressedOpen + PendingOpen
Resolved = ConfirmedResolved + PendingResolved
```

Sync初筛、批次后Pending重算和ErrPending判据都排除exact抑制。cursor仍只依据真实Confirmed前缀；抑制可能留下水位洞，不能把cursor解释为“所有版本都已保存”。Stats只读，不GC旧条目；过期指纹可暂存，失配就不生效。

新落盘版本因digest变化重新待推。注意DailyCap冻结时，新发生只增加overflow、record字节可能不变，因此仍可能命中同一抑制版本，直到既有Refresh合并overflow并写新digest。overflow不清，不为管理动作绕过cap。

## Cloud已执行、本地结果未知，要分开报告

Result显式保`CloudConfirmed`，即使CloudDeleted=0也能区分“已执行”和“根本未请求”。`LocalSuppressed`只有持久化确认后计数，`ChangedOrPreserved`与`Failed`分开。

Cloud确认后，本地短写、rename或目录sync失败，返回`LocalStateUnknown=true`和可识别error；不报全部已抑制，不声称远端回滚。重开后严格读账本是本地恢复真值。测试分别在replace前和rename后的目录sync失败：前者未抑制，后者账本已可见但调用仍报告未知。没有真实断电验证，也不宣称跨网络exactly-once。

## 实际验证与练习

2026-11-02，Go1.27.3：schema首测0.359s；完整新增真HTTP定向1.037s；最终新增范围race2.582s。包含零远端删除确认、confirmed优先、文件/ACK/overflow字节不改、同ms版本竞态、当前pending/SuppressedOpen一致、IO未知与重开、scope/type/slow隔离、busy零HTTP与坏账本保原。Host门槛首测0.849s，11模式真实Call首测1.987s，最终gate race1.549s/命令race14.731s；没有真实Cloud、全量或E2E。

用户自主练习待完成：解释抑制与intake ACK差别；画出网络期间新版发生的顺序；解释CloudDeleted=0且本地保存失败的结果；说明为何overflow冻结不保证每次发生立即待推，以及远端project范围为何大于本地scope。

受影响旧共享消费者由writer单独验证：SDK Stats/Pusher与Sync partial/同ms/state-write选定范围race2.977s，Host49五模式race7.517s；与本课新范围分栏，不重复旧全组。
