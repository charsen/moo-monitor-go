# 第11课：显式慢SQL、同一持久内核与独立Cloud协议

[上一课：10](10-host-runtime.md) · [课程目录](../README.md) · [下一课：12](12-gorm-slow-query.md)

本课完成显式ReportSlowQuery→过滤/脱敏/hash→聚合与可恢复Buffer→SQL typed同步/HTTP协议。没有安装Goravel数据库hook、执行SQL或连接真实Cloud。[计划](../plans/11-slow-query-pipeline.md)记录范围和验证。

## 为什么另建记录模型，却不另抄存储

runtime包含exception/trace/payload/snippet；慢SQL需要sql/took/at，没有这些runtime字段。把SQL伪装成异常会污染Cloud协议；复制一份文件锁、journal、overflow和确认逻辑又会留下两套修复点。

本课使用两个真实记录族和一份私有typed store。R是记录，S是输入，V是列表row。family只适配具体schema的Builder、Lifecycle、YAML与row，不接受第三方注册器或任意kind/path。现Buffer具体外壳保既有Runtime API及专属TagSource，新SlowBuffer只公开有过滤的ReportSlowQuery；共同主动作store.record为私有，避免绕过SQL接受边界。

Runtime.Record保留一个兼容入口，查询/快照/Compare等由嵌入共享，不逐方法转发。私有主函数仍顺序可读：gate→文件锁→恢复→读取→容量/cap/复发→最终图像→写intent→目标/清旧/overflow→清intent。没有pipeline或反射调度。

| 文件 | 职责 |
| --- | --- |
| [types.go](../../slowquery/types.go) | SQL独立Input/Record/Request/Context/耗时模型 |
| [builder.go](../../slowquery/builder.go) | 阈值/skip、hash、双SQL脱敏与累计 |
| [bindings.go](../../slowquery/bindings.go) | 窄binding转换与有证据的正数两位舍入 |
| [buffer.go](../../buffer/buffer.go) | Runtime兼容外壳及两族共享主存储流程 |
| [family.go](../../buffer/family.go) | Runtime schema与共同生命周期桥接 |
| [slow.go](../../buffer/slow.go) | SlowBuffer、SQL typed codec与row |
| [slow_nodes.go](../../buffer/slow_nodes.go) | SQL必须字段、容器和kind/tag校验 |
| [recovery.go](../../buffer/recovery.go) | 同一最终图像journal恢复 |
| [snapshot.go](../../buffer/snapshot.go) | typed快照、字节指纹与同版本确认 |
| [sync.go](../../cloudsync/sync.go) | 两typed同步器共用的确认协调器 |
| [slow.go](../../cloudsync/slow.go) | SQL客户端/明确端点及白名单wire |

## 一个显式调用

```go
builder, err := slowquery.New(slowquery.Config{
    Enabled: true,
    MaskKeys: []string{"password", "token", "secret"},
    Env: "testing", Project: "example",
})
if err != nil { return err }
store, err := buffer.NewSlow(buffer.Config{Root: privateRoot, Scope: "example"}, builder)
if err != nil { return err }
defer store.Close()
hash, accepted, err := store.ReportSlowQuery(ctx, slowquery.Input{
    Raw: "select * from foods where token=? and id=?",
    Bindings: []any{"fixed-fake-token", int64(42)},
    TookMillis: 100, File: "app/query.go", Line: 11,
}, time.Now())
```

privateRoot必须是owner事先创建的绝对0700私有目录，不沿用PHP旧桶。accepted=false表示过滤/未启用或失败；true表示此次发生已采集，可能只增加冻结期overflow，并不保证YAML正文改变。err必须处理，结果未知不能盲目重复提交。

Config不读取全局环境。ThresholdMillis为*float64：nil默认100，显式0收集非负耗时；100.5保留浮点过滤门槛，但Record.threshold_ms按源整数截为100。阈值和耗时拒NaN/Inf/负值，阈值还必须在记录整数可表达范围内。Enabled默认false；低于门槛或raw包含skip子串时不写盘，skip大小写敏感、空pattern忽略。

Input的Request、File/Line与Connection由调用方明确提供，Connection是连接名称而非DSN。SDK不采栈、读源码、解析原Request、执行数据库或排后台任务。

## bindings是展示算法，不是SQL执行器

真实[SqlSlowListener](../../../moo-monitor-laravel/src/Recorder/SqlSlowListener.php)按每一个?顺序替换，不识别SQL引号；缺binding保留?，多余binding忽略，无?不消费任何值。Go保留这条有限算法，不能拿结果当可安全执行的SQL。

已消费值支持string、int、int64、有限float64、bool、nil、time.Time。字符串先转义反斜杠再单引号；日期保其给定时区的秒级墙上时间；bool为1/0、nil为NULL。float按已核PHP precision14的字符串形式，指数保留源1.0E+20形状。未知对象/Stringer不调用；非法UTF8拒绝。

Report通过threshold/skip后先验证实际会消费的bindings，再进存储。原因是cap冻结会跳过Build/Refresh：若只在Builder构造时检查，坏binding也会错误增加overflow。测试先冻结，再交坏UTF8/不支持值，原record、overflow和intent均不变。没有placeholder和多余值仍遵循源“不消费”边界。

## hash保源，持久内容采用明确安全差异

真实[SqlSlowRecorder](../../../moo-monitor-laravel/src/Recorder/SqlSlowRecorder.php)的hash顺序是：原raw空白压单空格→IN占位列表压成(?)→数字字面量归N→截8192字节→trim，再对relfile:line|sql取MD5前12。大小写不统一，字符串常量也不整体替换；行号改变会改变身份。路径沿源原始前缀处理，不用path.Clean改聚合身份。

hash仍使用原raw，不因本地脱敏改变。8192字节可能切半UTF8，这是内部MD5输入，fixture用base64保存；输入原raw本身必须有效UTF8。

源仅对last做SQL→Secrets，raw可能含字面凭据；本课按已批准安全差异，对持久raw和last都先同样脱敏。两者各自记录“脱敏后、截断前”的UTF8字节数，再最多展示4096 Unicode码点+…；不新增原文长度字段，不把凭据长度当遥测。

双mask仍是第02课已核的有限SQL/文本算法，不是SQL parser或全部秘密检测器；原有语法盲区没有因调用两次而消失，不能据此证明任意SQL都可安全上传。

源的raw_bytes/last_bytes在mb扩展存在时实际是字符数；其truncate还会在字符数未超、字节数超的夹区切坏UTF8。真实fixture保留这些原结果，Go不复刻这两个缺陷。SQL列表首行仍限160字节，但退到有效UTF8边界，159ASCII+中文不会留半个字；Runtime旧row规则本步未顺改。

Cloud真实消费者把*_bytes当字节数。本课语义与其单位一致，但它的截断提示只判断keptBytes<reportedBytes；4097个ASCII截成4096+…时ellipsis增加3字节，可能不出现云端提示。没有为修UI另加协议字段或改真实Cloud。

## 舍入与累计都不能凭直觉

PHP round(1.005,2)=1.01，甚至1.0049999999999997也是1.01。直接math.Round(x*100)/100会在边界错。Go仅实现本入口需要的非负两位路径，按[PHP8.3.35官方算法](https://github.com/php/php-src/blob/php-8.3.35/ext/standard/math.c#L116)作14位预舍入，并对12个真实PHP样本验证；不构造泛用PHP运行时。非常大且已无两位精度意义的有限值保原值，不因乘100溢出产生Inf。

首次保存raw/first_seen及threshold_ms；后续只更新last/last_bytes、last_ms、max_ms、at/context/request。count增加本次1+非负overflow，daily只计本次1，跨日重置为1。max_ms是已写快照的最大值：冻结期只持久计数overflow，不保存那期间的耗时峰值，不能说成所有发生的绝对峰值。

同hash被resolved/deleted后复发会重开open、清处置字段，绕过“新建容量”闸；没有新增本地Resolve/Delete API。测试只在自有目录种桶记录。max_open=0拒新建，daily_cap<=0不冻结，全部沿共同存储规则。

## journal与严格SQL YAML

两个固定stem分别为go-runtime-buffer和go-slow-query-buffer，Scope再隔离。相同12hex hash跨类型/Scope也不共享文件、容量、锁、overflow或账本。

最终图像intent包含固定hash/目标桶/完整Record或冻结null/最终overflow。恢复不调用Builder重新累计；状态必须等于目标桶，先写目标再清其它桶，坏intent或坏记录保原并报错。源PHP是近似并发计数与缓存overflow，Go继承05已批准的scope锁、持久overflow与journal恢复差异。

SQL codec必须区分0和缺失/null：sql/took/at/daily/context/request/meta必须是map，字段必在，非指针数值必须匹配标量tag；connection及明确nullable字符串才能null。未知字段/重复键拒绝。不能先typed decode再把sql:null、last_ms:null或at.line:null当合法零，随后Refresh覆盖坏文件。负例验证错误后原bytes/overflow保留、无新intent；0耗时、0门槛、line0、connection:null仍合法。

现Runtime codec及OrderedObject/二进制trace保持；SQL自己的文本保证UTF8，不把Runtime允许的截断trace规则塞进SQL。

## 同一个确认核心，两套typed wire

Runtime的Client.Send、Syncer及Snapshot具体API保留。SQL使用SlowClient.SendSlow、SlowSyncer和SlowSnapshot，内部共用同一typed确认协调器；没有把SQL塞进Runtime.Record。

HTTP固定POST api/v1/slow-queries/intake，body只有token和records；DiscardSlowLocalNoise固定slow-queries/discard-local，只传远端协议，不清本地pending。重试/TLS/取消/重定向/响应上限复用07同一个post，不另写网络栈。

SQL记录包括生命周期、sql/took/at/daily/context/request/meta；request五键无guard，context为env/project/occurred_at。没有exception/trace/payload/snippet，也不发Snapshot的digest/bucket包装。输入坏UTF8整批ErrWire且零请求。实测15键白名单与真实本机POST，不代表真实Cloud接收/数据库已验证。

确认仍使用存储内容SHA256+时间+桶；发送期间同毫秒count改变，旧ack不匹配。saved/filtered确认，retryable留洞，永久拒收同版本隔离并保安全原因；open始终保累计anchor，resolved才同版本删除。云端count和max_ms是累计快照max，重发不能sum。坏整批receipt不推进确认，记账失败至少一次，不声称exactly-once。

## 源证据与实际验证

[纯probe](../../tools/probe-slow-query-records.php)只require五个已审声明，拒额外autoload，固定config/base_path/clock，Reflection只运行纯hash/build/refresh/binding/截断。没有record/handle、Host、数据库、YAML写盘或HTTP。[原stdout fixture](../contracts/fixtures/slow-query-records.json)保14hash、8bindings、12round、5truncate和三轮完整记录；重放逐字相同、五SHA与源clean已核。

共享抽取后早轮受影响race：slowquery1.482s、buffer12.808s、cloudsync6.671s。其后SQL节点/冻结输入/row补强，只新SQL范围race最终1.498s/3.754s/2.351s；未重复整批旧项。旧Capture必要项0.587s、日志/HTTP消费者1.093s，Host41默认stack实际消费者0.856s。命令、阶段与首轮编译/夹具修复见计划最终记录。

测试只自有临时目录、fake transport及httptest；不全量/E2E，不接生产DB或真实Cloud。独立发布解析、自动Goravel SQL hook与完整monitor矩阵仍未完成。

## 自主练习（待用户完成）

1. 预测cap2五次后次日count/daily/max_ms，指出冻结期只保哪一种信息。
2. 对比原raw身份、双mask展示、字节长度与4096码点截断；解释为何不能声称PHP逐字等价。
3. 画出同毫秒新版本发送期间写入、旧ack拒绝和下一轮回扫。
4. 解释Runtime兼容外壳、SQL接受入口与共享私有主链各自解决的问题。
5. 指出显式ReportSlowQuery与自动数据库hook、真实Cloud验收的差别。

代理参考验证不代做学习验收。
