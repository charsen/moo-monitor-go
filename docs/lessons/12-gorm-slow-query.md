# 第12课：在真实 GORM 执行窗口采集慢 SQL

[上一课：11](11-slow-query-pipeline.md) · [课程目录](../README.md)

第11课已能把显式 SQL 输入聚合、落盘、同步。本课将已存在的 Goravel ORM 实例接到这条链：执行成功的 SQL 在 GORM 回调内转成窄 Input，再交 SlowBuffer。SDK 不构造业务连接，不解析日志，也不连接 Cloud。[计划](../plans/12-gorm-slow-query.md)记录设计与实际验证。

## 先区分实例能力与框架契约

锁定 framework1.18.0 的具体 `database/gorm.Query` 有公开 `Instance() *gorm.DB`，但 `contracts/database/orm.Query` 未保证此能力。`Install(query, plugin)` 先检查这个能力，再检查非空实例和 SQLite dialector；失败返回静态错误，不通过反射修改 ORM。

这使生产安装入口可用，但不会发现所有 lazy 连接。owner 必须在应用原本初始化连接的位置、尚无并发查询时传入已有实例。不能为了监控枚举连接并主动打开数据库。另一个命名连接、新建实例或 Fresh 后的实例需要明确安装；本课不伪装成全连接自动发现。

| 文件 | 职责 |
| --- | --- |
| [plugin.go](../../adapters/gorm/plugin.go) | 实例能力、安装/回滚、六类回调、生命周期和安全诊断 |
| [binding.go](../../adapters/gorm/binding.go) | 已知绑定类型的同步复制，拒绝额外用户方法调用 |
| [plugin_test.go](../../adapters/gorm/plugin_test.go) | 真实 Goravel ORM、原日志与 after hook 窗口、安装回滚 |
| [lifecycle_test.go](../../adapters/gorm/lifecycle_test.go) | 连接/context、过滤、失败隔离、并发与关闭 |
| [binding_test.go](../../adapters/gorm/binding_test.go) | 绑定类型、坏 UTF8 与 Valuer/Stringer 边界 |

## 一个 SQL 的实际顺序

GORM1.31.2 的 `callbacks.go:processor.Execute` 先执行注册回调，再调用原生 Logger.Trace，最后清 SQL 和 Vars。因而不能到 Trace 的格式化字符串中反推参数，也不能等整个 processor 返回后再读 Statement。

插件在六个 core 回调 create/query/update/delete/row/raw 前保存开始时间，在 core 后、后续业务阶段前同步读取 SQL、Vars、Context。create/update 的后锚点在 `gorm:save_after_associations` 前，query 在 `gorm:preload` 前，delete 在 `gorm:after_delete` 前；row/raw 在对应 core 后。这样业务 AfterFind 再查询时，主 SQL 已交给 Collector，不会被后续 Statement 清理或嵌套查询覆盖。

实际主链是：取出并删除本次计时 → 跳过 DryRun/内部诊断/执行错误 → 进入在途计数 → 截定耗时 → 复制已知 Vars → 派生采集 context → 读取显式请求标量/callsite → ReportSlowQuery → 更新安全状态 → 离开在途计数。SQL 的 Error、RowsAffected 和返回结果不由监控修改。

`gorm.ErrRecordNotFound` 表示真实 SELECT 成功但无行，允许采集；其它执行错误跳过。事务内 SQL 已执行成功，即使后来回滚也仍有慢查询记录。`*sql.Row.Err()` 只读取已知错误，不提前 Scan；后续 Rows 迭代耗时和迭代阶段错误不在这个同步窗口。

原 Logger.Trace 仍执行一次，插件不再次调用 Explain 或其 fc。但原 Trace 在所有回调之后计算耗时，因此它会包含同步监控落盘的开销；本插件 Took 在 collect 入口截定，不含后续采集写入时间。不能据此宣称原生日志耗时完全不变。

## 安装属于启动期，关闭属于 owner

```go
// query 来自应用原有连接初始化；store 是已创建的 SlowBuffer。
plugin, err := gormcapture.New(gormcapture.Config{
    Connection: "sqlite",
    Recorder: store,
})
if err != nil { return err }
if err = gormcapture.Install(query, plugin); err != nil { return err }
// owner 退出时 plugin.Close(ctx)，之后再关闭 store/业务连接。
```

`Install` 只串行注册，不为查询设置全局 busy 闸。同实例同 plugin 指针重复安装幂等，不同 owner 拒绝。先核所有固定 anchor 和本插件12个名字未占用，再逐项注册；若编译中途失败，只反向移除本次已追加的项，保留原业务回调。这个回滚只允许在尚无业务流量的启动窗口；Close 不运行期卸载 callback。

Close 禁止新的采集，并按调用方 context 与 CaptureTimeout 的较早边界等待已进入采集。默认采集超时100ms，派生 context 不修改 SQL 的业务 context。同步文件系统 syscall 不是硬实时可中断操作，不能把这个超时解释成任何环境下的硬执行上限。

插件借用业务 DB 和 SlowBuffer，均不自行关闭。诊断回调不计入 Close 的在途等待；调用者必须遵守派生 context，SDK不通过无限 goroutine 强杀任意用户回调。

## 窄转换与旁路故障

Vars 只支持明确的 nil/bool/int/int64/float64/string/time.Time、有效 UTF8 字节串及列出的已知指针形式；复制后不保存 Statement。任意 `driver.Valuer`、Stringer、struct 或其它数值类型不额外调用用户方法，给安全 input 诊断。数据库驱动自身对 Valuer 的执行仍是原有业务机制，插件不重复调用。

阈值/skip、SQL/Secrets 脱敏、hash、daily/cap/overflow 全部由第11课权威实现处理。本课不复制规则。Request resolver 只从当前 context 返回已提取标量，SDK 不导入 Host 身份，也不保存原请求。

Recorder 错误和 panic 不写入 Statement.Error。Status/OnError 只存固定分类，不保存或输出任意 cause、原 SQL、token。OnError 使用 internal context 发日志或 SQL 时会跳过回采；丢弃这个 context 另起采集不属于自动防递归保证。诊断自身 panic 可从 ObserverPanics/ObserverFailed 观察，原业务结果保留。

## 覆盖矩阵与依赖

| 路径 | 本课覆盖 |
| --- | --- |
| 已安装 SQLite ORM CRUD/Exec/WithContext | 六 core 回调同步观察 |
| 事务内成功 SQL | 采集，后来 rollback 不撤销 |
| Goravel DB façade | sqlx 路径，不经过这些 GORM callbacks |
| 原生 database/sql、未安装连接、Fresh 后新实例 | 未自动覆盖 |
| PostgreSQL `$1`、其它 driver/自定义 callback 链 | 未验证；本适配拒非 SQLite |
| 真实 Cloud、Host 所有 lazy 连接 | 未完成 |

固定 framework/sqlite1.18.0 与 GORM1.31.2，不追 latest。GORM 从间接变直接依赖；测试 SQLite 增加 ncruces/gormlite、julianday、wazero，dbresolver1.6.2 来自真实 DB façade 消费；mysql driver 仅上游 dbresolver 测试图的 sum，不表示本测试连接 MySQL。Go最低1.26，实际1.27.3，x/sys0.49、cpuid2.4 不变。

## 验证与练习

本课只使用0700自有测试目录内 SQLite，TestMain 仅 Register 真实 sqlite provider，未 Boot Host/DB Provider。首轮测试误用 Update 三参 API 返回参数错误；改为 Model(...).Update 后发现 driver Grammar 需要公开 provider 的 Json 绑定，补真实 Register 后首两项通过1.369s。

补安装中途 Match 编译 panic/仅回滚本次项/原链仍可查/重装成功、过滤/双连接/当前身份、失败诊断、16并发、有界 Close、绑定边界的七项通过1.284s。其间测试在 Row 尚未 Scan 时的诊断内再次使用单连接 DB 导致连接等待超时，最终业务验数使用明确 internal context，避免验证动作自身再触发采集；生产 OnError 不应依赖同一耗尽连接池做阻塞诊断。

随后10项定向 race 通过6.906s，包括采集 deadline 不改业务 context、阻塞 observer 不拖住 Close。随后真实 NewDB→Table.Count 的 DB façade 负覆盖单项通过0.978s，确认该 sqlx 路径没有增加插件计数。未跑全量/E2E、外部数据库或真实 Cloud；Host43已审定默认空的显式业务预热列表，实际生产接线/allowlist和真实认证Food查询仍待该独立小步验收。安装回滚末尾增加最终compile失败的安全ErrRollback分类后，必要安装回滚单项通过0.969s；未重复前面的race。

自主练习待用户完成：画出 core/after hook/Trace/Reset 的顺序；说明为什么 rollback 后仍有 SQL 样本；列出第二命名连接与 Fresh 的安装责任；预测同一 Row 连接未释放时诊断再查 DB 的风险，并解释采集 timeout 与业务 SQL context 的区别。代理测试不代做学习验收。
