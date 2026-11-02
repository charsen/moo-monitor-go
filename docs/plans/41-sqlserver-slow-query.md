# 41 SQLServer双入口慢SQL观察

40/Host69独立验收后继续。范围锁定goravel/sqlserver1.18.0、gormsqlserver1.6.3、framework1.18.0、GORM1.31.2；依赖由唯一writer固定get/tidy并核MVS。无真实SQLServer/用户DB/Cloud访问，无全量/E2E。

## 已确认设计边界

SQLServer使用@pN，原Raw仍拥有hash身份，展示先安全bindings归一再由成熟dialector.Explain产生；独立4MiB预算在Explain前核重复编号放大。未知Valuer/Stringer不再次执行，展示仍经既有双mask/cap且绝不执行。

必须先读锁定dialector的create/update替换实现，确认实际SQL执行与Statement读取窗口，再用真实GORM六callback+sqlmock证各一次记录和原Trace；不从MySQL默认callback行为推断SQLServer。DB façade继续原BuildGorm/Query/Tx及5direct路径，官方Pool映射与sqlmock覆盖层、版本/连接初始化旁路明确记录。

验证最小范围：新SQLServer展示/六callback/context/commitrollback/error/cancel/Close/Valuer，与原native DB同池/QueryTx/5direct/原Trace；共享PrepareSQL变化仅保必要已有回归。Host70仅消费者和可选接线，不选择默认连接或开启监控。

两driver独立commit后固定新SDK/scaffold/Host三SHA，只做一次Go1.26生产构建；旧38两工具链报告不改、旧全套测试不重跑。实际证据见下方实测分栏。

## 已核锁定源码

官方固定tag [Initialize/Explain](https://github.com/go-gorm/sqlserver/blob/v1.6.3/sqlserver.go)、[Create](https://github.com/go-gorm/sqlserver/blob/v1.6.3/create.go)、[Update](https://github.com/go-gorm/sqlserver/blob/v1.6.3/update.go)：Initialize替换同名create/update；Create内部执行并保留Statement，Update委托原callback，因此原before/after窗口仍可观察，不复制执行器。Explain会修改bool为1/0，只允许已归一独立slice进入；每个@pN引用先预算。

官方[goravel/sqlserver1.18.0](https://github.com/goravel/sqlserver/blob/v1.18.0/sqlserver.go)的Pool映射真实执行；Grammar/Processor可直接委托，不额外探测版本。fixture只覆盖transport dialector为sqlmock；BuildGorm/NewQuery/NewTx仍为原框架。实现文件为adapters/gorm/sqlserver.go、sqlserver_render_test.go、sqlserver_test.go及adapters/db/sqlserver_factory_test.go；共享入口只扩明确dialect分流和支持名单。Host70只补官方Provider/可选via与最小双入口消费者。

## 首真实回调发现并修复的隐私缺口

首GORM组失败在敏感token创建展示：SQLServer生成INSERT(columns) OUTPUT INSERTED."id" VALUES(...)，原PHP式INSERT masker只识别紧邻VALUES，故未遮敏感列。不是测试换列绕过。经design_review确认，同41最小扩展mask/sql.go有限语法：OUTPUT INSERTED.标识符的逗号列表（双引号/方括号/裸标识符），保原column→VALUES映射与所有捕获索引；不接受函数、表达式、子查询或alias。这是源算法之外的SQLServer语法安全加固，不宣称通用SQL解析或所有MERGE/触发器形状均已覆盖。

mask/sqlserver_test.go验证新形状多列/多行/字段顺序与不支持片段不误认；原PHP sourcefixture保留并通过。首次机械插入未命中正则，新test仍失败，纠正实际正则后源fixture+新mask0.344s通过；失败六callback修后0.732s通过。首SQLServer DB Factory1.213s通过；首GORM全部组只有创建脱敏断言失败，展示/事务/错误取消/Close/Valuer组无另失败，未重复这些已过组。

固定get/tidy由writer完成，实际MVS核goravel/sqlserver1.18.0/gormsqlserver1.6.3/mssql1.9.1，framework1.18/GORM1.31.2/xsys0.49未变；新增civil/sqlexp间接及真实tidy checksum，不是全图升级。

## 公共掩码四层引用核查

按当前源码定向rg核MaskSensitiveSQL/SanitizeStoredSQL：①SDK内直接调用为records/records.go的SQL文本净化、slowquery/builder.go的Raw/Last双mask，migration/conversion.go间接复用同存储净化；②当前Host engine与scaffold源码/文档无直接该方法调用，Host通过SDK Buffer消费，新Host70补消费者集成；③无接口/方法签名、实现类或容器绑定增删，既有Masker.New依赖未变；④原mask/sql_test.go的PHP fixture与新SQLServer测试共同验证，旧02/25/31教程只解释原有限算法，41显式记录本次源外语法扩展，不改历史证据。未删除符号，不触发删类审计脚本。

未知OUTPUT表达式仍可能保留敏感值；有限mask不提供任意SQLServer语法的全面安全保证，部署前应避免直接采集未覆盖原始SQL或先补对应语法验证。

Host70新消费者单项race2.317s通过：官方Provider/可选via注册、Create OUTPUT回填ID及共享mask实际消费、DB Select结果、同池双入口各一次及native Trace两次、连接/身份均有断言。owned空env/环境白名单、默认DB空、Cloud/owners关闭、GOPROXY/GOSUMDB关闭；LOG_PATH用owned相对路径，未执行生产via/真实服务。SDK未因Host单项补跑已通过组。
