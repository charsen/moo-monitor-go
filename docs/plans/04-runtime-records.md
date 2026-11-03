# 第04步：M02运行时记录构建与刷新

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

本步把显式Snapshot变成typed Record，含脱敏、稳定hash、累计刷新与source优先级；不采集真实请求/栈/源码，不接record落盘、三桶、Dispatcher、Gin或Cloud。职责和调用链见[第04课](../lessons/04-runtime-records.md)。

## 已确认方案

[records](../../records/records.go)独立消费[mask](../../mask/masker.go)，不依赖Host/scaffold。New复制配置，Build(snapshot,now)与Refresh(existing,snapshot,now,overflow)按业务顺序准备末次字段，再设置身份/累计；每次只用调用方提供的一个time.Time。SourceRank为后续dispatcher唯一优先级真值。

Snapshot明确提供Exception含至多保留三层previous、Request、GoContext、Trace.Full/Frames、Snippet、Payload、Source与scalar Meta。Builder不读取runtime栈/任意文件/框架全局。Context是env/project/go/goravel/occurred_at，不能填假php/laravel；源fixture比较时只单独转换Context版本字段，原始PHP输出保留不改。

hash采用源class|relPath(file):line|normalize(message)字节拼接MD5前12。先SQL→Secrets→单引号→双引号→UUID→0x→长hex→数字→1024字节；不使用JSON，不含code、previous、时间、payload/meta或键排序。0xH里的0还会被末步变N。relPath对显式base先rtrim再保源纯前缀行为（显式/与未提供base不同），不擅自realpath/clean或加目录段判断；同一显式身份可跨语言一致，Go原生error类别/位置通常不同，不宣称事件自动合并。

Build count/daily=1/open、resolved字段null；Refresh保hash/first_seen/status/resolved，累计count+=1+max(0,overflow)，daily同日+1/跨日本次1，不把overflow加daily。重开是未来桶层行为；meta.updated_at是未来writeFile行为，本步不伪补。int64累计溢出返回error无输出，和PHP极值类型行为不同。

Exception message/previous、Trace.Full、字符串Meta先SQL再Secrets；trace先mask再按字节截，帧按配置前缀选至多20；Payload仅POST/PUT/PATCH/DELETE访问，其他方法直接[]；写方法nil也[]，实际访问时复用03。Request只按源MaskURL处理url，身份字段来自外部采集者。Snippet.Code按源保原文，不能视为任意源码可安全上传，后续必须审文件范围/敏感策略，可默认关闭采集。

metadata新值覆盖旧值，旧标量也重脱敏；嵌套值忽略，空串/null过滤但0/false保留，敏感键scalar/null变***，字符串mask后500codepoint。source规范化、64字符；source只升优先级、同rank后者胜；sources保旧顺序去重并集，不受传入meta.source/sources覆盖。Go metadata支持int/int64/float64/bool/string/nil；其他整数宽度等并未泛化适配，legacy sources要求字符串，非法UTF8字符串/非有限float拒绝。Payload类型边界仍沿03。

## 纯源证据与验收

[probe](../../tools/probe-runtime-records.php)仅四个已审声明、拒额外autoload；显式config/basePath、假Exception file/line、固定base_path纯字符串helper与nowIso子类。仅Reflection hash/meta/build/refresh，Request=null；不存在的假源码路径只stat，无record/存储/Host/DB/Cloud。原始[fixture](../contracts/fixtures/runtime-records.json)含source commit/4文件SHA/PHP版本、13hash/3path/7meta及3个完整记录；截断后可能非法UTF8的归一化文本以base64保字节。

Go比较完整记录时仅明确替换Context的php/laravel为测试显式go/goravel，其他字段逐项语义比较；hash和归一化base64逐字比较。独立边界覆盖外部请求URL/trace/帧/previous/payload、配置拷贝、旧记录不变、错误无partial与计数溢出。2026-10-29，Go1.27.3/GOWORK=off本块两项最终PASS1.981s；初轮测试适配修复PHP空[]与map表示、JSON HTML转义断言，不改业务迎合预期。

纯SDK构建不等于自动采集、M18全链失败隔离或Cloud已兼容；CloudSync当前原样承载record仅是源码证据，真实接收端仍待验证。用户练习待完成。
