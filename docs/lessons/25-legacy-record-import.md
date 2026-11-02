# 第25课：先封印转换计划，再导入冷命名空间

[上一课：24](24-migration-inspection.md) · [下一课：26](26-mcp-stdio-loop.md) · [课程目录](../README.md)

24只证明旧信封浅可读，本课才做完整typed转换与显式写入。[计划](../plans/25-legacy-record-import.md)记录批准语义、文件职责与实际结果。导入不是把旧YAML直接复制到活动Go目录，也不是调用Record重放每次异常。

## 哪些值保持，哪些值明确转换

hash、count、daily、原时间、状态桶和resolve信息保留。runtime的真实php/laravel进入新增可选字段；原生Go采集仍用真实go/goravel，nil legacy字段不进入原JSON。空Laravel字符串也用指针保presence。旧cursor永不变成digest ACK，导入后open/resolved仍待推。

慢SQL的PHP raw不保证脱敏，必须经过既有SanitizeStoredSQL双mask/UTF8/cap规则；raw/last内容及bytes会按Go存储策略变化，但不重算hash或count。敏感键由显式ConversionOptions传入；Host复用现配置，未配置有效键拒绝。不存在暗中“无键也已脱敏”的兜底。缺存储updated_at只用原last_seen补，不填now。

未知字段、别名/重复键、未知标签、不兼容标量和坏UTF8明确拒绝整个计划，不静默drop。payload和元信息按已支持typed存储表示处理，旧缓存overflow不能从现YAML复原；不把历史snippet/自由文本当已全面秘密清洗。

## 读主链与文件

| 文件 | 职责 |
| --- | --- |
| migration/conversion.go | 完整DTO形状校验、真实legacy元信息与SQL安全转换 |
| migration/plan.go | 有界源读取、源集合/全文摘要、目标编码摘要与不可变seal |
| migration/apply.go | 新scope归属、manifest、逐项写入与可审部分结果 |
| buffer/import.go | 内部codec与热锁、精确镜像写入/小journal重试，不增计 |
| records/records.go | 可选PHP/Laravel字段；native路径仍原合同 |
| slowquery/builder.go | native与import真实复用同SQL存储规则 |

BuildPlan只读源，不打开或创建target。Apply先核整份源集合，再认领空命名空间；无manifest已有数据即使相同也不冒领。每项再核原源digest，内部Import只写封印镜像；同plan同bytes幂等，异桶/异bytes拒绝。

manifest先记录归属，最后标complete；中途失败可能已有部分文件。同plan原源不变可以续，文件计数不变。OwnerSeal是归属摘要，不是绕过私有目录权限的认证。实际Buffer里若出现ACK、suppression、native journal、overflow或外来记录就停止，保原不接管。

## 不能承诺的原子性

导入锁不约束任意native Record。源应停写/用固定副本，目标只用新隔离scope；前后摘要核对仍不构成多文件事务。最后检查到complete之间仍有窗口。未知残留临时文件/损坏归属不会自动删；保留源和部分结果，人工审查。

## 验证与练习

纯转换0.349s、Plan0.341s、Apply/重开/fakeSync1.043s。最终定向race migration2.734/buffer1.751/records1.406/slowquery1.513；最后tag/UTF8路径守卫仅窄单项0.071s，时点分清。Host55十模式12.771s另列。真实Root的Cloud健康saved证明不覆盖本课导入记录或新字段兼容；这里只私有fixture，无全量/E2E/真实迁移。

自主练习待用户完成：解释cursor不能授权ACK；构造同bytes/跨桶两个目标结果；模拟manifest已认领但只写一条后续作；说明为什么Apply后开始正常Sync便不能继续冷导入。当前持续推进，不因本课完成停止执行队列。
