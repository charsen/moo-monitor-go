# 36 日志来源与级别策略

沿PHP log_context_hook/log_message_hook及共用log_context_levels，加入显式LogPolicy；不实现Host负责的dontReport类列表。旧NewDriver三参数用nil保原>=Error，新增NewDriverWithPolicy接可选策略：context/message独立bool，已知Go级别集合，空集合不采；非法level静态构造error。

先原delegate Handle，再只按级别/typed ExceptionKey选择来源。明确error只走context，禁context不降级message；普通字符串才message。过滤在request/frames/ID dedup前，不占Scope事件。delegate Enabled OR可采级别，保原delegate返回及安全诊断。

Go六级debug/info/warning/error/fatal/panic，不造PHP critical/alert/emergency枚举。Host默认两hook true与error/fatal/panic；warn归一warning；新公共配置/runtime由writer统一。仅真实Writer新策略定向/必要既有Writer消费，不全量/真实Cloud/E2E。实际结果见下方，练习待用户完成。

## 实测

SDK首两项1.181s，新2+原Writer3必要范围race2.637s通过；构造兼容/不可变map/来源不降级/同ID过滤前置有实际断言。Host63共享配置/Runtime接入后，七模式唯一race9.520s通过：默认、context-only、message-only、warn别名、空集合、双关、无效level，所有原native日志保留。
