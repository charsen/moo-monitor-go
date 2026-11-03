# 31 Postgres成熟dialector执行回调

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

仅SDK。使用锁定gorm postgres1.6.0/GORM1.31.2与sqlmock固定假执行，不安装/连接真实PG。Plugin只放开明确postgres实例，原6callback/业务结果/native Trace/Close不替换；Host或真实PG部署不冒称已验证。

Raw保原$n用于hash；Vars先现bindings归一为不可变scalar/time，拒任意Valuer/Stringer。随后成熟Dialector.Explain生成展示SQL，通过Input.RenderedSQL传给Builder；它与Bindings互斥，nil继续旧?填值。展示SQL绝不执行，不把Explain当wire反编译；quoted-dollar/索引展示限制按实际测试记录。

RenderedSQL UTF8/4MiB上限，Accept在threshold/skip后、dailycap冻结前校验；统一SanitizeStoredSQL双mask/cap。渲染前预算避免重复占位符放大，不新造SQL执行器。pgx.QueryExecMode只允许首项已知枚举并剥除，未知拒绝，不能Stringer。

预期唯一writer get/tidy：gorm.io/driver/postgres1.6.0、sqlmock1.5.2（官方固定release）、pgx5.7.2对齐现Host；保GORM1.31.2/framework1.18/xsys0.49，不追latest。新6callback/事务/WithContext/失败/取消及SQLite必要旧组定向，实际结果见下节。

## 实际执行证据

固定get/tidy由writer完成，MVS核postgres1.6.0/sqlmock1.5.2/pgx5.7.2/GORM1.31.2/xsys0.49；mod+6/sum+13，无旧版漂移。sqlmock固定tag依据：https://github.com/DATA-DOG/go-sqlmock/releases/tag/v1.5.2 。

纯Rendered首测0.707s；新adapter普通0.816s（同条误传-env给纯slowquery包导致整命令失败，未记整体通过）。正确分包旧Slow sourcefixture/safety+Rendered0.409；SQLite callbacks/window/rollback+冻结Rendered无写1.331。新Postgres范围race1.936s，包含六callback/真实public Instance消费、WithContext/事务/错误取消/Close/无额外Valuer。public Instance来自实际framework.Query，测试SQL由成熟GORM+sqlmock执行；未验证真实Goravel PG池/Host部署/PG服务，不安装或连接它们。

Quoted-dollar按成熟Explain有限行为公开，展示SQL绝不执行；Raw/hash不取绑定值。Source Record schema/Cloud字段不变。用户练习待完成，后续真实PG验证保持外部待项，不阻本地队列。

## 第32步后的真实PG补证

后续在root独立构建的PostgreSQL17.10私有0700 Unixsocket实例，以真实Goravel Postgres公开driver运行GORM Plugin与DB Factory两路径单项0.944s通过。实际参数化INSERT/SELECT/UPDATE、Commit/Rollback、原SQL错误不采、原Trace一次、请求/连接和脱敏目标记录均有断言。测试唯一表已DROP/池关闭，root已停服务并确认PID文件消失；没有读真实.env或连接用户DB。原本课sqlmock-only时点证据保留，当前不再笼统称未验真实PG。尚未验证正常Host部署配置、TLS/权限模型、生产及更多类型/Rows迭代窗口。原Raw作为hash输入，持久Raw仍脱敏，不能要求敏感$n位置逐字保留。详见[32课](../lessons/32-db-facade-observation.md)。
