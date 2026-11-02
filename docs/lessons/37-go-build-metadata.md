# 第37课：报告实际构建元信息

[上一课：36](36-log-capture-policy.md) · [课程目录](../README.md)

[计划](../plans/37-go-build-metadata.md)把版本来源集中在buildmeta，不用硬编码平台版本或把VCS提交号冒充发布版本。

## 从BuildInfo到三个安全字段

Current读取Go标准runtime/debug.ReadBuildInfo，Resolve是可确定性验证的纯入口。按精确module path在Main或Deps找SDK和Goravel；模块未贡献到此binary时可以缺失，返回unknown。找到但空或(devel)表示development；本地Replace或不同模块身份的Replace也只标development，不能继续声称被替换前的发布版本。同模块带版本Replace取真实替代版本。

GoVersion使用BuildInfo自身字段，缺失才用runtime.Version。所有版本文本只收64字节有效UTF8，不自行解析/编造Go semver，也不据元信息断言tag已发布。返回对象只有SDKVersion、GoVersion、GoravelVersion，不暴露module本地路径、校验sum、VCSrevision或modified状态。

当前workspace的私有pure Go helper实测SDK=(devel)、framework=v1.18.0、Go=go1.27.3；这是本机具体编译图，不是永久版本表。测试binary也可能省略部分module信息，unknown应保留，不能为让断言好看补伪值。

Heartbeat和Host消费者已沿同一metadata接入。Go/Goravel字段是显式可选协议字段，不伪造php_version/laravel_version；是否被目标Cloud存储仍要以消费者版本与验证为准，不能从HTTP成功推断全面兼容。

## 初步验证与练习

独立buildmeta两项0.686s，覆盖Main/Deps、缺失、devel、本地/异身份/同身份Replace、长值/坏UTF8与VCS不当版本。可选Go/Goravel与旧空字段wire必要race1.381s通过：新字段12项、旧默认仍10项，坏UTF8/超64字节零HTTP且没有PHP/Laravel字段。Host64真实bootstrap新项race2.966s验证Capture Context、CrashDiagnosticSettings、显式health heartbeat+runtime saved同源；独立MCP Run initialize race1.367s验证第4个实际SDKVersion消费者，零HTTP，不重复信号/工具测试。旧push manual受影响消费普通0.924s另列。没有读取实际.env或请求真实token/Cloud。

自主练习待完成：解释unknown和development区别；说明为何原Version在本地Replace后不再可信；找出SDK作为Main与依赖时的位置；解释为何SHA不是semver；说明编译元信息无法证明哪些发布/部署事实。

四层引用检查发现MCP Server.Version也硬编码development，因此同批改为Current.SDKVersion，保持bootstrap-free入口。Cloud兼容代码已有独立本地提交edcaca4，以上Go wire证据仍是本地fakeHTTP，不能据此声称目标Cloud已持久化/展示平台字段；真实验证由root另行授权执行。
