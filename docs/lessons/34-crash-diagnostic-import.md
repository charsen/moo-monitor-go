# 第34课：导入一次诊断，永久记住本地处置

[上一课：33](33-crash-output.md) · [下一课：35](35-cloud-summary.md) · [课程目录](../README.md)

[计划](../plans/34-crash-diagnostic-import.md)给33私有spool一个显式单ID入口。它不自动扫描、startup导入、删除raw或发Cloud请求；导入事件成为普通pending，后续独立push可正常发送，不能把命令零HTTP解释为永久禁止同步。

## 为什么不能直接Capture再写done

Capture→done是两步，进程可能在已增计但done未落盘时中断；重试便重复count。25的Import虽可重试固定图像，但resolved经Sync移除后也会重新创建，且其冷scope语义绕过正常容量，不能原样套到运行中诊断。

新buffer/diagnostic.go在同一gate/flock内：先恢复已有intent，再查永久receipt；同artifactID/rawDigest已完成就直接AlreadyImported，不再次Build(new now)。新身份检查receipt预算及MaxOpen、拒已有无归属record/overflow/其它import journal，再用该Buffer真实Builder.Build一次生成Count/Daily.Count=1的固定图像。

永久diagnostic-receipts/<id>.json只保存version、artifactID、rawDigest、hash、imageDigest，不重复保存敏感图像。MaxReceipts默认10000、最多100000，有界枚举；预算满拒新身份，既有receipt查询/恢复仍可进行。没有自动GC后仍宣称永久去重；手动删receipt会破坏去重依据，不是受支持维护流程。

## version2恢复是真正的提交边界

recovery.go保原version1严格格式，显式分流version2诊断变体。顺序是持久intent→固定open record→永久receipt→清intent；固定图像仅在未完成intent。每个native恢复/读取先收完该序列，ReadOnlySnapshot遇未决intent仍报需恢复，Slow拒v2且不改文件。

已有receipt必须与intent全部artifact/raw/hash/image一致，才仅清intent，不重写已被后续确认移除的record。损坏/冲突保原并报静态错误；IO未知结果保errors.Is(ErrOutcomeUnknown)，重开沿原图像恢复而非再Record。容量不足在intent前拒，不能写done假成功。

crashoutput/diagnostic.go的Prepared私有持有masked snapshot与首次时间，不给调用方可变map。Apply先重新Inspect同ID全文digest；同ID内容改动明确冲突，不降格为AlreadyImported。原raw应已停写/固定副本，最后检查与buffer提交之间仍不是跨文件系统事务；一旦intent承诺，恢复使用原已认领图像。

事件Class=CrashDiagnostic，MayBePartial标记保留，File=crash-output/<artifactID>是人为稳定身份，非真实栈路径；不交SourceContext读raw，也不从无法证明完整的文本推断确定panic。直接Buffer.ImportDiagnostic是可信typed入口，spool归属/digest由Manager负责。记录仍经过真实Builder脱敏/cap，不复制另一套记录器。

## Host61与实际证据

真实生成的moo:monitor:crash-import默认只Prepare；--apply与显式--confirm=true才写现成Ready Runtime.Buffer，false不授权。Cloud readonly不适用于本地追加事件，不用Cloudtoken；没有Ready Buffer就Unavailable，不造第二Builder。命令只输出ID/digest/hash/安全类别，不输出MaskedText或raw，保源文件。显式`--all`只遍历当前spool并复用同一Prepare/Apply与receipt；`MONITOR_CRASH_STARTUP_IMPORT=true`可选在Runtime启动时做同样的批量导入，默认关闭。构造阶段的Buffer目录/空spool副作用与命令操作分开，示例建议capture.crash=false只读旧文件。

首buffer闭环1.066s；补坏receipt/foreign record/坏v2及Slow拒绝1.377s；不可变Prepare→Apply→fakeSync→retry保持Count1/原时间与raw漂移冲突0.419s。最终buffer新项+旧v1恢复/typedImport race3.215s，crashoutput本项race1.683s。Host真实Call十模式普通1.580s/race12.637s，受影响旧CrashConfig race1.546s；均owned空-env/固定假配置，没有真实.env、Cloud或当前进程hook。compile-only0.896不是测试证明。

自主练习待完成：解释为什么确认删除record后仍需receipt；对比raw digest与image digest；说明未知结果何时恢复、何时保原报错；预测同ID raw变更与新ID预算耗尽；解释人为File为何不能用于源码读取。
