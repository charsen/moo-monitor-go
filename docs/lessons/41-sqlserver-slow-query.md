# 第41课：SQLServer替换回调后的观察窗口

[上一课：40](40-mysql-slow-query.md) · [课程目录](../README.md)

本课在[计划](../plans/41-sqlserver-slow-query.md)范围内补SQLServer，两入口仍共享PrepareSQL与SlowBuffer，测试传输只使用sqlmock，不连接SQLServer服务。

## 同名callback中的真实执行

固定[sqlserver1.6.3 Initialize](https://github.com/go-gorm/sqlserver/blob/v1.6.3/sqlserver.go)先注册默认callbacks，再替换gorm:create/update。名字还在不代表实现相同，所以不能只看安装成功。

[Create](https://github.com/go-gorm/sqlserver/blob/v1.6.3/create.go)在替换函数中生成Statement，执行ExecContext或带OUTPUT的QueryContext并Scan，返回时SQL/Vars仍存在；[Update](https://github.com/go-gorm/sqlserver/blob/v1.6.3/update.go)增加自增主键omit后委托原Update callback。插件沿原同名before/after注册，窗口仍包围执行，在save_after_associations前读SQL。实际六callback测试使用成熟dialector执行并检查自动ID、RowsAffected、记录与原Trace各一次，不能只靠函数名推断。

adapters/gorm/sqlserver.go仅负责显示值安全归一与预算；plugin.go仍负责执行时钟/生命周期；sql.go把两生产入口接到同一PrepareSQL。没有复制SQLServer执行callback。

## @pN与原始身份

BindVarTo使用@pN。sqlserverRendered先复制支持值，再按每个编号引用计算预算，重复@p1同样重复计费，避免Explain前放大；@p10按对应位置取值。成熟Explain会把bool转成1/0，因传入的是新归一slice，不会修改调用者参数。

Raw作为原参数化hash输入；RenderedSQL只作展示，仍独立受4MiB、UTF8及既有双mask/cap保护。成熟formatter也会替换引号里的@pN，不是wire反编译器。未知Valuer/Stringer安全拒采，不为监控再次执行；展示SQL绝不执行。

## 原DB façade不换执行器

官方NewSqlserver的Pool映射及Grammar/Processor公开委托是真实的，只覆盖Pool的transport dialector以注入sqlmock。它的Grammar不需要MySQL式额外版本探测。BuildGorm复用同池，5direct方法、NewQuery、NewTx仍按原Goravel路径执行。原结果/error/Context、事务提交日志时机及Close只停观察的边界不变。

真实服务的TDS、TLS、权限、驱动编码、OUTPUT/MERGE触发器和多池行为仍待授权验证；sqlmock的OUTPUT结果不能证明这些部署行为。

自主练习：指出Replace前后为何同名仍需读源码；追踪一次Create的OUTPUT到Record；预测@p1重复/@p10及bool参数原slice；区分已执行后rollback和最终提交；说明真实Pool映射、sqlmock传输与实际SQLServer服务各证明什么。代理结果不代做用户学习验收。

## 首次执行为什么必须看持久记录

六callback首测发现SQLServer的INSERT OUTPUT列列表隔开了columns和VALUES，原有限PHP式INSERT masker没识别创建token。监控不能因为“记录成功”就算完成；本课扩mask/sql.go只接受已核OUTPUT INSERTED.标识符列表，保原敏感列位置算法。新定向正反例与原PHPfixture0.344s通过，真实六callback修后0.732s通过，DB Factory1.213s通过；没有改数据逃避失败。未支持的表达式/函数/子查询/alias仍不当作已理解的INSERT，有限mask不等于所有SQL均已脱敏。

文件职责因此增加mask/sqlserver_test.go的源外语法回归；SQLServer展示helper没有第二份敏感列规则。依赖固定与失败修正记录见计划。未运行真实服务、全量或E2E。

未知OUTPUT表达式仍可能保留敏感值；有限mask不提供任意SQLServer语法的全面安全保证，部署前应避免直接采集未覆盖原始SQL或先补对应语法验证。

Host70的新消费者单项race2.317s通过，真实Create OUTPUT回填ID并检查本次mask加固，DB Select/同池双入口与原Trace均符合预期；正式可选连接已注册但生产via未执行。测试owned空env、默认DB空与Cloud关闭，不替代真实服务验证。

## 两driver后的单次Go1.26生产构建

固定SDK `80ac77741b6596f2ccee7837a7854935cfa8b876`、scaffold `321580afc8dbf95989e2a0843f2a1093e9b983c3`、Host `fdd747c34b2bc0561a0888d4ea911c78cc62612f`，已读工具只archive这三提交，未混Designer未提交代码。新[driver最低线报告](../contracts/fixtures/release-reproducibility-drivers-go126.json)记录Go1.26.8/darwin-arm64/CGO1生产Host111248146字节、MCP9169202字节编译通过，均未执行；ownedroot清理通过、无replace的私有模块仅从ownedproxy加载。

仅临时fixture的MVS补flock0.13.1并按SDK需求将Host声明的x/sys0.48选择为0.49；源manifest未编辑，差异完整保留于report。旧38及P1报告未改，未重复双工具链/旧测试；本报告是独立本地解析构建证据，不是远端发布、tag鉴权、真实服务或目标部署证明。
