# 第02课：SQL诊断文本按列脱敏，不等于解析SQL

[上一课：01](01-sensitive-masker.md) · [课程目录](../README.md) · [下一课：03](03-bounded-payload.md)

[第01课](01-sensitive-masker.md)完成query与自由文本，本课补SQL值侧。目标是对真实源算法的当前行为建立可重放证据，不把有限正则写成完整SQL parser，更不把处理后的诊断文本交回数据库执行。

范围见[本步计划](../plans/02-sql-masker.md)，来源仍是[PHP SensitiveMasker](../../../moo-monitor-laravel/src/Recorder/SensitiveMasker.php)。本步不接Host、数据库或Cloud，也不混入下一步递归payload。

## 两阶段各解决什么

MaskSensitiveSQL先匹配“敏感列 比较符 字面值”，把值替为***；再识别INSERT列清单与values tuple，按列的位置替对应值。两种形式需要不同逻辑：WHERE里列和值相邻，INSERT里只有先对应列序号才能知道哪个值敏感。

比较符保留源的=、!=、<>、>=、<=、>、<、LIKE、IN；字面量支持当前引号/反斜杠形态、括号串与非负数字片段。列名按配置子串、ASCII大小写不敏感，反引号、双引号、bracket与限定列按源有限pattern处理。

动态SQL比较pattern在New中用regexp.Compile构造，错误返回，不让运行配置MustCompile panic。实例仍不可变，沿第01课有效UTF8配置边界与原串byte索引替换，不扩大Unicode casefold。

## INSERT要保留列和值的位置

INSERT阶段先拆列清单，对每个列标识符取最后点段、按源ASCII trim移除外层引号/bracket，再用ShouldMaskKey决定敏感位置。每个匹配tuple按同样quote/backslash状态机拆值，只替存在的对应位置，普通列值保留。

引号中的逗号不能直接strings.Split，反斜杠转义也要保持quote状态；这是真实需要的一段局部解析，不为此引入全SQL语法依赖。源算法会把被处理tuple重组为逗号加空格，fixture也记录这类格式变化，不能只比“秘密消失了”而忽略其余输出。

多个tuple都按同一列位置处理；列数和值数不一致等奇怪输入沿源行为保留，不在脱敏器里新增数据库合法性判断。SQL合法与否应由原业务数据库链负责，Monitor不能改变查询。

## 与MaskSecrets保持两个明确入口

MaskSensitiveSQL不会自动再调用MaskSecrets。源Recorder在需要的记录边界显式组合SQL与自由文本脱敏；本课只复现这个SQL方法自身，note='Bearer fake-token'仍可能原样保留。

这样调用者能看清选了哪种规则，而不是以为一次SQL处理已经覆盖所有JWT、Bearer或任意原payload。未来事件采集必须按实际字段组合适用入口，并在落盘前守住原内容不泄露的边界；当前没有这条Host采集链。

## 用真实fixture公开盲区

[SQL fixture](../contracts/fixtures/sensitive-masker-sql.json)包含30个固定假样本：全部比较符、LIKE/IN、单双引号/反斜杠、数字、限定/反引号/bracket、多个tuple、普通值含逗号/引号、敏感列不同位置、空keys/无匹配、占位符与已知语法盲区。

源输出中password=-42、password=NULL和password=fake保留原样。嵌套函数也不安全完整：INSERT concat样本实际得到 `values ('ordinary', ***, 'value'))`，嵌套IN样本得到 `IN ***)`。这些只是原有限pattern的诊断文本结果，不是可执行SQL，也不是秘密已全部清除的证明。

Go当前30例与源相同，不应擅自修改原fixture把这些结果美化掉；若未来决定更强脱敏，需要明确兼容/安全方案和新证据。当前一致性只对已覆盖样本成立，不是全SQL安全证明。

## 文件与实际调用链

[masker.go](../../mask/masker.go)的New增加SQL配置pattern；[sql.go](../../mask/sql.go)的MaskSensitiveSQL顺序执行比较替换→INSERT列位置→tuple重组。splitSQLList、cleanSQLIdentifier与ASCII trim分别承载源真实状态机和标识符规则，不建立通用Pipeline。

[sql_test.go](../../mask/sql_test.go)逐项读取原始SQL fixture调用对应方法；[probe](../../tools/probe-sensitive-masker.php)新增固定--sql模式，默认仍生成原basic。无SQL连接、执行或数据库语法解析依赖，go.mod保持纯标准库。

## 操作与实际验证

在Monitor仓根先审脚本与精确源文件，再运行：

```sh
php -l tools/probe-sensitive-masker.php
php tools/probe-sensitive-masker.php ../moo-monitor-laravel/src/Recorder/SensitiveMasker.php --sql
```

脚本只require原类，固定假输入，stdout原样保存；source commit为28d424e286222c391b7e64cfb5643e788dfcc456，PHP8.3.35，源文件SHA包含在fixture。默认无--sql的stdout已与原basic fixture逐字节cmp一致，未重写第01课预期。

独立定向命令：

```sh
GOWORK=off GOTOOLCHAIN=local "$HOME/Library/Caches/go-moo/toolchains/go1.27.3/bin/go" test -mod=readonly ./mask -run '^(TestSQLSourceFixture|TestSourceFixture|TestConfiguration)$' -v
```

2026-10-29实际通过（1.183s），新SQL30例与受New变化影响的原basic31例/配置检查通过；只有这三个定向入口。未实际运行Go1.26最低版，没有DB、Host、Cloud、race、全量或E2E。

## 用户练习（待完成）

只用固定假值，先预测WHERE敏感列、INSERT第二列和普通note值各走哪阶段，再对fixture核对。解释普通值含逗号为什么不能直接Split，以及quote/backslash状态在哪个文件维护。

做一次修改定位练习：如果以后想支持负数字面量，先指出当前源为何留gap，列源契约、纯probe、新fixture、Go pattern与安全调用边界需要怎样一起审定；不要只扩大一个正则就宣布支持完整SQL。

最后解释为什么MaskSensitiveSQL不暗中调用MaskSecrets，为什么30例一致仍不能把任意SQL原文写入日志。用户尚未亲手完成预测与复述，代理验证不代做学习验收。
