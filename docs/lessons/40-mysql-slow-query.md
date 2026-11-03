# 第40课：MySQL展示SQL与原业务执行分开

[上一课：39](39-self-test-resolution.md) · [课程目录](../README.md) · [下一课：41](41-sqlserver-slow-query.md)

本课的参考实现与[计划](../plans/40-mysql-slow-query.md)只覆盖明确安装的MySQL实例与DB façade。sqlmock提供SQL执行结果，没有运行真实MySQL服务。

## 从一个更新动作读到记录

业务GORM Update先由成熟MySQL dialector生成带?的Statement；插件在gorm:update前记时间，在其后、save_after_associations前读取原SQL/Vars。mysql1.6.0使用默认callback注册，并未像后续SQLServer那样替换create/update。

collect → PrepareSQL → mysqlRendered → bindings先复制允许的不可变值 → dialect.Explain生成独立展示 → SlowBuffer → 既有hash/双mask/存储预算。adapters/gorm/mysql.go负责展示预算，sql.go仅按dialect分流，plugin.go继续拥有执行窗口和Close。原Trace仍按原生路径一次，监控工作可能增加原Trace观测到的耗时。

原Raw仍决定累计身份，不把含绑定值的展示SQL当hash。持久Raw也会脱敏，所以不能通过持久Raw逐字还原原查询。RenderedSQL与Bindings互斥；有限formatter不是协议反编译，引号中的?等边界按成熟Explain行为处理。展示结果绝不发回数据库。

4MiB展示预算独立于4096码点存储cap：调用Explain前限制raw、参数数目与最坏字符串转义开销，调用后再核UTF8/长度。未知Valuer/Stringer不为监控再次执行；数据库可能已调用一次，但监控安全拒采，不替换原结果。

## DB façade沿原路径运行

Factory通过公开BuildGorm复用原连接池。Table仍由NewQuery执行，BeginTransaction仍用NewTx；5个直接方法则调用原DB后准备监控输入。原生日志的事务提交时机不改，已执行后回滚的SQL仍可记录。WithContext/Connection保留身份及连接名；Close只停止观察，不关闭业务池。

测试中官方NewMysql(config).Pool映射是真实的；仅SQL transport dialector注入sqlmock。版本探测会额外连接服务，所以公开Grammar固定8.0.36并明确跳过探测；不据此宣称生产版本协商、MariaDB returning、TLS或真实类型行为已验证。

## 验证与自主练习

本机Go1.27.3定向GORM组1.130s、Factory0.759s通过，实际六callback、原Trace、同池、原Query/Tx、5direct methods、Context/Connection、commit/rollback、原error/cancel/Close与安全拒绝均有断言。失败修正与依赖映射见计划；未做全量/E2E或真实服务验证。

请自主追踪一个Update从原业务SQL到Record.Last；解释Raw/hash为何不随绑定值变化；预测未知Valuer为何可以业务成功却不采；说明rollback仍有记录、Close后业务继续、sqlmock通过仍不能证明哪些真实服务行为。代理通过不代做用户学习验收。

必要共享回归与新MySQL范围race通过：adapters/gorm 7.137s、adapters/db 6.858s；范围仅新MySQL、旧SQLite callbacks/window、原Query/Tx与PG展示，未运行全量测试。

## 2026-11-03 真实 MySQL 双入口补证

使用本机隔离的 MySQL 26.7（127.0.0.1:3307，临时 datadir，未连接生产）运行了 opt-in integration 测试：

- `adapters/gorm/mysql_real_test.go`：真实 MySQL GORM 创建/写入/查询，GORM 插件捕获原始与展示 SQL；
- `adapters/db/mysql_real_test.go`：真实 MySQL Goravel DB façade 同池观察，`Statement`/`Table` 路径捕获 `At.Connection`；
- 两包并行运行通过，测试使用独立表名，结束后清理。

命令：

```bash
MOO_MYSQL_DSN='root@tcp(127.0.0.1:3307)/moo_monitor_integration?parseTime=true&charset=utf8mb4'   go test -tags=integration ./adapters/gorm ./adapters/db -run 'TestMySQLReal' -count=1
```

这证明当前代码能连接真实 MySQL 并完成双入口慢 SQL 观察；仍不代表目标 MySQL/MariaDB 版本、TLS、生产权限/连接池或所有 SQL 类型已验证。
