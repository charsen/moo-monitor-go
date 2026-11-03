# 34 显式单ID崩溃诊断导入

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

仅显式手动导入单个spool ID，不自动扫描/startup导入、删除raw或上传Cloud。先Prepare固定masked snapshot/时间，Apply再次Inspect核当前ID+raw全文digest；源应已停写/固定副本，跨spool与buffer不是事务，仍可能在最后检查后改变。MayBePartial始终保留，不把raw解释成确定panic/完整fatal。

不能直接用25 Import：记录被后续Sync删除后AlreadyPresent不提供永久幂等，且Import绕过容量。新窄Buffer.ImportDiagnostic复用真实Builder.Build/Encode、gate/flock、count/find/atomicWrite与recover。新身份先核MaxOpen，在持久intent之前拒绝容量不足；daily初始固定1，不Record重计。

显式version2 intent.yaml诊断变体，旧version1原样可恢复：{version,artifact_id,raw_digest,hash,image_digest,target,record}。固定图像只在未完成intent中，永久diagnostic-receipts/<id>.json只保{version,artifact_id,raw_digest,hash,image_digest}，不重复保内容。序列intent→open record→receipt→删intent。native操作先恢复同图像再继续，Slow拒v2并保原；ReadOnlySnapshot仍拒未决journal。

恢复先于再次Build。receipt已存在须与intent全部身份一致才仅清intent，不能重建已移除的record；损坏/冲突保原错误可见。done同artifactID+rawDigest永久AlreadyImported，哪怕record后来被Sync移除；同ID内容改变冲突，避免活跃spool反复计数。

MaxReceipts默认10000，hardcap100000，磁盘独立文件、有界枚举；新身份达预算ErrReceiptLimit，已有身份查询/恢复不被预算挡住；不自动GC并继续宣称永久去重。诊断Exception.Class=CrashDiagnostic，File=crash-output/<artifactID>仅稳定人为身份，不是源码路径，不交给SourceContext读取raw。

初始验证安排（最终结果见下）：Host命令单ID及确认门槛；不读取真实.env，不连接真实Cloud。先内核定向：固定图像/恢复/永久receipt、容量、坏状态与版本1兼容，实际结果见后续证据。

## 当前实际节点

2026-11-02：buffer compile-only0.896不算测试；首永久receipt/三故障重开/容量节点TestDiagnostic1.066。补坏receipt/foreign record/坏v2与Slow拒绝后定向1.377。Manager不可变Prepare→Apply再次核源→真实Buffer→fake Sync确认→同ID重试保持Count1/原时间及源变更冲突单项0.419。以上无真实Cloud或真实进程hook；race与Host实际命令仍待本功能后续节点，不拿compile-only替验证。

## 最终接口与验证

crashoutput.Manager.Prepare(ctx,id,DiagnosticConfig,at)返回私有不可变Prepared，Apply(ctx,prepared,*buffer.Buffer)重新Inspect再交可信typed ImportDiagnostic。Prepared不能跨Manager冒领，原raw不改；事件固定CrashDiagnostic/diagnostic_may_be_partial与人为File身份，非源文件路径。不走Collector的SourceReader，不解析成确定panic。

最终必要race：buffer新诊断及旧version1恢复/typedImport3.215，Prepare/Apply/fakeSync1.683；Host61真实命令十模式normal1.580/race12.637，旧CrashConfig1.546。默认prepare字节不改Buffer/receipt，apply/confirm=false、缺Ready Buffer、重复Count1、同ID变更、空raw/非法ID/额外参数均断言。owned空env/假配置，无真实Cloud/业务数据或当前进程hook。

命令只追加本地pending，不自行发HTTP；后续独立push可按已有配置发送。Cloud readonly门槛不复用于本地追加，实际需apply+confirm；MaxReceipts配置独立。Host bootstrap已有owners可能初始化资源（capture.crash=true时含空spool），不把操作只读等同整进程零副作用。原raw应已停写/固定副本，跨FS竞态窗口与永久receipt手工删除风险公开。没有自动startup导入/自动GC/真实Cloud/全量/E2E。

## 当前契约衔接（2026-11-03校准）

当前采用单ID显式Prepare/Apply，不自动扫startup；仅现有Ready Buffer、源固定副本与永久receipt。真实业务崩溃材料/跨平台行为仍未代验，不按历史“继续35”误记当前Summary未实现。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
