# 第11步：显式慢SQL、共享持久聚合与Cloud协议

仅monitor；Host41已提交e72093f、monitor10已提交4df5f02。本步不接Goravel数据库自动hook、不执行SQL、不连真实Cloud。布局与纯probe已审定，独立slowquery与共享存储/同步已落，实际验证见下方。

## 实际源与明确安全差异

SqlSlowListener按took>=threshold采集，skip_patterns为raw SQL大小写敏感子串；bindings按每个?顺序拼接而非SQL语法分析，DateTime到秒，字符串先反斜杠再单引号转义，bool1/0/nullNULL。记录hash使用原raw：空白压空格→IN占位列表压(?)→数字N→8192字节截→trim，再MD5(relfile:line|sql)前12。

SqlSlowRecorder build/refresh保首次raw/first_seen、更新last与last_ms、max_ms取大、累计count/daily并回填overflow；复发重开/容量/cap与runtime共用基类。CLI request五键无guard，context只有env/project/occurred_at，没有伪PHP版本。

root批准安全差异：hash仍用原raw按源归一；持久raw和last都先SQL值侧→Secrets脱敏，再以4096 Unicode码点+…展示截断。raw_bytes/last_bytes明确定义为脱敏后、截断前UTF8字节数，与Cloud真实计量匹配，不另加原文长度。输入raw/生成last非法UTF8明确error无输出；hash内部8192切半字节仍可MD5，probe用base64保证据。源raw未脱敏、mb字符计数及字符/字节夹区截坏UTF8保fixture作差异，不冒称逐字等价。

耗时非负有限；PHP round2的1.005/2.675/边界先probe，不能无证假定math.Round完全一致。

## 公共API与兼容布局（已审定）

新增slowquery包：typed Input包含Raw、窄类型Bindings、TookMillis、File/Line、Connection与显式Request；Config包含阈值/skip/mask/path/env/project。SlowBuffer.ReportSlowQuery显式入口消费typed Input和now，过滤后进入共同私有store.record；Builder纯构造/刷新/hash，不采栈/请求全局/数据库。Bindings仅支持明确原生标量与time.Time，未知Stringer/对象不调用；精确数值转换以probe决定。ThresholdMillis为*float64，nil默认100、显式0保留；拒NaN/Inf/负值及不能表达Record整数的范围。过滤用原float（100.49<100.5），记录threshold_ms按源整数截取（100.5→100），不偷丢源已核浮点监听边界。

公共存储采用buffer包内私有typed store[R,S,Row]，不新增可注册的通用pipeline。现Buffer具体外壳嵌入store[records.Record,records.Snapshot,Row]保Record/Get/List/Count/Snapshot/Compare/ScopeRoot/Close消费者；TagSource只留Runtime外壳，不让SQL暴露它。新增SlowBuffer嵌入slowquery的具体实例，公开SlowRow/SlowSnapshot。

两真实record族的family实现只处理Hash/Build/Refresh、typed YAML Encode/Decode、Lifecycle读取/写回与DeriveRow。Lifecycle只含hash/status/resolved、count/daily、first/last与meta.updated_at，不含SQL或异常业务。公共主Record顺序仍可直接读：gate→scope锁→恢复→读取→容量/cap/复发→Builder最终图像→intent→目标/清旧/overflow→清intent。固定type stem分别go-runtime-buffer与go-slow-query-buffer，类型/Scope隔离，不开放任意路径kind。

files/lock/atomicWrite、严格journal presence/kind/status==target、恢复与Compare隔离逻辑保一份。Runtime codec保既有OrderedObject/二进制trace，Slow codec只接受当前typed schema，坏/未知字段原文件保留。恢复重放固定最终图像，不调用Refresh二次累计。现包内故障注入位置保留，仅内部测试可设，不暴露产品开关。

Snapshot用typed公共别名兼容现runtime字段访问，内部可使用VersionedSnapshot[R]承载Hash/Bucket/Timestamp/Digest/Record。现Snapshot仍具体runtime实例，新SlowSnapshot为SQL实例，不能把SQL填进records.Record。

Cloud同一个私有typed确认协调核心，runtime Syncer和新SlowSyncer只作为真实消费者兼容外壳，保06指纹账本/水位/洞/隔离/锁逻辑一份。现Client.Send(runtime snapshots)保持；新SlowClient.SendSlow(slow snapshots)，HTTPClient提供两个明确typed入口，各自白名单编码与固定路径，共用07网络发送/重试/安全错误/响应限制。慢SQLintake=api/v1/slow-queries/intake，discard=api/v1/slow-queries/discard-local；不自动协调本地删除或真实推送。

Cloud按(project,hash)累计快照max(count/max_ms)，以meta.updated_at/last_seen保护旧状态；不能重试sum。SQL协议只sql/took/at/context/request+生命周期/meta，无runtime trace/payload/snippet。

## 文件职责与验证闭包

新增slowquery/{types,builder,bindings}.go及定向fixture测试；新增buffer/slow.go与SQL codec/row adapter，把现通用I/O/聚合/恢复迁到私有typed store，不复制目录树。cloudsync提同一typed协调器、新增slow typed client/wire，现HTTP网络策略不另写。具体文件名可按真实职责合并，避免每小方法一层。

必要旧消费者：Runtime Buffer所有受影响聚合/codec/恢复/Compare/跨进程案例；06 runtime receipt/state/同步与07 HTTP wire/重试；08 capture/09 HTTP与Host41默认stack/私有Buffer的必要实际消费。只受影响包定向，不全仓/E2E/真实Cloud。新SQL覆盖源hash/build/refresh/bindings、批准安全差异、阈值/skip、max耗时、cap/overflow/复发/类型scope隔离、journal阶段恢复、同ms新版本与部分回执/SQL真实httptest POST。实际并发才一次定向race。

## PHP纯probe机制

tools/probe-slow-query-records.php仅接受已核SqlSlowRecorder源路径；精确require SafelyLogs/SensitiveMasker/BucketedYamlRecorder/SqlSlowRecorder/SqlSlowListener五声明并拒任何额外autoload。显式never-written basePath/config，固定base_path和只返回app.env/app.name的config helper、固定nowIso；Reflection只normalizeSQL/hash/build/refresh/truncate/strLen与Listener.fillBindings/formatBinding，不record/handle/Host/YAML/DB/HTTP/写盘。Date binding标准DateTime固定值，坏字节base64，raw stdout保source commit/SHA/PHP与mb状态，不手写期望。脚本已先审后真实执行，结果与重放见下方。用户练习待完成。


## 实际验证与阶段（2026-10-30）

真实纯PHP源28d424e/PHP8.3.35、mb=true/precision14；raw fixture为[slow-query-records.json](../contracts/fixtures/slow-query-records.json)，14hash/8bindings/12round/5truncate及build/两轮refresh。php-l、无参exit2、五源码SHA和原stdout重放逐字cmp通过，源仓clean。官方PHP8.3.35 math.c正数两位14位预舍入用于边界实现；不以数学直觉代替证据。SourceFixture/Safety首轮0.411s，浮点门槛补项0.311s；Config指针类型编辑及泛型机械替换有编译错误，修正后才记通过。

抽取后旧Runtime Tag/恢复/坏数据三项2.026s；原receipt源/部分确认/同毫秒/HTTP POST四项0.828s。共享包受影响范围早轮race：slowquery1.482s、buffer12.808s、cloudsync6.671s，含旧Runtime聚合/codec/并发/子进程/恢复/确认/传输；这是SQL递归节点、冻结坏binding与row补强前证据。

设计审发现：typed Decode把缺/null数值变零；SQLRow160字节可能切坏中文；冻结分支跳过Builder会错误计入坏binding。已分别补固定SQL节点kind/tag/presence、SQL row退UTF8边界、SQL接受边界验证已消费binding。Runtime codec/row不顺改。共享主动作私有，Runtime仅一个现有Record兼容桥，Slow不暴露无过滤Record。最初SQL记账故障测试用错钩子名，改真实replace-state第2次替换后，明确断errors.Is及保open anchor/重试；不把未触发注入当恢复通过。

最终仅新SQL范围race PASS：slowquery1.498s、buffer3.754s、cloudsync2.351s。包括100.5门槛/12舍入样本/批准安全差异、cap2五次→次日count6/daily1（冻结峰值不保）、两桶复发、type/scope同hash隔离、五提交阶段恢复、坏节点保原/overflow7/无intent、合法0/connection:null、冻结坏binding不计、row159ASCII+中文、两实例12并发不丢计、四回执/同ms新版本/失败洞/重开/原anchor、实际本机SQL POST/discard/白名单15键/坏UTF8零请求。

```sh
GOWORK=off GOTOOLCHAIN=local go test -mod=readonly -race ./slowquery ./buffer ./cloudsync -run '^(TestSlow.*)$' -count=1 -timeout=90s
```

原实际消费者必要回归：capture三项PASS0.587s、Writer/HTTP五项PASS1.093s；Host41原default stack私有Buffer真实bootstrap子项PASS0.856s，沿41受控环境/relative LOG_PATH命令，仅-run '^TestMonitorRuntime$/^stack$'。Host无源码改动，workspace仍四模块；SDK测试GOWORK=off、实际Go1.27.3、Go下限1.26未改。未把Host工作区测试当独立发布解析。

未全仓测试/E2E、未执行SQL/生产DB/真实Cloud。自动数据库hook、完整监控矩阵及发布解析待后续；Cloud截断提示可能因ellipsis字节而漏极短尾差，未另造协议字段。用户练习待完成。
