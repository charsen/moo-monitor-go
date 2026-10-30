# 第12步：真实GORM回调与显式Goravel实例接入

Monitor11已提交9f12bf1。本步先SDK adapters/gorm和真实私有SQLite ORM测试，Host正式allowlist/Provider后半另与implementation协调，当前不改40或其它Host文件。不解析字符串日志、不改module cache、不复制ORM，不接真实Cloud/生产DB。

## 公共能力与覆盖

锁定framework1.18的具体database/gorm.Query公开Instance()*gorm.DB，contracts orm.Query未保证；Install以能力接口接受已有query，无能力/空实例/非SQLite明确安全错误，不偷偷构造连接。GORM1.31.2公开DB.Use/Callback是真实生产入口，框架自己的telemetry GormPlugin也是如此。

六core anchors：gorm:create/query/update/delete/row/raw。processor.Execute先callbacks，再原Logger.Trace，再SQL.Reset与Vars=nil，所以after内可得原prepared SQL/Vars/Context/RowsAffected/Error。Logger.Trace的fc只给Explain SQL和rows，不能反推bindings。保原logger、tx.Error/RowsAffected与SQL返回，不调用fc/Explain第二次。

覆盖显式已安装实例的ORM CRUD/Exec/事务/WithContext clones。DB façade走sqlx Builder不经过这些callbacks，不覆盖；直接database/sql、未安装命名连接、Fresh后的新实例不自动覆盖。Connection标签由owner显式给，不能从DSN或可能旧dbConfig的Orm.Name缓存分支猜。

本步只SQLite问号placeholder；Postgres$1等明确不支持，不能ReplaceAll假装兼容。未知Vars不调用Valuer/Stringer，窄转换后复用11 Input，暂不可表示值给安全诊断，原SQL结果仍返回。Source QueryExecuted只在成功执行/重试恢复后发；执行错误跳过，gorm.ErrRecordNotFound属成功SELECT空结果应允许，DryRun不采。事务内成功SQL即使随后rollback也已发生，按源仍采，不称提交后事件。

## 最小API与生命周期

adapters/gorm（package gormcapture）：New(Config)→Plugin；Config明确Connection、窄Recorder（真实SlowBuffer）、Clock、context→slowquery.Request resolver、OnError、CaptureTimeout。Clock默认time.Now，CaptureTimeout零值默认100ms，负值拒绝；只派生采集ctx，不改Statement.Context/业务超时。文件syscall不可硬中断，不能承诺硬实时上限。

Install(query,plugin)只拿已有Instance；启动/无并发查询窗口安装。内部安装mutex只串行注册，绝不以全局busy丢并发SQL。同Plugin指针在同注册表幂等，不同owner固定名已占报静态错误。Initialize预核六anchor与12固定名字；逐项注册失败只反向移除本次项，原有同名项不删，不支持不假称成功；Close(ctx)禁新采集并以同采集超时/owner ctx有界等已入采集，不运行期Remove/Replace callback。借用DB/SlowBuffer，不关闭它们、不flush。

计时state仅同步Statement.Settings，after取出即删；转成11窄Input/复制所需值，返回前不保Statement/Request。每个query自己的state，不用字符串消息去重、不跨请求存全局身份。原始SQL中的?保源展示算法限制，不把展示SQL当可执行SQL。

自动采集失败不抛回SQL链，不AddError；安全诊断仅静态分类/计数，任意Recorder cause或panic文本不保存。OnError使用internal派生ctx避免递归SQL/日志，回调自己panic可观测；不将诊断回调纳入Close等待。回调必须响应ctx，SDK不造无限goroutine强行终止任意用户代码。

## 文件与依赖

新增adapters/gorm/{plugin,binding}.go及定向测试、12两正文/入口。生产使用既有gorm1.31.2（变直接require不升级）；测试固定goravel/sqlite1.18.0，实际manifest为go1.25/framework1.18/gorm1.31.2、ncruces sqlite.25/gormlite.24/wazero1.9/julianday，xsys源.46低于现.49；get/tidy后审实际图，不复制replace/不追latest。SDK最低Go1.26、实测1.27.3。

## 最小验收

自有0700目录内SQLite文件/内存，通过公开构造取得真实Goravel ORM Query与Instance，不测试替私有成员。CRUD/Exec/事务/WithContext/raw&row六类、原native Trace一次、threshold/skip、Request当前标量、同实例重复/异owner拒绝、双连接、Closed/canceled/Reporter故障不改SQL结果；DryRun/真实SQL错误不采，SELECT无行可采，rollback不抹已发生SQL。DB façade负覆盖公开说明。并发真实查询才一次定向race，无全量/E2E/真实Cloud。

结果未执行不填通过，用户练习待完成；Host后半文件与默认关闭开关已审采用独立43业务预热，待其实现验收，不能监控一开就隐式新建DB连接。

## 已验证的SDK节点

2026-10-30，GOWORK=off，Go1.27.3：真实六回调/after hook嵌套/事务/关闭两项PASS1.369s；补安装回滚、过滤/双连接/context、失败隔离、16并发、Close、binding七项PASS1.284s；10项定向race最终PASS6.906s。首轮Update参数错误与driver Json未Register失败已修，诊断测试Row未Scan占单连接的等待问题按internal验证路径修正，详见教程。无全量/E2E/真实Cloud。

原Trace调用一次/业务结果保留，但其计算位置在回调后，耗时包含同步监控开销；本插件Took在collect入口截定。安装回滚仅启动窗口，Close不卸载。SDK依赖实际回读framework/sqlite1.18、GORM1.31.2、dbresolver1.6.2、xsys0.49、cpuid2.4；mysql sum来自dbresolver.test，不代表运行MySQL。

Host后半待审：公开框架无新连接created hook，不能Provider主动解析Orm以监控名义造连接；Host43已批准DB_PRELOAD_CONNECTIONS默认[]独立业务预热，在单线程Boot拿已有query后才显式Attach；非空列表必须默认连接首项，监控开关不创建连接。仅测试Attach不等于全Host生产自动接线。Host42已分配Schema，本课不占其文件。

额外公开NewDB→Table.Count负覆盖PASS0.978s，原业务count正确且插件计数不变；与前10项race证据分开。

最后回滚尾部安全原因收口后，TestInstallationRollback必要复验PASS0.969s；前6.906s为该分类补强前race，未重复并发。
