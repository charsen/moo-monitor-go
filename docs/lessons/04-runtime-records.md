# 第04课：从显式事件快照构建运行时记录

[上一课：03](03-bounded-payload.md) · [课程目录](../README.md)

前三课建立脱敏组件，本课把它们放进一次真实的记录构建流程：接收快照、准备末次字段、计算身份、建立或刷新累计记录。只实现纯内存Builder，采集来源、桶存储、并发聚合和上传会分步接入。范围见[本步计划](../plans/04-runtime-records.md)。

## 先把采集事实与记录规则分开

[records.go](../../records/records.go)中的Snapshot是显式输入，不接受任意error后自动扫描环境。它包含Exception、Request、Context、Trace、Snippet、Payload、Source、Meta：

| 输入 | 调用方提供 | Builder负责 |
| --- | --- | --- |
| Exception | class/code/message/file/line、previous | message双mask、相对路径、previous最多三层 |
| Request | method/url/ip/user_id/user_name/guard | 复制指针字段、URL按源规则mask |
| GoContext | env/project/go/goravel | occurred_at统一为本次时间 |
| Trace | Full文本和原始Frames | 双mask文本后字节截断、前缀选帧最多20 |
| Snippet | file/line/language/code | 相对路径；code按源保留 |
| Payload | 03支持的值类型 | 写方法才访问，深度/共享预算/字符截断 |
| Source/Meta | 采集来源与标量诊断 | 来源优先级、合并、重脱敏与过滤 |

Builder不读取请求全局、不调用runtime栈、不读取源码文件、不访问磁盘/云。后续采集器负责把真实Go事件转换到这个边界；typed输入存在不代表自动采集已经完成。

GoContext使用真实的go/goravel字段；与PHP版本字符串没有冒充关系。环境和项目名由调用方明确提供，occurred_at不能由输入绕过本次时间。CloudSync原样转发record的源码只能证明字段能被SDK承载，真实Cloud接收Go元信息仍待兼容验证。

## 调用链保持一个动作读到底

New(Config)构建不可变Masker并复制prefix配置。TraceMaxBytes与StringMaxChars为可选指针：nil取65536字节/200字符；非nil保留显式0、负字符cap等源边界。Trace负值沿PHP substr负长度，即去掉末尾对应字节后加marker；不是推荐生产配置。

Build(snapshot,now)先prepare：按Request.Method筛payload → metadata → exception/previous → request → trace → snippet/context，任一已验证边界错误就返回nil/error，无半条记录。之后按原始异常身份Hash，再设置first/last_seen、count=1、daily=1、status=open和三个resolved字段null。这里没有Repository或隐式Pipeline。

Refresh(existing,snapshot,now,overflow)检查int64累计范围，准备新的末次字段，保留hash、first_seen和status/resolved信息。它不重算已有hash、不隐式重开resolved、不写文件。旧Record的map/字符串指针不会被修改；调用方仍需管理聚合时同一身份/并发读写归属，本步不提供事务或并发累计保证。

所有时间都来自本次一个time.Time；输出毫秒ISO格式，UTC写+00:00，daily取该带时区时间的日期。外部采集发生时间和调用构建时间的选择，由未来采集适配明确；本步不悄悄读取time.Now。

## hash到底使用什么

PHP [RuntimeErrorRecorder](../../../moo-monitor-laravel/src/Recorder/RuntimeErrorRecorder.php)直接构造：

```text
class + "|" + relPath(file) + ":" + line + "|" + normalize(message)
```

再MD5取前12个小写hex字符。没有JSON编码、对象键排序、时间、code、previous、请求、payload或meta参与。MD5在这里是源聚合身份契约，不是安全签名或无碰撞保证。

normalize严格按顺序：SQL值mask → Secrets → 单引号内容换'X' → 双引号内容换"X" → UUID换U → 0x地址换0xH → 长hex换H → ASCII数字串换N → 前1024字节。不能为了“更完整”换SQL parser，也不能把数字步骤提前。0xH中的0最后仍变N，因此真实归一结果包含NxH。

最后一步按字节，不按字符；342个“中”会截在UTF8字符中间。Hash仍对原始1024字节计算；probe用base64保存归一输出，避免先JSON替换非法UTF8导致hash证据失真。超过1024字节的相同前缀也可能被归成同一身份。

Payload只在POST/PUT/PATCH/DELETE（方法转大写）访问；GET/CLI/HEAD等直接[]，连不支持的Stringer也不遍历。写方法缺失nil也按源request.all空数组归[]。这条规则集中在Builder，后续采集者无需各写一份。

relPath沿[基类](../../../moo-monitor-laravel/src/Recorder/BucketedYamlRecorder.php)的纯字符串前缀删除：显式base先去末尾/，配置base=/fixture时，/fixture/app/x.go变app/x.go，/fixturesuffix/x.go也会删除前缀，得到suffix/x.go；不加目录段边界，不执行realpath或filepath.Clean。BasePath为空时Go保留原路径，相当于没有Host base_path上下文；显式/经rtrim后是空前缀，因此移除路径开头/。3组额外纯源path样本覆盖尾slash、前缀碰撞与根路径。

13组源fixture证明相同显式class/file/line/message得到相同hash；code变化不改变hash，class/line/path变化可改变。Go原生错误类型名和栈位置与PHP自然不同，不能据此承诺两个SDK的所有事件自动合并，也不能为合并擅改算法。

## 累计记录与末次快照

新记录count=1、daily={当日,1}。同日刷新时count增加1+max(0,overflow)，daily只增加1；跨日daily重置1。fixture验证count 1→5→6，对应overflow3与负overflow，daily是1→2→1。

first_seen保留，last_seen更新；exception/request/context/trace/snippet/payload由本次覆盖。resolved_at/by/note和status保持现值。PHP外层persistAggregated会搬桶/重开，这属于后续持久化块，不能把refresh本身写成已完成重开生命周期。

Go用int64累计，越界返回error无输出；PHP极值可能变float，当前不模拟其类型漂移。每日cap、overflow缓存的真实产生与恢复、重启累计、多进程一致性都尚未实现。

## 来源优先级与metadata合并

SourceRank只有一份，后续dispatcher复用：queue_failed30、schedule_exit28、http_5xx25、log_context20、log_message15、reportable10、未知0。source先把非ASCII字母/数字/_/-变_，小写并去两端_/-，空则reportable，最多64字符；原字符串恰为"0"时先因PHP ?:假值回退reportable，单独源样本已覆盖。

incoming优先级不低于旧source才替换，否则保留旧primary。sources按“旧sources、旧primary、新source”顺序去重，保留见过的来源；meta里传入source/sources不能覆盖这个权威结果。

普通metadata先旧后新合并，旧值同样重新脱敏。只保标量/null分支，嵌套容器不作为payload递归；敏感键scalar/null直接***。字符串先SQL再Secrets，再截500个codepoint；空串/null过滤，数值0和false保留。保留的旧updated_at若存在也只是旧标量；本步不生成新的updated_at，它在源writeFile层才写。

Go支持metadata string/bool/int/int64/float64/nil；int转int64，float须有限。legacy sources须字符串/字符串列表；非法UTF8 metadata字符串、非法sources或非有限float返回error。其他整数宽度/复杂对象未作通用适配，nested忽略；Payload仍遵守03的更严格类型边界，普通int在Payload内仍不支持。

## 输出不意味着所有字段都可公开

Exception/previous、Trace.Full和字符串Meta用SQL→Secrets双mask，trace必须先mask再字节截，避免先截断凭据后使pattern失效。Frame按配置前缀筛选，最多20，function去两端冒号；Builder不自己推断哪些Go包是业务包。

Request的身份字段按源保留，URL只用有限MaskURL；其fragment/userinfo等盲区沿01公开。Snippet.Code按源保持原文，不伪造一层未经验证的源码mask，不能据此安全上传任意源文件。未来真实采集/上传必须审文件范围和敏感内容策略，可默认关闭snippet采集。当前假源码路径从未被读取，不能把本课当作已获得读取业务源码的授权。

Hash的1024字节截断与trace的字节截断都可能生成非法UTF8字节；本步保源记录/指纹行为，未来持久化或网络编码必须明确处理方式，不能假称JSON传输已经验证。GoContext版本字段也未经过真实Cloud兼容测试。

## 如何复现源证据与定向验证

[probe](../../tools/probe-runtime-records.php)只require四个已审声明，额外autoload拒绝。真实ctor使用非nullconfig与假basePath，固定异常class/file/line、固定base_path字符串helper和nowIso子类。仅反射hash/meta/build/refresh；null Request走CLI；不存在的假file仅做stat，不执行record/存储/Host/DB/Cloud。

在Monitor根执行已审命令：

```sh
php -l tools/probe-runtime-records.php
php tools/probe-runtime-records.php ../moo-monitor-laravel/src/Recorder/RuntimeErrorRecorder.php
GOWORK=off GOTOOLCHAIN=local "$HOME/Library/Caches/go-moo/toolchains/go1.27.3/bin/go" test -mod=readonly ./records -run '^(TestRuntimeSourceFixture|TestRuntimeSnapshotBoundaries)$' -count=1
```

[原始fixture](../contracts/fixtures/runtime-records.json)保留真实stdout、源commit/四文件SHA/PHP8.3.35、13hash/3path/7meta与三个完整记录。[测试](../../records/records_test.go)不手填源expected：hash/base64精确比较；完整记录仅把来源Context中的php/laravel明确换成测试提供的go/goravel后逐字段比较，原fixture不改。Trace.Full的源完整样本为#0 {main}；含凭据trace/帧筛选等另由显式Snapshot定向测试，不混称真实Go采栈结果。

2026-10-29，Go1.27.3/GOWORK=off本块两项最终通过（1.981s），包括GET/CLI不遍历payload、POST实际脱敏、nil写payload、显式根路径与省略base、负trace cap、旧记录不变和累计越界。初验通过后集中补这些受影响边界，仅重跑同两项。初轮失败是测试读取PHP空array为map、以及JSON默认HTML转义后的字符串断言；修测试表示后通过，不修改业务掩盖差异。本课没有全量/race/E2E、真实Host/Gin/Cloud、磁盘缓冲或最低Go1.26运行验证。

## 用户练习（待完成）

先预测一个带UUID、0x地址、数字与token的message归一顺序，再说明code/请求/时间为何不进hash。比较同日overflow3和跨日负overflow的count/daily，解释resolved为何暂不重开。

自行构造一个显式Snapshot，说明哪些数据由采集者提供、哪些由Builder处理；指出Snippet原文与URL有限算法的风险，以及int在Meta和Payload中当前不同的支持范围。代理fixture通过不代替这些练习。
