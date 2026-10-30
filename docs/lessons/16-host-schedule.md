# 第16课：让Host真实Schedule消费调度观察器

[上一课：15](15-host-queue-owner.md) · [课程目录](../README.md)

本课对应模板Host第46课的真实采用；第14课已有适配器，当前一步处理谁创建Schedule、事件何时注册、谁等待任务关闭。SDK没有新生产API，也不复制cron。第15课已独立提交；Host采用以模板第46课课号定位，与SDK独立提交顺序分开。[本课计划](../plans/16-host-schedule.md)记录范围。

## 一个原生实例，两个独立开关

Host的`MonitorScheduleServiceProvider`嵌原Provider保留Relationship/Boot，业务owner关闭时原Register/Runners原样委托。业务owner开启才替同一Schedule binding的factory，以原`NewApplication`和单一cron创建受管理实例。`MONITOR_CAPTURE_SCHEDULE`只选是否给这个Schedule专用ScheduledArtisan，不替全局Artisan，不因监控开关注册任务。

事件默认空，教学command默认不注册。真实项目可删除教学资产，再通过明确登记的安全signature和具名callback使用同一基础接线，不需要Food依赖。未启用监控或Runtime初始化失败时，任务仍照常执行。

## 包装的是事件执行，不是重写调度规则

Host Event外壳嵌原Event，只覆`GetCallback`。原getter每次执行读一次，callback按显式`GetName`观察；cron、command、OnOneServer、Delay/Skip等metadata保原值。不可信名称给静态诊断，业务callback仍运行。

原Artisan返回error时传真实error。原callback panic采后仍repanic原value，由原cron.Recover接收。未知退出码不造1，最终`meta.exit_code`缺键。已知外部完成值通过显式`Complete`；本课验证层是本次测试二进制真的退出2、父进程读取`exec.ExitError`，不是任意生产CLI已自动捕获。`CallAndExit`/真实`os.Exit`未拦截。

原调度器可能日志化完整command/error或cron stack，因此Host专用logger只对已核精确模板与参数形状静态化，再带内部context交回原sink。普通业务日志与相似非模板日志仍透传，不是全局内容过滤。它也不能推断无共享EventID的业务日志与typed异常是同一事件。

## 为什么Run结束仍不能立即关闭Buffer

原cron.Stop可使Run先返回，在途callback却还没完成。Owner的Closed必须同时得到native.Shutdown成功和Run done；第一次Close超时后第二次仍须重新等待native排空。否则监控Buffer可能先关闭，末尾失败任务就无法采集。

Close先设Stopping，阻止尚未开始的Run。若Stop早于原cron真正启动，短间隔重复Stop等done；不加假探针任务、不反射cron。Runner最后用Background等待真实排空，任意永不返回的业务callback仍需应用自行终止。Owner保原goravel:schedule名称使app.disabled_runners原键继续有效；初始化业务owner失败静态可见，不当作监控故障吞任务。bool沿框架GetBool，默认false。Host已核priority3先于queue5/auth10/monitor200，不表示未来第三方自动符合。

## 证据与练习

Host工厂真实原Artisan/cron执行了监控开关两模式，普通与原生内部日志边界通过2.180s；关闭修复与真实在途callback重试1.010s。metadata、成功不采、真实退出码和采集失败保原panic等三个同package项race3.745s。getter/不可信名称补强单项race1.378s；真实bootstrap七模式4.373s，最终生命周期/七模式/owner失败四命名项race13.626s。测试只启动真实Schedule Provider的原生Runner，保全局Artisan与原业务sink；不是生产部署或SDK独立发布解析验收。无真实Cloud、E2E或全量测试。

用户练习待完成：画出业务开关/观察开关矩阵；说明未知exit与已知回执区别；解释原Run done为何不代表任务排空；为一个可移除的具名本地任务选择安全signature。代理验证不代替学习完成。
