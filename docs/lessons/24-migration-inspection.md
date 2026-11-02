# 第24课：先看清旧数据，再设计迁移

[上一课：23](23-confirmed-resolved-maintenance.md) · [课程目录](../README.md) · [下一课：25](25-legacy-record-import.md)

本课实现只读 `migration.Inspect(ctx, Config)`。它不把 PHP 文件复制进 Go，也不因为 YAML 能解析就认定迁移可用。范围见[计划](../plans/24-migration-inspection.md)。

## 从源命令理解风险

PHP `src/Command/MigrateCommand.php` 把旧 scaffold/runtimes 和 scaffold/sql-slows 的 open/resolved/deleted 搬到新目录。同桶同文件名冲突按 meta.updated_at、last_seen 或 mtime 决定胜者，落败旧文件可能被删除；cursor 逐类型取较旧时间。它不是 PHP→Go 的记录格式转换器。

Go 的记录 Context、严格 YAML、journal、digest ACK 与 suppression 有各自契约。旧 timestamp cursor 不能证明某个实际内容已确认，不能变成 Go ACK 后直接跳过待推数据。本课始终返回 `ImportSupported=false`，列出格式转换与 ACK 障碍；这不是永久放弃迁移，而是把后续转换导入的前提显式化。

## 一条只读调用链

调用方显式给 RuntimeSource、SlowSource、CursorFile、EnvFile。未给的项不检查，不猜当前 PHP 仓、默认 storage 或真实 .env。

`Inspect` 先检查 context，再依次打开源目录、枚举三个固定桶、核文件名与 inode、限长读取、解析 YAML envelope，最后检查可选 cursor 和环境键名。没有 Buffer 构造、锁文件、目标目录、rename/unlink 或 HTTP。报告只包含固定 source/code、计数与键名映射，不包含原始路径、任意文件名、message、SQL 或配置值。

源根和文件先 Lstat 拒绝链接/非常规类型；打开后核 inode，内部访问用 os.Root 约束。每文件最多4MiB、每族枚举最多10000项、YAML节点深度最多64；超限不能当完整检查通过。别名/anchor、重复键与坏UTF8被拒绝。读取不是源目录全局一致快照；外部仍可能并发修改，后续真正导入必须重新验证。

`Readable` 只表示浅 envelope 检查通过：hash/文件名/桶状态、时间和族字段组存在且类型合理。它不完整验证嵌套 exception/sql 内容，不保证 Cloud 接收或 Go decoder 能读取。跨桶同hash单列冲突，本课不选择胜者。

cursor 要求严格 JSON 对象、唯一已知类型键和有效 RFC3339 时间，不接受 PHP 源那种坏JSON当空的宽松回退。env 只拆等号前的旧键名；已支持 Go 键给映射，其余标 unsupported。不会执行、复制或解析值，键可映射也不代表值域或默认行为等价。

## 如何使用结果

目录不存在会给 Absent，不是“已迁移”。有损坏/不安全项时同时返回安全报告和 error，源保留。没有损坏也只有体检完成，仍然禁止把 ImportSupported=false 解释成导入授权。

Host54 把这份能力接到显式命令（五模式真实bootstrap/Artisan定向通过1.255s）；真实转换导入将另行设计 schema 转换、目标所有权、冲突处理与可恢复提交。本课不自动获取这些权限。

## 实际验证与练习

SDK `TestInspect*` 使用既有纯PHP假fixture的build输出转YAML与自有临时目录。首测0.332s，补入口取消后0.471s，补缺失/null族字段守卫后最终0.348s。覆盖两族浅envelope、字节不变、安全报告、损坏/链接/重复hash、限制、cursor和取消；没有运行PHP Host、真实Cloud或导入真实数据。只读纯函数及文件读取没有新增并发写入，不为此启动全量/race。

用户自主练习待完成：解释“可解析、浅可读、可导入”三者区别；说明时间cursor为什么不能生成digest ACK；给一个跨桶同hash案例，列出真正迁移前必须由人确认的目标规则。

配置映射的 Supported 仅表示现Go有对应键的参考映射，不证明完整语义等价、不自动搬值。特别 SCAFFOLD_RUNTIME_ENABLED 对应 MONITOR_ENABLED 后是Go总开关，不能假设仅影响runtime。
