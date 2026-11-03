# 24 只读迁移体检

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

本步新增 `migration.Inspect(ctx, Config)` 与 Host54 显式命令。所有路径由调用方提供，仅扫描 open/resolved/deleted，不创建 Buffer、锁、目录或网络请求。

报告只含固定分类、计数、已知旧配置键映射；不携带路径、文件名、记录内容或配置值。坏格式、链接、跨桶重复与无效时间可见，原文件保持。PHP YAML envelope 可读不代表 Go typed 记录可直接导入；`ImportSupported=false`，Context/格式转换与 digest ACK 是后续真实迁移必须解决的阻碍。cursor 不生成 ACK。

使用现有 YAML 依赖，文件有大小上限，rooted 读取加 inode 复核；不承诺并发源目录一致快照。定向测试仅自有目录/既有纯假 fixture，验证报告及字节不变、坏输入与取消。实际结果：SDK TestInspect最终0.348s（入口取消/缺失族组修正后）；Host54真实bootstrap/Artisan五模式1.255s。均只自有假目录，没有导入或网络。

本轮完成后继续设计真实转换导入，不因预检完成永久停留在体检。用户练习待用户完成。

配置映射的 Supported 仅表示现Go有对应键的参考映射，不证明完整语义等价、不自动搬值。特别 SCAFFOLD_RUNTIME_ENABLED 对应 MONITOR_ENABLED 后是Go总开关，不能假设仅影响runtime。

## 当前契约衔接（2026-11-03校准）

只读体检不等于转换导入。25已提供固定只读源副本/冷scope的显式幂等导入；旧cursor不变ACK、不自动写PHP目录，真实legacy数据仍未代验。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
