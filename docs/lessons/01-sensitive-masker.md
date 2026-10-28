# 第01课：用真实假输入对照建立基础脱敏边界

[课程目录](../README.md)

本课是独立Monitor的第一段Go实现，先做M07中的基础纯函数：敏感键判断、URL query与自由文本规则。没有采集请求、保存缓冲、接Host或连接Cloud，也不依赖scaffold。

前置是[完整复现计划](../port-plan.md)与[本步设计](../plans/01-sensitive-masker.md)。用户自主练习尚未完成，代理源fixture和定向测试不等于学习验收。

## 先用源执行结果约束实现

PHP来源是[SensitiveMasker](../../../moo-monitor-laravel/src/Recorder/SensitiveMasker.php)。[局部probe](../../tools/probe-sensitive-masker.php)只require这一个精确类文件，使用固定假值，记录keys、method、input、output、源commit、文件SHA与PHP版本；[fixture](../contracts/fixtures/sensitive-masker-basic.json)是实际stdout，不手写预期。

没有加载Composer、Host、.env或数据库Provider，也没有访问外部云。脚本只用只读git获取源reference，当前源commit为28d424e286222c391b7e64cfb5643e788dfcc456，PHP8.3.35。fixture表示选定纯类方法的真实行为，不是完整Host监控链实测。

## New负责配置所有权和失败边界

[masker.go](../../mask/masker.go)提供New(keys)(*Masker,error)，复制输入keys，不把调用者slice保存在可变共享状态里；过滤空字符串和字符串0，保留PHP array_filter对应行为。后续改原slice不会改变已构造实例。

Go配置限定有效UTF8 keys，非法UTF8返回明确error；PHP bytepattern能表达更宽输入，本课不冒称完全覆盖。动态配置pattern用regexp.Compile处理错误，不用MustCompile把坏配置变成panic。真正consumer以后要按M18隔离初始化失败，当前没有接Host，不能忽略err继续调用nil实例。

敏感键是大小写不敏感子串匹配，例如token命中ACCESS_TOKEN，但不是Unicode全折叠。实现只lower ASCII匹配，并用原串byte位置保留捕获前缀和未命中文字；避免Go的(?i)额外扩大Unicode匹配。固定Ä/ä用例保护这一边界。

## URL只负责query中的值

MaskURL不使用url.Values重编码或排序。它保留原始query顺序、重复键和原键编码，键经容忍坏percent的urldecode后判断，命中且有等号才把值替成***；+在匹配键中按空格解释。

比如源fixture中的%74oken与TOKEN都可命中，但输出键仍保持原样；无等号的token条目保留，空query段按源折叠。scheme、host、path、userinfo和fragment不是这个方法的脱敏范围，不能因方法叫MaskURL就认为整条URL所有敏感内容都被清除。

fixture专门保留fragment/userinfo中的假值，公开证明缺口。后续若需要更强策略，应明确接口和兼容决定，不能把保留的假值写成已经安全处理。

## 自由文本规则有明确顺序

MaskSecrets按源顺序处理Authorization头值、JWT、Bearer，再处理配置键的key:value或key=value。Authorization捕获前缀保留，JWT变***JWT***，Bearer输出Bearer ***；已有***不被错误扩成其他明文。

空mask_keys只取消配置键判断，不关闭固有Authorization/JWT/Bearer规则。普通中文文本保持原样，正则特殊字符键会转义；vertical-tab空白也按源规则处理。它是有限pattern集合，无标签自由文本中的假敏感值可能原样保留，不能承诺任意原payload经此处理后都可落日志。

最后两组纯源case专门锁定交互与缺口：Bearer加JWT得到 `Bearer ***JWT***`，因为JWT阶段先替换后Bearer pattern不再吞掉星号；`password: fake first second`得到 `password: *** first second`，多词后半仍保留。Go与该实际源输出一致，不能为了看起来遮得更多而篡改fixture，也不能把一致性当成全面安全保证。

SQL需要专门值侧规则，递归payload需要深度/节点/截断预算，本课都未实现。监控后置采集也不能撤回宿主已经写出的raw日志，Goravel/Gin坏JSON原body问题仍需在记录边界另行设计，不修改模块cache来隐藏问题。

## 调用链与文件职责

调用者处理New错误 → 持有不可变Masker → 选择ShouldMaskKey/MaskURL/MaskSecrets → 得到纯函数结果。实例没有请求、数据库、日志writer或后台goroutine状态，方法不会替Host决定事件采集和持久化。

[masker_test.go](../../mask/masker_test.go)读取源fixture逐case调用对应公开方法，比较bool/string实际结果，另检查配置拷贝和非法UTF8拒绝。lowerASCII、原串捕获替换、query键解码分别解决真实ASCII/位置/编码适配，不建立通用Pipeline或完整PHP运行时。

## 操作与实际验证

在Monitor仓根，先审脚本和精确源文件，再复现纯类输出：

```sh
php -l tools/probe-sensitive-masker.php
php tools/probe-sensitive-masker.php ../moo-monitor-laravel/src/Recorder/SensitiveMasker.php
```

脚本只向stdout输出；更新fixture须保留真实stdout并审diff，不手改expected。实际已运行go mod init后设go1.26.0，标准库无require，不加入Host workspace。在仓根用隔离工具链定向测试：

```sh
GOWORK=off GOTOOLCHAIN=local "$HOME/Library/Caches/go-moo/toolchains/go1.27.3/bin/go" test -mod=readonly ./mask -run '^(TestSourceFixture|TestConfiguration)$' -v
```

2026-10-29最终通过（0.810s），31个源fixture场景与配置测试覆盖上述边界；两组交互/gap case补齐后只重跑既定两项，未扩成其他测试。两Go文件gofmt -d无差异、diff空白检查通过。没有跑Go1.26最低版、SQL/递归/Host事件、真实日志、Cloud、全量或E2E；纯函数没有并发状态，本步未加race。

## 用户练习（待完成）

先预测空keys时ShouldMaskKey(token)与MaskSecrets(Bearer假值)为什么不同，再从fixture找依据。比较query编码键、裸无等号键与fragment的结果，说明哪些信息根本不在本方法范围内。

做一次修改定位练习：假设未来增加一个需脱敏的配置键，列出调用方配置、纯类probe、源/Go fixture与边界测试的位置；不把任何真实token或业务payload放进练习。最后解释为什么坏配置应返回error、为什么初始化错误仍要由未来Host接入按M18隔离。

隔天只用固定假值重画三入口的职责与缺口，说明本课完成哪些M07子项，以及SQL/递归/事件/原日志预防仍缺什么。
