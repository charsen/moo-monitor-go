# 19：精确待推只读统计

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

2026-11-02，本轮第一功能，对应Host49；第二功能为Cloud确认后的local噪音协调，分别验收commit，不push/merge/tag/全量/E2E/真实Cloud。

NewStatusReader(runtimeBuffer,slowBuffer)无需Client/URL/token/cloud开关，复用既有scope/锁/严格ACK读取；现Syncer/Pusher.Stats共用每类型实现。Stats使用相同type gate/flock，完整bucket+digest+timestamp匹配，按open/resolved给pending/confirmed及Scanned/Cursor。Known=false时Counts/Cursor缺席，Busy/坏状态/不可用不能当pending0。

严格只读只指Stats操作：不HTTP/AfterPush/save/ACK剪枝/GC/推进cursor。构造可prepareLock；已有Buffer初始化/HostBootstrap可能按原契约恢复，须单列。新ReadOnlySnapshot检查intent存在即ErrRecoveryRequired，不recover；复用Snapshot的枚举/解码/指纹，不复制权威规则。

共享文件snapshot/state/sync与Host入口由implementation唯一写；第二项doc仅新discard/suppression文件，第一项不实现suppression。后续同精确分类扩展，不把未完成能力算通过。

验证最小私有目录：未推/部分ACK/同ms改值/已确认/resolved/badstate/未决journal/Busy/跨实例锁/取消/无网络，Stats前后文件列表/bytes不变；Host真实make命令及本地service，Cloud关闭/无凭据仍读，未知不打印0。只新增定向+必要旧Snapshot/Sync/Pusher消费者。实际结果见下。

## 2026-11-02实际结果

SDK四个TestPendingStats首测0.829s，补坏open记录/真实Sync ACK/禁推buffer仍统计后0.528s。最终Stats与现Pusher关闭/取消/partial、选定Sync消费者race3.430s；buffer选定Snapshot消费者race2.102s。Host五模式真实Bootstrap/Artisan首测1.217s，最终race7.597s。Cloud关闭/无token/push禁用仍能读已有源，文件列表/bytes不变、无HTTP/AfterPush、坏状态/journal/Busy非有效0均有断言。全部只私有目录与HTTP spy，父日志目录已白名单清理；无全量/E2E/真实Cloud。

Pusher构造保留所有传入buffer句柄供Stats，因此新增公共构造路径已有Pusher必要回归；Stats不按发送开关隐藏现buffer。StatusReader使用私有具名字段，不暴露client-free Sync方法。19/49不引用20新符号，无init副作用，不为闭包重复已过业务验证。用户练习待完成。

## 当前契约衔接（2026-11-03校准）

NewStatusReader仍是strict combined构造，任一源失败关闭已开句柄；Host工作台只为Status/Preview提供分类型fallback，不能将它借给Prune。Stats本身只读，构造可能准备锁文件。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
