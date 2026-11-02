# 20：Cloud确认后的local噪音抑制

本轮第二功能，对应Host50；第一功能为精确待推统计，分别提交后暂停。仅SDK/Host已审路径，Git由implementation统一；不真实Cloud外写/E2E/全量，不新分支或worktree。

## 已确认业务语义

不物理删除或移动任何record，也不把discard当作intake saved/filtered ACK。使用当前scope/type独立suppression ledger，保累计open锚点及所有resolved/deleted/cloud-rejected。新版本因digest不同自然重新待推；intake确认账本和cursor不因抑制前进。

操作显式获取与同类型Sync相同的非阻塞gate/file lock；网络前读取严格ledger、快照，候选仅当前未确认/未抑制的open且context.env精确local。固定候选后释放buffer热锁再请求Cloud；type锁覆盖整次协调，锁忙不请求。Cloud必须返回严格ok/deleted>=0；失败不新增任何抑制。成功后逐项重新比较bucket/timestamp/实际内容SHA256，只有同版本仍是local open才持久记录suppression；网络中新版本/已确认/已解决记录保留且不抑制。scope/type隔离，未知损坏文件失败可见，不能当空覆盖。

Stats与Sync共同读取suppression作为独立选择依据；不能复制成另一套pending算法。统计应区分confirmed/suppressed/pending；Sync不发送匹配抑制版本，cursor只按真实intake确认前缀处理，不能把suppressed当uploaded。旧抑制随版本改变失配，不应隐藏新事件。

## 与源差异和远端副作用

PHP CloudSync84先remote discard，后按cursor/partial ack删除本地pending open/resolved；本地并未检查context.env。Go采用更窄安全边界，所有记录保留且只抑制local open，避免同hashcount重建后Cloud max漏累计。Cloud既有endpoint仍是当前项目token下所有env=local且open/in_progress的远端集合，不是本地候选hash集合；deleted可为0，不与本地抑制数量相等，不能伪称跨Cloud事务或只影响当前scope候选。

若Cloud成功但本地记账失败，报告remote已发生数量与本地实际完成/失败；不宣称回滚。没有全局event ID/远端幂等receipt时重试至多是至少一次的显式动作。远端resolved/其它env/project仍沿现消费者契约保留，真实Cloud未执行。

## 文件与验证

独占新增discard/suppression实现、定向测试和20详课；现sync/state/snapshot与①Stats共享引用由writer统一整合，先第一功能commit后接第二功能引用。Host50命令须真实make后深化，默认不执行，用户显式调用才会请求并提示远端project-wide local副作用。正式API为Syncer/SlowSyncer.DiscardLocalNoise(ctx)，复用既有HTTP两个discard入口；Host50真实make后的命令为moo:cloud:discard-local。

仅本机httptest与自有临时buffer：remote失败/坏回执零抑制、type忙零请求、confirmed/resolved/nonlocal/deleted保留、同ms新版本竞态、累计count不重置、重开/重启抑制持久、Stats与Sync一致、ledger损坏/IO失败保原、scope/type隔离。ctx沿原HTTP与锁机制传递，本批未新增discard整体取消实测。实际结果见下方，用户自主练习待完成。


## 最终实现与实际验证

文件：cloudsync/discard.go（锁/候选/HTTP/复核/结果）、suppression.go（严格独立ledger与精确pending谓词）、两对应测试；保存复用state.saveBytes，Stats/Sync所有pending判据由writer统一接入。ledger文件为suppression-runtimes.json与suppression-slow-queries.json，Version1+Entries每hash仅Digest/Timestamp/Bucket，bucket只open，坏状态保原。

CloudConfirmed显式区分deleted0有效远端执行与未请求；LocalSuppressed只有保存完成才增加。保存error返回LocalStateUnknown和Failed，重开读账本核实际图像，不假报回滚。历史ACK不证明远端当前存在；remote project-wide操作也可能改其它scope或本地已confirmed对应的local open。本地ACK/所有记录仍保留。DailyCap冻结只增overflow时仍可能命中旧抑制，直到新落盘digest；不能宣称每个新增发生立即pending。

2026-11-02，实际Go1.27.3，SDK单module GOWORK=off；固定fixture clock保留历史值，不作执行日期。

- 严格schema首测PASS0.359s，Host7门槛首测0.849s。
- 完整新增SDK真HTTP首测1.037s；最终TestDiscard/SuppressionSchema范围race2.582s，涵盖Cloud0/坏回执、confirmed优先、record/ACK/overflow字节不改、same-ms新版本、网络中Record、type busy零HTTP、跨scope/slow类型、坏ledger保原/Unknown统计、replace前/rename后目录sync故障与重开图像。没有真实断电验证。
- Host11模式实际Artisan.Call首测1.987s；最终gate race1.549s、command race14.731s。readonly/production/无confirm/falseconfirm/Clouddisabled/无source/缺slow/成功/partial/Cloud0/push开关全关均覆盖；输出不含URL/token/body，文件不变，部分失败继续。
- 必要旧共享范围由writer执行：SDK Stats/Pusher和Sync partial/同ms/state-write选定race2.977s，Host49五模式race7.517s。没有重复旧全组、全量/E2E/真实Cloud。

```sh
GOWORK=off go test -mod=readonly -race ./cloudsync -run '^Test(Discard|SuppressionSchema)' -count=1 -timeout=45s
# Host：显式安全env，父DB/认证/自动网络关闭，子进程仅本机假Cloud/自有buffer/相对app根私有日志。
go test -mod=readonly -race ./app/monitor ./tests/feature -run '^(TestLocalDiscardGate|TestMonitorDiscardLocalCommand)$' -count=1 -timeout=90s
```

SDK/Host本轮两功能分别提交后暂停，不push/merge/tag或自动清理。S-Cloud页面、真实Cloud与后续矩阵待未来明确范围；用户自主练习未代做。
