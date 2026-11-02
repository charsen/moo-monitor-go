# 第32课：观察原生 DB façade，而不替换查询链

[上一课：31](31-postgres-slow-query.md) · [课程目录](../README.md) · [下一课：33](33-crash-output.md)

[计划](../plans/32-db-facade-observation.md)补齐另一条真实 SQL 生产者。ORM 插件观察 GORM callbacks；DB façade 的原 Query/Tx 使用 CommonBuilder，不能靠解析原日志还原绑定值。

## 文件职责与一次调用

`adapters/db/factory.go`在业务第一次解析 DB 时调用原 driver callback，使用公开 BuildGorm 的连接缓存和 NewDB。同名物理连接状态只存 driver/GORM/logger/instrument；WithContext 保存请求上下文，不把上一请求 context 缓存进连接状态。

Table 创建原 NewBuilder，保留原 telemetry wrapper，再以五方法 CommonBuilder 观察器交给原 NewQuery。Where/Get/Insert 等链仍由框架编译和执行；没有重写查询 DSL。四个执行方法直接依次做：计时、原执行、安全准备、ReportSlowQuery、返回原结果；Explain 原样委托。直接 Select/Statement/Insert/Update/Delete 则观察原 DB 执行一次。

`adapters/gorm/sql.go`把真正共用的准备步骤提为 PrepareSQL：只接受已核标量/日期，SQLite保原问号绑定路径，Postgres用成熟 dialector Explain 生成展示文本。Raw保持原 SQL 身份，展示文本仍经原 SlowBuilder 脱敏与截断；不执行展示 SQL。

BeginTransaction使用原 NewTxBuilder与NewTx。观察器在真实 TxBuilder执行一次，Commit/Rollback和原txLogs仍由框架处理。回滚之前已执行的SQL仍是一条发生过的慢查询，不把监控记录等同于已提交业务。

## 资源与失败边界

Owner只拥有采集，不拥有连接池。Close先停止新采集，等待已经进入采集阶段的调用；超时保Stopping，释放后可再Close。它不等待仍在数据库执行的业务SQL；该SQL晚返回时不再采集，业务结果仍返回。采集故障、未知binding与诊断callback panic不覆盖业务结果；公开Status只存静态类别，不持SQL、error或payload。原框架日志仍保原行为，不能据此宣称所有native日志都已脱敏。

QueryxContext仅观察取得rows的同步窗口，后续迭代耗时/错误不在范围。当前公开适配SQLite/Postgres，真实服务覆盖另列，不能把同池SQLite测试称所有驱动部署通过。

## 实际验证

原Query/Tx首次因夹具未注册SQLite公开Provider失败，补Register后定向通过1.166s。Factory同池、请求Context、第二连接、原Trace次数及事务日志/回滚通过1.533s。Owner关闭/静态诊断两项0.516s；新增driver失败测试首编译误写Select参数顺序，纠正后新DB范围race通过6.540s。共用PrepareSQL受影响旧Postgres callbacks、SQLite callbacks与窗口三项race通过6.781s。全程独立SDK、自有SQLite/空env；没有真实Cloud、生产DB、全量或E2E。

Host60的enabled必要race8.042s通过，其余三个负例首轮通过；共享未接与临时注册编译失败分栏保留。root构建的私有PostgreSQL17.10（Unixsocket、无TCP）新增两路径集成0.944s通过：原公开Postgres driver/GORM Plugin与DB Factory各验证实际INSERT/SELECT/UPDATE、Commit/Rollback、原错误、每次native Trace一次、请求与连接。只创建并清理本次唯一表，不用生产库。首轮夹具遗漏database导致原BuildGorm返回nil，第二轮Raw断言过严，分别修夹具与断言后通过，未改产品。

这里必须区分三者：hash取未脱敏Raw输入；持久Raw也经过SQL/Secrets脱敏，敏感列的占位本身可能被遮盖；Last为成熟dialector生成后再脱敏的展示文本。验证保非敏感$n与目标SQL，而不要求持久Raw逐字等于输入。此服务实例验证不证明所有Host池、其他PG部署或驱动全部通过。

自主练习待完成：沿Table→NewQuery→ExecContext找执行与采集边界；说明tx回滚为何不撤销慢SQL；解释Owner.Close为何不关闭池；比较物理连接缓存与请求context；说明Queryx返回后尚未覆盖的错误。
