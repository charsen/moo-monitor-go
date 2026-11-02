# 37 Go构建版本元信息

新增buildmeta.Metadata/Current/纯Resolve，按精确SDK/framework module path查Main/Deps，标准ReadBuildInfo形状。缺失unknown，空/(devel)/本地或异身份Replace为development，同身份版本Replace取真实替代版本。文字只收64字节有效UTF8，不拿VCSrevision/Path/Sum当版本或发布证明。GoVersion取build info，缺失用runtime.Version。

HeartbeatMeta可选go_version/goravel_version，Host共用metadata替现SDKdevelopment与Capture/SelfTest/Crash三处Goravel硬编码。36提交后共享Heartbeat/Host已接入，Cloud代码root独占。仅fakebuildinfo/fakeHTTP/Host自有空env定向，不真实token请求。实际结果见下方，练习待完成。

## 初步证据

私有临时pure Go helper（不import Host bootstrap/config）在当前workspace读取真实BuildInfo：SDK `(devel)`且无Replace、framework `v1.18.0`且无Replace、Go `go1.27.3`；未输出模块本地路径/Sum/VCS。独立buildmeta两项首次0.686s通过，覆盖Main/Deps/缺失/local与同身份/异身份Replace、长值/坏UTF8、VCS不当版本。当时仅独立包阶段，未改共享。后续整合结果如下，未由本执行者请求真实Cloud。

## 整合实际结果

SDK可选平台字段+旧wire必要race1.381s；Host64新metadata consumer race2.966s；MCP唯一initialize版本断言race1.367s；旧push manual普通0.924s。四个实际平台消费者（Capture、SelfTest、Crash Diagnostic、MCP）与Heartbeat共享Current，不再维护平行版本常量。字段省略保旧10项、显式提供12项；错误UTF8/>64byte零请求。Cloud本地兼容独立edcaca4与Go fakeHTTP分栏，目标部署/存储展示不能从receipt推断。
