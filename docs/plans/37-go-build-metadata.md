# 37 Go构建版本元信息

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

新增buildmeta.Metadata/Current/纯Resolve，按精确SDK/framework module path查Main/Deps，标准ReadBuildInfo形状。缺失unknown，空/(devel)/本地或异身份Replace为development，同身份版本Replace取真实替代版本。文字只收64字节有效UTF8，不拿VCSrevision/Path/Sum当版本或发布证明。GoVersion取build info，缺失用runtime.Version。

HeartbeatMeta可选go_version/goravel_version，Host共用metadata替现SDKdevelopment与Capture/SelfTest/Crash三处Goravel硬编码。36提交后共享Heartbeat/Host已接入，Cloud代码root独占。仅fakebuildinfo/fakeHTTP/Host自有空env定向，不真实token请求。实际结果见下方，练习待完成。

## 初步证据

私有临时pure Go helper（不import Host bootstrap/config）在当前workspace读取真实BuildInfo：SDK `(devel)`且无Replace、framework `v1.18.0`且无Replace、Go `go1.27.3`；未输出模块本地路径/Sum/VCS。独立buildmeta两项首次0.686s通过，覆盖Main/Deps/缺失/local与同身份/异身份Replace、长值/坏UTF8、VCS不当版本。当时仅独立包阶段，未改共享。后续整合结果如下，未由本执行者请求真实Cloud。

## 整合实际结果

SDK可选平台字段+旧wire必要race1.381s；Host64新metadata consumer race2.966s；MCP唯一initialize版本断言race1.367s；旧push manual普通0.924s。四个实际平台消费者（Capture、SelfTest、Crash Diagnostic、MCP）与Heartbeat共享Current，不再维护平行版本常量。字段省略保旧10项、显式提供12项；错误UTF8/>64byte零请求。Cloud本地兼容独立edcaca4与Go fakeHTTP分栏，目标部署/存储展示不能从receipt推断。

后续root获用户授权向已部署Cloud执行真实health，heartbeat与runtime/slow两类saved回执通过；页面仍显示旧PHP/Laravel标签，新Go字段生产持久/展示未确认。该外部健康结果不替代本地wire测试，也不升级为字段展示已验。

## 当前契约衔接（2026-11-03校准）

真实Go/Goravel版本发送已接并在Cloud心跳页实见。SDK未正式发布时development不是semver；runtime记录级版本持久化仍有独立缺口，见cloud-go-compatibility，不可泛称全部元信息兼容。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
