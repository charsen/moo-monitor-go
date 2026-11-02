# 29 明确过滤与可选HTTP payload快照

SDK独占capture/filter及capture窄接入、Goravel HTTP/logger真实消费，不改records/codec/buffer或25迁移。过滤在scope登记/tag之前；HTTPStatusCode能力<500跳过、5xx保留；ConsoleInputError仅CLI默认跳过，CLIExperiment只有显式标记且CLI才跳过。未知错误不靠文本/类型名猜。源依据ExceptionDispatcher与Recorder.shouldReport。

HTTPConfig.Payload可选，nil默认关闭；请求入口仅POST/PUT/PATCH/DELETE，All读取已解析数据后MaskPayload有界深拷贝，状态只存脱敏副本，关闭清空，日志/5xx/panic共用。默认200chars，MaskKeys沿现Masker，非法值安全诊断并忽略payload，不改业务响应。Gin All包括route且JSON数值已float64，原精度不可恢复；框架getHttpBody自身先读body，SDK不再直接读流/文件。

仅自有Gin/Buffer fixture，验证JSON/form/multipart、秘密不落/原handler不变、首日志+5xx同ID、未知值/预算/过滤panic。Host57可选开关由writer共享接线，首SDK闭环后进行。实际结果待实测，用户练习待完成。

## 实测收口

过滤首测0.861s；adapter首轮unused import编译fail已修，新边界0.858s。必要范围race capture4.844/adapter5.278。终读改只单unwrap、HTTP100..599有效以及归一5000预预算；仅受影响最终capture0.630/adapter0.338，原race标修前不重复。

Host57七模式首轮bad-json误期望decoder强制500导致包1.634s失败，其余六子例完成；源码仍让原handler503，修测试后唯一bad-json0.900s。产品未改绕过。自有空-env/fake APP_KEY、清MONITOR继承及关闭所有Cloud/DB/owner，无真实.env读取；自有日志清完。未知对象安全省略，已知FileHeader只静态类型标记，普通业务/响应与原logger继续。

Host终读统一有效cap：开启payload时显式0归一默认200，并把同一值同时传HTTP与records.Config.StringMaxChars，避免Builder二次保护又截回200。仅large-cap(500完整保320字符)/zero-cap两实际落盘子例最终0.933s；此前七模式与SDK不重复。SDK直接组装者也须协调HTTP与Builder的各自限制，不把更大的入口cap当绕过最终Builder保护。
