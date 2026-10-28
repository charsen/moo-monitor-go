# 第03课：递归预算、字符截断与输入边界

[上一课：02](02-sql-masker.md) · [课程目录](../README.md)

前两课处理单段文本，本课处理payload树。除了敏感键，还要明确深度、全树共享预算、字符串截断，以及未知Go类型如何失败；不能把整个请求对象交给JSON或Stringer就声称脱敏完成。

来源是[RuntimeErrorRecorder.maskRecursive](../../../moo-monitor-laravel/src/Recorder/RuntimeErrorRecorder.php)，范围见[本步计划](../plans/03-bounded-payload.md)。本课没有事件采集、存储、Host/Gin接线或Cloud。

## 先用受控方式取得私有方法证据

[probe](../../tools/probe-sensitive-masker.php)新增固定--payload模式。只require四个已审声明：SensitiveMasker、SafelyLogs、BucketedYamlRecorder、RuntimeErrorRecorder，额外autoload立即报错；未加载Composer或Host。

真实构造器传非nullconfig和假absolute basePath，短路config()/StorageScope路径解析，实际只赋字段与创建Masker。ReflectionMethod只调用私有maskRecursive，不调用record/build/extractPayload。假stdClass只含固定假属性，resource仅php://memory并在finally关闭，不产生外部文件。

[原始payload fixture](../contracts/fixtures/sensitive-masker-payload.json)保留28组真实stdout，包含源commit/文件SHA、PHP/mbstring、keys/cap、起始depth/budget、剩余budget与输入有序spec。没有手工填写预期，basic/sql两种原stdout也逐字cmp不变。

## 深度与预算是不同的限制

根depth为0；depth>5先返回<deep>。容器每个key先消耗1预算，初始全树5000；如果变负，就在当前容器写__truncated__:<too many keys>并停止。预算跨兄弟与嵌套分支共享，不是每层重新给5000。

敏感key也耗1，但整值直接***，不再访问其中对象或子树。非敏感key才递归depth+1；scalar根不耗key预算。这个顺序会影响depth5里的敏感键与普通键输出，不能只实现一个通用“超过五层截断”而忽略分支顺序。

源实证小预算会在内层、外层各留下marker，remaining可到-2；marker不是被当普通输入再脱敏一次。若原对象已含__truncated__，耗尽时覆盖原位置，不追加第二个同名键。

## 字符串先脱敏，再按codepoint截断

字符串只调用MaskSecrets，不自动调用MaskSensitiveSQL。处理后的字符数大于cap时，保留max(0,cap-20)个codepoint，再接…<+N chars>，N为被移除的字符数。

中文、emoji与组合字符按PHP mbstring/Go rune对应的codepoint数，不是字节，也不是用户看到的一整个字形。组合字符可能由多个codepoint组成，不能据此宣称所有grapheme都保持完整。

cap10的200个a得到…<+200 chars>；cap0也可只剩marker，负cap的空串按源得到…<+0 chars>。marker可能比cap长，cap不是严格输出总长上限。先脱敏再算长度意味着很长的Bearer假值可能缩成短文本而无需截断。

OpaqueObject/Resource输出<object:名称>/<resource:名称>，不序列化属性，也不再按字符串cap截这些新生成占位；cap0也保留完整类型标记。这是与普通输入字符串不同的分支。

## 原生map好用，但不能恢复PHP插入顺序

[payload.go](../../mask/payload.go)公开MaskPayload(value,maxStringChars)，可直接接map[string]any与[]any，不要求业务手拼树。SDK只在访问到当前map时排序其keys，再按共享budget走值，不先完整递归转换全树。

原生Go map没有可恢复的输入插入顺序；排序使输出和预算子集稳定，却可能与PHP按插入顺序截取的子集不同。需要精确保序时，可选OrderedObject([]Entry)或后续保序wire适配。这个差异是明确的Go调用边界，不能写成原生map与PHP原序完全等价。

普通数组保持[]；源预算耗尽时附字符串marker，会使数字索引数组变成对象，例如{"0":"a","1":"b","__truncated__":"<too many keys>"}。输出OrderedObject自定义MarshalJSON保顺序；空对象/空数组均按源的空PHP array输出[]。

对象key须有效UTF8且唯一，规范整数串key明确拒绝，以免暗中模拟PHP key coercion；需要数字索引应使用数组。输出内部因截断产生的数字key对象是另一条源结果路径，不代表输入已支持所有PHP数组键转换。

## 支持哪些Go值，错误时返回什么

当前标量支持nil、bool、int64、float64、string，float必须有限，文本必须有效UTF8。普通Go int尚不支持，常用map可以直接传，但其值不能是任意类型；例如数值写成int64(0)，或由未来采集适配统一转换。

任意struct、指针或Stringer不会被反射、调用String或遍历属性；可由显式OpaqueObject/Resource名称代表，否则遇到即error并无partial结果。不在错误里dump原值，不把失败变成“返回未脱敏原payload”。敏感整值或已被depth/budget截掉的分支不会继续检查其内容。

key结构预检与排序仍扫描当前容器全部keys，字符串脱敏也需读取当前字符串。budget限制访问/输出分支，不是整个输入分配、排序CPU或总耗时的硬上限；未来真实采集的类型/容量与M18失败隔离仍要另外设计。

## 调用链与实际验证

MaskPayload设depth0/budget5000 → 当前类型判断 → map当前层排序/有序对象预检 → key预算与敏感分支 → 递归/字符串/类型占位 → 成功输出或error。只有[测试](../../mask/payload_test.go)为源边界调用私有walk传小budget/depth，生产API不暴露任意起点。

在Monitor仓根审脚本后可重放源：

```sh
php -l tools/probe-sensitive-masker.php
php tools/probe-sensitive-masker.php ../moo-monitor-laravel/src/Recorder/SensitiveMasker.php --payload
```

源PHP8.3.35、mbstring=true，commit28d424e286222c391b7e64cfb5643e788dfcc456。专属28例含默认5000/5001，因此fixture约1.53MB，来自固定假输入与原stdout，不是生产数据。

独立定向命令：

```sh
GOWORK=off GOTOOLCHAIN=local "$HOME/Library/Caches/go-moo/toolchains/go1.27.3/bin/go" test -mod=readonly ./mask -run '^(TestPayloadFixture|TestPayloadInputBoundaries)$' -v
```

2026-10-29实际通过（0.881s）：源输出语义与剩余预算、depth、UTF8/小负cap、5000/5001、marker形状/碰撞与对象/resource占位一致；另核map不同插入顺序稳定、ordered保序、原map不被修改、非法类型/键/UTF8/float失败无partial且Stringer未调用。

首轮通过后，截取前缀改用UTF8位置扫描，避免为只保留短前缀而分配整串[]rune副本；同两项重跑得到上述最终结果。字符计数/脱敏仍扫描当前字符串，未把这个局部优化写成整体输入内存上限。

未修改既有New/三文本入口，不重复其回归；没有Go1.26最低版、Host/事件/存储/真实请求、DB、Cloud、race、全量或E2E。用户自主练习保持待完成，整个M07尚未完成任意请求表示与Host适配。

## 用户练习（待完成）

先手算一个父子对象用三key预算会在哪层留下marker，说明敏感整值为何不消耗子树预算。再比较数组耗尽、空对象、已存在__truncated__的JSON形状。

只用固定假值，预测中文/emoji、cap10与先脱敏后长度的结果。做一次修改定位练习：如果未来采集器收到普通int或自定义struct，列出类型适配、容量、失败隔离与fixture需确认的位置，不直接用fmt.Sprint或JSON序列化整个对象。

最后解释原生map排序与OrderedObject原序的预算差异，以及为何5000-key预算不等于SDK绝对CPU/内存上限。用户尚未亲手完成这些预测与复述，代理定向验证不代做学习验收。
