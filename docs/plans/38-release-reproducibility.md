# 38 分层验证独立解析与构建

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

本步先验证本地HEAD快照的可复现构建，不伪称已发布。用各仓已提交HEAD的git archive，不打包工作区、ignored .env、账号、用户IDE或未提交P1；记录SDK/scaffold/Host SHA与toolchain/平台。

SDK与scaffold使用缓存锁定golang.org/x/mod v0.37.0的zip.CreateFromDir，生成显著fixture prerelease版本的标准Go module zip/mod/info。全部helper/proxy/新cache/temp Host在自有临时根，结束只清该根。临时Host只加两个精确模块的fixture require；记录完整临时manifest差异，不改真实Host require，不用replace/workspace来证明独立解析。

运行时GOWORK=off、GOTOOLCHAIN=local、GONOPROXY=none；GOPROXY只含本地fixture与当前cache/download的file URL，无HTTP/direct fallback。GOSUMDB=off仅本次离线夹具，不作为生产配置建议。外部依赖缺缓存即报错，不为补齐追latest/联网/系统安装。构建Host主程序与独立MCP二进制但不运行，不执行全量/E2E/生产迁移。产品源码缺失/提交闭包问题应报告具体消费者，不临时patch产品冒通过。

[Go Modules Reference](https://go.dev/ref/mod#private-modules)明确GOPRIVATE默认影响GONOPROXY/GONOSUMDB；fixture要显式GONOPROXY=none才不绕过本地proxy。[VCS发现](https://go.dev/ref/mod#vcs-find)与[proxy协议](https://go.dev/ref/mod#module-proxy)分别说明仓发现与版本归档机制。SSHclone成功不代表无.git模块路径的go-get discovery和私有认证已通过；本步不取凭据、不请求真实Gitee、不push/tag。远端发布/解析另需对应授权与实测。

当前Host实际manifest未require两个私有包，依赖workspace；夹具改动仅说明将来的发布形态可否构建，不能把fixture版本复制进真实生产manifest或当远端tag证明。计划起点当时待37提交；实际执行结果见下方，未阻P1并行。

## 实际执行

固定SDK5cceac7241f305c35b9cb67652b973c35dc76b14/scaffolda9c9b46975f5cff342ea5da028e58eef315a56f9/Host8fd3e278f395aac15e6eaccf4f733b224dea42db，archive PAX与SHA256均实核。首工具mac temp alias guard误拒保failed原JSON；规范化root后Go1.27.3 app+MCP build通过。root提供官方已核Go1.26.8绝对工具链后，同图两target floor通过，不运行或复跑测试。

两个passed原始报告+八项比较在docs/contracts/fixtures/release-reproducibility*.json；完整临时mod/sum差异与两requires/MVS分别保存（29/47行），包含flock indirect与xsys0.48→0.49，没有改真实Host manifest。两次ownedroot清理/残留0实核。仅同源同图可构建，不是bit-identical、远端tag、Gitee auth/discovery、生产部署或其它平台验收。38后继续P1，无自动发布副作用，练习仍待用户。

快照边界：上述scaffolda9c9b46/Host8fd3e27早于P1-01的52dc598/b326d77，不能据此称新账户/session已独立floor构建。root新增要求：38提交后固定新三个SHA，另存一次Go1.26生产构建报告，保本两份report原样，不改产品规避失败；路由目录只读准备可并行。

## P1-01新消费者追验

38提交后按root新增授权，用新固定SDK `4ba95a6b1ca4e533a526fc27ca08a5199acaa9de`、scaffold `52dc59850a2c11c1d8a634c84eeaad454cc59c59`、Host `5ca7f26ce29ead4f0de94a77c4aa2b3b1ef38269`另执行仅Go1.26.8生产构建。[独立新报告](../contracts/fixtures/release-reproducibility-p1-go126.json)中主App108555586字节/MCP9169202字节成功、未运行，P1账户/session实际新依赖已进入快照；ownedroot清/残留0。原38两报告未改，不能把新结果倒写到旧快照。此为编译/独立解析证据，账户行为仍按P1-01自己的定向/race结果，不是Go1.26功能全回归或远端发布认证。

## 当前契约衔接（2026-11-03校准）

当前已存在final-go126固定快照，且三仓main/dev后来已推远端；报告中的file-proxy fixture版本依然不是稳定tag/discovery/私仓解析证明。后续源码修正仅有各自定向证据，不覆盖旧Go1.26固定SHA。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
