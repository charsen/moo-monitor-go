# 第06课：增量快照、严格回执与确认账本

[上一课：05](05-runtime-buffer.md) · [课程目录](../README.md) · [下一课：07](07-cloud-http-client.md)

05把显式事件可靠地留在本地，本课解决“哪些版本已被处理、失败后只重发什么”。新增[cloudsync](../../cloudsync/sync.go)，消费05的Buffer，不自动调度，也不连接真实Cloud。范围见[本步计划](../plans/06-incremental-sync.md)。

## 两种指纹解决不同问题

04的12位hash仍是事件身份：class、file/line和归一化message决定哪些事件累计在同一记录。06新增64位SHA256，是实际存储YAML字节的版本指纹，不能把它当成新的事件hash。

Snapshot包含Hash、Bucket、Timestamp、Digest与typed Record。Timestamp按源选择meta.updated_at，仅不存在或null才回退last_seen；空串不回退。Go私有缓冲要求有效RFC3339时间，空/坏值明确error，不猜一个当前时间。源[CloudSync.recordTimestamp](../../../moo-monitor-laravel/src/Cloud/CloudSync.php)的纯probe保留了空串样本。

同一毫秒可以发生count1→2，两份YAML的timestamp相同、Digest不同。只比较时间会把旧回执错当新版本确认，或因cursor相等永久跳过；本课的实际定向测试证明旧ack不能确认count2，下一轮会发送它。

## 调用链从快照走到确认

| 文件 | 职责 |
| --- | --- |
| [buffer/snapshot.go](../../buffer/snapshot.go) | 锁内恢复/读快照、实际字节Digest、同版本确认/隔离 |
| [cloudsync/receipt.go](../../cloudsync/receipt.go) | HTTP状态与JSON协议存在/类型/闭合校验 |
| [cloudsync/sync.go](../../cloudsync/sync.go) | push锁、扫描筛选、Send、逐项处置、账本/安全水位 |
| [cloudsync/state.go](../../cloudsync/state.go) | 私有scope身份、独立锁、严格JSON账本、完整原子写 |
| [buffer定向](../../buffer/snapshot_test.go) / [sync定向](../../cloudsync/sync_test.go) | 版本竞态、故障、真实源fixture与受控HTTP替身 |

典型使用由调用方显式组合：

```go
syncer, err := cloudsync.New(store, client, cloudsync.Config{})
if err != nil {
    return err
}
defer syncer.Close()
result, err := syncer.Sync(ctx)
```

store是05显式打开的Buffer；client实现Send(ctx, []buffer.Snapshot)(cloudsync.Receipt,error)。默认batch100，显式BatchSize小于1按源取1。没有启动后台worker、默认Host绑定、真实Token或远端连接。

Sync先拿同实例gate和同scope/runtime独立非阻塞push锁，重叠调用返回Busy。然后读账本、短持Buffer锁恢复与扫描、释放热锁、筛未确认版本、Send、整批校验、逐项Compare、原子持久账本。网络期间Record仍能取得自己的热锁；不能为省事让push长时间占用异常记录锁。

Close由owner负责；传入Client须遵守ctx与安全错误边界。当前没有生产HTTP实现，不能从接口存在推导出鉴权、TLS/超时/连接重试已完成。

## 什么才算一批有效回执

[CloudClient源](../../../moo-monitor-laravel/src/Cloud/CloudClient.php)只有2xx、ok严格true、非负整数saved/filtered/skipped且总数等于sent时才允许确认。JSON的1.0、null、字符串数字均不是整数协议；缺字段也不能默认成0。

results缺席仅兼容skipped=0的旧完整成功；一旦字段存在，null/标量/残缺数量都拒绝。实际PHP把JSON对象关联值也当array处理，因此Go同时接受数组或对象values，最终按item.index排，不依赖对象键顺序。纯probe也实测sent0时空[]和{}都成功、null拒绝；Sync不会对空待发集合调用Send。

每项须有index/hash/status/retryable/reason。index是唯一合法整数，hash等于对应输入，status只saved/filtered/skipped，retryable是真bool，reason必须存在且为null/string。非skipped不能retryable；skipped必须非空reason。各status计数还要与顶层闭合，不能只检查每项看起来合理。

Decoder验证原wire；Sync还会验证Client返回的typed Receipt，避免自定义Client绕过数量、index、hash和状态规则。整个batch通过后才开始第一项本地副作用。某后续batch无效不回滚此前已持久确认的batch，也不确认这个坏batch里的任何项。

source连接异常最多3次/200ms、4xx/5xx不即时重试是未来HTTP adapter的依据，本课没有实现这些网络策略。httptest中只有测试adapter走Decoder、请求体也固定假值，不是完整生产鉴权/Record JSON序列化验证。

## 确认账本才是跳过依据

同scope固定sync-runtimes.json保存：

```text
version: 1
cursor: 安全处理水位
confirmed:
  <hash>: {digest, timestamp, bucket}
```

每hash最多一条权威确认。只有当前Snapshot三个版本字段都精确匹配才跳过，不因timestamp<=cursor直接丢掉记录。扫描会清理已不live/版本不匹配的旧确认；已确认open仍是累计锚点，不再重复发送原版本。

cursor按本轮扫描时间组表示连续安全处理前缀。同timestamp组任何一项未确认，该组就不能越过；前面已确认组可以推进。若新版本时间回退到旧cursor之前，水位可回落到安全前缀或空。网络前可持久清理和回落，但不能制造新确认。

这明确强于PHP时间-only的保护方式，不冒称原单调cursor协议完全复刻。下一轮仍扫描实际记录并用指纹账本判断，所以时钟回退、同毫秒更新不会被旧水位永久跳过。代价是需要扫描和算实际内容Digest，并持久每hash的当前确认。

扫描之后产生的新事件留下一轮；Result.Pending=0只说明本轮快照已处理，不能解释成磁盘全局实时没有待发数据。cursor也只描述所见快照与安全历史，不是对未来回填事件的全局时间承诺。

## 四种结果如何影响本地记录

| 回执 | 本地动作 | 下一轮 |
| --- | --- | --- |
| saved | 同版本Compare成功后确认，open保留，resolved删除 | 同指纹跳过 |
| filtered | 同saved，表示最终过滤而非可重试失败 | 同指纹跳过 |
| skipped / retryable=true | 保原文件、不确认 | 只重发未确认版本 |
| skipped / retryable=false | 同版本原文与安全receipt完整隔离后删源 | 不再扫描隔离区 |

失败洞不会阻止同batch后面的成功项持久确认。实际四项混合样本第一次处理saved/filtered/permanent三项，retryable一项留下；重开syncer后只发送这一项，已确认open锚点仍留盘。

发送期间Record可刷新相同hash。Compare再次加热锁、恢复05intent、读实际源文件，按bucket/timestamp/actual bytes Digest比对。旧open ack不确认新count，旧resolved ack不删已经复发成open的新版本；false会作为待重扫处理，而不是盲目记账。

## 永久拒收要先保留证据再删源

固定隔离目标为cloud-rejected/<hash>-<digest>.yaml与同基名.receipt.json。先完整原子落原YAML和receipt，再逐一回读核字节相同，最后删源。任一目标已存在但内容不同就error保源；不会先删源再尝试保存。

旁车固定version/hash/digest/status=skipped/retryable=false/reason。源只是把reason非ASCII字母数字/_/-换-、trim且无长度cap；自由文本或单词形式凭据仍可能留下。Go更窄：只有已证分类码invalid_record、ingest_filter、upsert_failed原样保留，未知大小写、自由文本或假token句子都变rejected。旁车和公开Result共用SafeRejectionReason；Receipt中的原reason只用于内存协议判定，不透传到日志/持久文件。

两文件不是事务，重试可发现已经完成的一半并核相等，再补另一半；实际测试覆盖原YAML rename失败、receipt rename失败、删源失败、目标/旁车冲突、重开后原因可读。相同版本后来的拒收reason如果不同，也会因旁车严格bytes不一致冲突保源；不能悄悄覆盖已有诊断。

05journal只负责本地Record提交，未被改造成跨云事务。这里没有“云响应和本地写入一起提交”的承诺。

## 本地记账失败为什么仍可能重发

云已保存，但本地账本替换失败，open仍在、指纹未被持久确认，重开后会再发相同快照；这是至少一次。云端累计count应按快照max合并，不能把重复快照当增量sum；本SDK不把open锚点变成删除队列。

resolved同版本确认后可先删除，而账本写入随后失败；该旧记录已经得到有效云确认，所以不需要重新造回原文件。后来同hash复发的新open必须重新扫描；测试证明发送中复发不能被旧resolved ack删除，确认删除后记账失败再出现的新event也能发送。

短写、rename、目录sync失败会保留完整旧账本或返回明确error；不能忽略错误后向调用方声称确认已持久。实际状态JSON拒null必填、重复键、未知字段、非法版本和指纹格式，坏账本保原，不像源readState把解析失败降成空。

## scope、锁与文件安全

Buffer.ScopeRoot只读返回固定path与锚定目录FileInfo。Sync用os.Root重新打开并SameFile核身份；同scope的.sync-runtimes.lock以NOFOLLOW打开，持锁后再比inode。私有权限、拒链接、rooted相对I/O与固定文件名均沿05约束，不仅相信一个路径字符串。

push锁与buffer热锁分开，scope alpha不能确认beta的同hash/digest，也不会阻塞beta独立push。state文件完整写、sync、同目录rename、目录sync；错误不dump记录、回执正文或原reason。只在Darwin实际运行，不承诺Linux/Windows/网络FS或断电耐久已验。

## 真实源与测试替身分别证明什么

[纯PHP probe](../../tools/probe-cloud-receipts.php)只加载SafelyLogs、CloudClient、CloudSync三个声明，拒autoload；构造传固定假cfg/cursor，反射parseItemResults/isAcknowledged/recordTimestamp。不调用send/sync、不发HTTP、不读写cursor或Host。

[原始fixture](../contracts/fixtures/cloud-receipts.json)来自PHP8.3.35、源commit28d424e，保18组receipt、3空results wire形状、5版本比较、3timestamp选择与源SHA。源版本比较只看时间；例如无效相同字符串也能fallback字面相等，Go私有快照明确拒无效时间，不用它作为确认依据。

在Monitor根审脚本后重放：

```sh
php -l tools/probe-cloud-receipts.php
php tools/probe-cloud-receipts.php ../moo-monitor-laravel/src/Cloud/CloudClient.php
```

Go source fixture测试比较实际源输出与Decoder，另验证顶层HTTP/wire门禁。fake Client测试累计/洞/竞态和状态故障；httptest只是受控回执HTTP替身，未实现生产HTTP Client、真实鉴权、连接重试、非法UTF8 Record JSON策略或真实Cloud接收。

2026-10-29，Go1.27.3/GOWORK=off新增buffer五项最终race通过（2.441s）、cloudsync十项最终race通过（3.073s）。早轮普通buffer四项1.233s/cloudsync八项1.558s，必要补强cloudsync五项0.949s/buffer两项0.878s；它们与最终race分开记录。完整新增定向命令：

```sh
GOWORK=off GOTOOLCHAIN=local "$HOME/Library/Caches/go-moo/toolchains/go1.27.3/bin/go" test -mod=readonly -race ./buffer ./cloudsync -run '^(TestSnapshotCompare|TestQuarantineRecovery|TestQuarantineReasonAndFailures|TestSnapshotTimestampAndResolved|TestConfirmResolvedFailure|TestReceiptSourceFixture|TestReceiptWireGuards|TestSyncPartialAndRestart|TestSyncSameMillisecondAndBackwards|TestSyncBadBatchAndState|TestSyncStateFailureAtLeastOnce|TestSyncLocksAndScope|TestSyncHTTPFixtureAdapter|TestSyncResolvedRaceAndStateFailure|TestSyncPrivateStateIO)$' -count=1
```

之后仅维护文档，未改代码或追加测试。没有全量/E2E/外部服务或最低Go1.26运行验证。

## 用户练习（待完成）

先手推同毫秒count1/2对应的hash、timestamp、digest与账本，解释旧ack为什么不能确认新版本。再画四项混合结果的确认表，说明重开后哪些被重发、cursor为何停在失败洞前。

自行解释open累计锚点、resolved同版本删除、隔离两个文件和本地账本分别何时可能已完成；为什么这些仍不能叫exactly-once。指出Pending=0、纯PHP fixture通过和httptest通过各自不代表什么。代理参考验证不代做学习验收。
