# 32 DB façade原始SQL观察

新增adapters/db：Factory依原driver callback一次、公有BuildGorm同池cache、NewDB；不造平行sql.DB，不改modulecache。原Query链保持，Table只用原NewQuery与五方法CommonBuilder观察；真实事务NewTxBuilder→观察TxBuilder→原NewTx/Commit/Rollback，保Telemetry与txLogs，不双计。直接SQL五方法仅观察原delegate完成。

WithContext/Connection物理state与请求ctx分离；关闭/配置错误禁采但业务继续，原驱动/DB错误不吞。只真实执行完成后以安全归一/展示→原ReportSlowQuery；共用31PrepareSQL不复制类型和mask规则。Queryx仅返回rows窗口，后续迭代耗时/错误不伪涵盖。

SDK首私有SQLite原Query/Tx闭环后再Host lazy binding.DB，监控开关不resolve新池。新依赖预计无，mod/shared由writer。只自有文件/mock配置、无真实DB/Cloud/env，实际证据见下方；用户练习待完成。

## 当前实现与证据

SDK真实Factory/原Query/Tx已实现，Host采用走替换原DatabaseProvider的lazy binding；原ORM/Schema/Seeder生命周期保留，监控三开关不构成额外预热。原DatabaseProvider.Boot可能因Schema构造初始化默认连接，不能承诺整个进程零连接。

首Query夹具注册修正后1.166s；Factory同池/Context/Tx1.533s；Owner2项0.516s；新DB选定范围race6.540s；PrepareSQL旧PG/SQLite3项race6.781s。新driver负例首编译Select顺序误写已修，未将失败标通过。Host enabled必要race8.042s通过，三个负例首过；共享接线前失败/临时注册编译失败如实保留。root私有PG17.10新两路径集成0.944s通过，原目标表已DROP/池关闭；夹具database遗漏与Raw断言精度修正后通过，非产品修复。hash原输入与持久Raw脱敏/Last展示分别说明，不宣称所有Host池或PG部署验证。练习仍待用户完成。
