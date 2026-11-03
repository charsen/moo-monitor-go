# 40 MySQL双入口慢SQL观察

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

范围：SDK真实MySQL GORM dialector与Goravel DB façade，Host69消费者及默认关闭配置。固定goravel/mysql1.18.0、gormmysql1.6.0、framework1.18.0、GORM1.31.2；不连接用户数据库、真实MySQL/Cloud，不跑全量或E2E。

## 已采用设计

Install/Initialize与DB Factory同时支持mysql；不是仅增名字白名单。PrepareSQL调用mysqlRendered：原Raw保留作为hash输入，bindings先复制已支持标量/Carbon/DeletedAt/time，未知Valuer/Stringer安全拒采。真实dialector.Explain只生成展示SQL，4MiB独立预算在调用前保守核算，随后仍走既有双mask与存储cap。展示SQL绝不执行，不改变业务绑定或云端Record结构。

锁定gormmysql Initialize公开代码注册默认callbacks，并未替换gorm:create/update。插件仍包围核心执行且在save_after_associations/preload等后处理前读取Statement。六callback fixture执行真实dialector生成的SQL，SQL传输由sqlmock替代。

DB fixture从官方NewMysql(config).Pool取得配置映射，仅将Pool.Writer.Dialector换为带sqlmock.Conn的成熟dialector；官方Grammar()会额外探测服务版本，因此fixture使用公开NewGrammar固定8.0.36，未执行该探测。真实BuildGorm、NewBuilder、NewQuery、NewTx仍走原实现；测试driver仅提供受控Pool/Grammar。官方ServiceProvider显式注册以满足Grammar对App的依赖。

## 文件与验证

- adapters/gorm/mysql.go：安全展示与预算；sql.go：共享PrepareSQL分流。
- adapters/gorm/mysql_test.go：六callback、native Trace各一次、身份、rollback、原error/cancel、Close、Valuer仅native调用一次。
- adapters/gorm/mysql_render_test.go：引号/日期/空值与非法类型、UTF8、预算。
- adapters/db/mysql_factory_test.go：原业务同池、5direct methods、Table Query、Context/Connection缓存、事务日志延迟、commit/rollback、error/cancel、关闭仅停观察。

实际定向：GORM MySQL组1.130s通过；Factory0.759s通过。首轮render fixture误用Carbon构造参数编译失败，改为官方FromStdTime；Factory首轮误判原SQL大小写/quoted格式，改断言匹配真实原生输出；另发现官方Grammar需Provider.App，补显式Provider注册后通过。无失败被吞。

依赖由唯一writer固定get/tidy并回读MVS；现有framework/GORM/xsys版本未漂移，新增SQL driver1.9.0、semver3.5.0；sqlx/dbresolver仅真实测试引用导致改direct，不升版本。Host69消费者单项race2.365s通过：正式Provider/可选via注册、同池双入口各一记录/native Trace两次；owned空env+默认DB空，不执行生产via，未连接服务。

必要共享回归与新MySQL范围race通过：adapters/gorm 7.137s、adapters/db 6.858s；范围仅新MySQL、旧SQLite callbacks/window、原Query/Tx与PG展示，未运行全量测试。
