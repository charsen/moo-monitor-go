# 第36课：独立控制日志来源和级别

[上一课：35](35-cloud-summary.md) · [课程目录](../README.md)

[计划](../plans/36-log-capture-policy.md)对应PHP exception.log_context_hook、log_message_hook与共用log_context_levels。它只控制监控采集，原业务logger规则仍由delegate决定。

## 一条日志的顺序

NewDriver仍为原三参数，内部使用nil策略，保持所有level>=Error的旧捕获行为，包括自定义数值level。NewDriverWithPolicy显式接收LogPolicy，复制bool和级别集合到私有map；调用者后续修改slice不会改变运行中的策略。显式空集合表示不采，非法Go级别返回静态配置错误。

Handler.Enabled取delegate启用或监控可能采集的并集。Handle先按原delegate.Enabled调用原Handle并保返回error，然后检查内部诊断标记与级别。只有入选级别才读取typed ExceptionKey判断来源；有真实error只走context，无真实error才走message。关闭context不会把同entry降级为字符串重新采集，这是PHP两hook已有的排除规则。

来源过滤发生在复制请求/读取frames/登记Scope事件之前。因此被过滤的同EventID不会占去容量或挡住之后真实Capture。既有安全诊断与内部ctx防重入不变；没有用error.Error/类名/日志文字猜异常身份。

## Go级别与Host配置

锁定Goravel公开级别是debug、info、warning、error、fatal、panic；PHP的critical/alert/emergency没有独立Go枚举。本Host显式默认采用error/fatal/panic，warn输入别名归一warning。SDK nil则保旧>=Error，与显式只接受已知级别的策略有意区分。Fatal/Panic仍可能执行原框架os.Exit/panic，本课没有阻止这些业务语义。

Host总capture.logs开关仍在；context/message只是其下的独立选择，不开启HTTP/队列或其它采集。原sink未被全局脱敏，这些策略也不是任意业务日志自动去重。

## 实际验证与练习

SDK首两项1.181s；新两项和原Writer三项必要race2.637s通过。真实Goravel Writer验证来源开关、warning入选、空集合/全关、策略不可变、非法级别、同ID过滤后Capture、nil旧级别及原delegate错误/消息保留。Host实际配置七模式唯一race9.520s通过，逐项验证来源/警告级/空集合/全关/无效配置静态失败及原native三条日志。总Logs关闭时不解析子policy，沿既有开关边界；未扩大旧HTTP/payload全组。

自主练习待完成：为什么禁context不能降级message；对比Enabled与Handle职责；解释空集合与nil的区别；说明为什么筛选应在Scope之前；列举此策略不能提供的日志秘密保护或异常身份推断能力。
