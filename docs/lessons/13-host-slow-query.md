# 第13课：真实Host暴露的慢SQL适配边界

[上一课：12](12-gorm-slow-query.md) · [课程目录](../README.md)

第12课证明了显式安装的SQLite实例可观察后续SQL。本课让真实Host业务消费它，并补实际需要的请求快照与标准模型类型；Host完整教学为第43课，SDK先提交，所以这里不链接尚未进HEAD的Host课文。[计划](../plans/13-host-slow-query.md)记录两仓证据。

## 一次认证请求怎样进入SQL记录

Host HTTP middleware拥有有限生命周期state：请求开始只复制method/url/ip，事件发生时才解析当前typed Subject。此前日志Handler已消费这个state。新增包级[HTTPRequestSnapshot](../../adapters/goravel/http.go)让SQL成为第二个真实消费者，两者都调用同一个copiedHTTPRequest，没有第二份规则或请求缓存。

函数返回复制的URL/IP/身份标量；调用方修改副本不会反写原状态。非HTTP context、已经关闭的scope返回exists=false，不能在后台拿旧context重新读Request。身份解析失败交给安全诊断，不改业务响应。SQL只映射method/url/ip/user_id/user_name，Guard并未加入原SQL协议。

Host Food已有WithContext(ctx)，无需全局猜用户。真实Authentication后执行Store/Update/Show，SQL同步回调从当前ctx读取，原scope退出后清理。未安装HTTP生命周期时只能明确无快照，不能把随意ctx伪造成HTTP。

## Store成功与INSERT采到是两回事

初次Host测试中，Store201成功、SELECT样本存在，但INSERT目标断言失败。真实Vars的类型包括uint8、*int64、*string、Carbon DateTime及gorm.DeletedAt。若只断“SQL总数大于0”，这个缺口会被名称唯一查询掩盖。

[binding.go](../../adapters/gorm/binding.go)因此按实际类型补：

- 原生int8/16/32、uint8/16/32和范围内uint/uint64精确映射int64；超过MaxInt64失败无safe输出，float32须有限才转float64。
- 锁定framework carbon.DateTime值/指针读取Carbon状态与StdTime，保留时区；nil/zero/empty归null，错误从严拒采。
- 锁定gorm.DeletedAt值/指针仅按Valid/Time转null或time.Time。

这不是通用Valuer白名单。任意别的对象、别名、Stringer仍拒绝，不为采集调用Value/String。数据库驱动与原Logger本来的方法调用保留：独立真实SQL测试比较安装前后次数，并断业务两次写入都成功，证明监控没有额外调用用户方法。它不能替代标准Food写链确实采到的验证。

源码依据是锁定Carbon `type_layout.go:Value`与GORM `soft_delete.go:DeletedAt.Value`。已知有效日期读取与其数据库绑定同义；错误日期安全拒采属于明确边界，不概括成任意用户日期类型都兼容。

## 生产安装不靠监控主动造连接

Host独立业务名单DB_PRELOAD_CONNECTIONS默认空，按真实配置校验唯一名字、非空默认连接首项；得到已有query后才按监控allowlist安装。原ORM仍拥有池，monitor只拥有插件/本地SQL Buffer，不自动同步Cloud。

实际源码还揭示原DatabaseProvider.Boot为命令注册调用MakeSchema/Seeder，已配置默认DB时原框架就可能建连接。正确结论是“监控开关不新增或预取连接”，不能把默认DB已配置的进程写成完全零连接。默认配置空时验证无DB；已配置时对比原生baseline，再验证无额外连接/Attach。未知名单的拒绝只约束本次新增预热动作，不擅改框架启动流程。

Close先停止新采集；已入采集没排空时可返回deadline/canceled，保Buffer让owner重试。只有资源真正收尾才Closed；Stopping不是关闭完成。插件不在运行期Remove callback，不关业务池。Host用真实私有flock阻塞一次Record，验证取消Close后Buffer仍能读、放锁排空后重试成功。

## 文件与真实结果

| 文件 | 本步职责 |
| --- | --- |
| [http.go](../../adapters/goravel/http.go) | 只读当前生命周期快照，复用已有实现 |
| [http_test.go](../../adapters/goravel/http_test.go) | late身份、副本、不持有关闭scope |
| [binding.go](../../adapters/gorm/binding.go) | 原生宽度及精确已知日期归一 |
| [binding_test.go](../../adapters/gorm/binding_test.go) | 数值边界/溢出、日期null/error/时区 |
| [lifecycle_test.go](../../adapters/gorm/lifecycle_test.go) | 实际未知Valuer故障隔离、不重复调用 |

2026-10-30，SDK GOWORK=off、Go1.27.3：最终6项GORM race通过6.719s，4项Goravel race通过2.345s。前轮0.796/0.891/0.517与首次unknown Valuer1.544为阶段证据，最终精简fixture后统一结果以前两项为准。

Host实际四module workspace单列：认证Food INSERT/UPDATE/SELECT及故障隔离两模式2.071s，修正原生默认DB基线后的10负例+旧41默认stack组合5.547s，真实在途Close重试race7.606s。BasePath最后只复验受影响active，通过1.769s并断相对app/food/...；没有把SDK race当整个Host或真实Cloud通过。

本步未改go.mod/sum。开发workspace不是已发布依赖解析，Go Context也没有伪装PHP版本。SQLite已安装实例覆盖不等于DB façade/所有驱动/全部lazy连接、Rows迭代、完整M矩阵已完成；未跑全量/E2E/真实服务。

## 自主练习（待用户完成）

画出HTTP私有state→复制标量→SQL五键的边界，解释为什么关闭scope后不应该再拿身份。再说明Store201与INSERT记录断言的区别；预测任意Valuer为何不能为了“兼容”直接调用。最后区分Stopped/Closed/close失败后重试、业务连接所有权与Cloud同步权限。代理测试不代做学习验收。
