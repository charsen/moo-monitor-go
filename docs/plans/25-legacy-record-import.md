# 25：受控旧记录转换与幂等导入

2026-11-02，root已批准：保原hash/count/daily/first_seen/last_seen/bucket/resolve字段；runtime新增可选真实PHP/Laravel指针字段，不填假Go/Goravel。源只读，未知字段/不兼容类型/坏UTF8/不支持YAML标签整Plan拒绝。旧cursor只报告与封印，不生成ACK或suppression；PHP缓存overflow无法从记录复原，不承诺跨语言在线合并。

## 接口与顺序

- migration.ConvertRuntime/ConvertSlow：已核hash/bucket的PHP YAML转换为完整typed Record。真实纯PHP假fixture作为来源；不启动PHP Host。
- ConversionOptions.MaskKeys显式传给Slow转换，必须有有效敏感键；Host取现monitor.mask_keys，不复制默认列表。slowquery.Builder.SanitizeStoredSQL由native prepare和converter真实双消费，双mask/UTF8/cap/bytes；保hash与累计，不调用Build计数。PHP build缺meta.updated_at时只回退原last_seen，不造当前时间。
- BuildPlan只读显式源，固定3桶、完整源digest及目标编码digest；private条目加Plan seal，确定性、全Plan拒绝，不探/建target。源与目标重叠、非UTF8路径/键等提前拒绝。
- Plan.Apply只接新私有Root/Scope或同一sealed计划的部分结果。先全源重建seal核集合→目标锁/归属预检→先写import manifest→逐源重核→内部typed Buffer.Import→再核全源/目标→manifest complete。不能把逐文件原子写称跨文件事务。

Manifest绑定version/plan_digest/target_root/scope/每条family+hash+bucket+source/target digest/complete。没有manifest的目标即使bytes相同也拒绝；已有ACK/suppression、外来记录、native intent、overflow、不明journal均不接管。同plan+同编码同桶可续，不同bytes/桶拒绝，不使用mtime赢家。

Buffer.Import内部codec/热锁/atomicWrite，独立小import-intent仅绑定OwnerSeal/hash/bucket/digest；同owner/同最终镜像才可续。源不删、不搬cursor/lock，不调用Record或Refresh增计。完整后转成活动采集/同步scope再有新增状态时，不可继续用此cold import接管。

## 实测

纯converter完整fixture与SQLmask配置修正后0.349s；离线Plan0.341s；真实Apply→同plan幂等→重开两Buffer→Stats pending→fakeSync count5/7且resolved获确认1.043s。初Slow fixture因缺Go存储meta.updated_at失败，补源last_seen；nil MaskKeys实际没有敏感SQL默认键，改显式配置后通过，未放宽断言。

最终必要race：migration2.734s（Convert/Plan/Apply与漂移/恢复），buffer1.751s（typed import journal真实IO故障及原codec消费者），records1.406s（Native Context原fixture/边界），slowquery1.513s（原SQL fixture/安全）。末次严格拒绝未知YAML tag与路径UTF8的小守卫仅对应Convert/路径单项0.071s；前race属于补强前时点，不重跑无关旧项。Host55十模式race12.771s，另列Host计划。

全源检查包括新增/删除集合和每项全文摘要，但不等于源在线事务快照。源必须停写或用固定副本，目标不能被native Record/其它进程使用；migration锁只协调本导入器。最后核到complete仍有竞态窗口，不宣称阻止任意进程。未知临时文件/损坏manifest保原并拒绝，非通用自动恢复系统。

只私有fixture/本地fakeSync，无真实迁移、Cloud导入、全量/E2E或最低工具链/其它平台验证。用户练习待完成；每项验收commit后按执行队列持续推进，不自动push。
