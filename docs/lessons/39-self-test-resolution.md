# 第39课：saved之后可选解决自检runtime

[上一课：38](38-release-reproducibility.md) · [课程目录](../README.md)

[计划](../plans/39-self-test-resolution.md)补齐PHP CloudTestCommand133–146的实际语义：解决失败只警告，自检是否成功仍以saved为准。旧第18课不支持resolve是当时边界，当前显式选项已可用。

## 主调用链

原SelfTest.Run保持签名，转RunWithOptions空选项，旧Client无需新增Resolver。需要解决时，SelfTestConfig显式给RuntimeResolver，SelfTestOptions.ResolveRuntime=true。

顺序是heartbeat → 构造固定runtime并intake → 严格saved → 可选resolve → slow intake。legacy aggregate、filtered、retryable或坏回执都不满足saved，绝不resolve。hash只能来自本次固定自检记录，note/by也是固定自检标记，没有任意业务hash或备注输入面。

selftest_resolution.go复用HTTPClient.ResolveRuntime成熟传输，再核返回hash/status/flag。Resolution单独给Applicable、Attempted、Resolved、Hash、Outcome；Result没有自由response/error/token/URL。Applicable仅说明runtime适用；缺Resolver是unavailable且未Attempted；Attempted表示调用Resolver，不证明HTTP已到达Cloud。slow-only为not_applicable，false为not_requested。

## 三层结果不能混为一句成功

Heartbeat、runtime/slow是否saved、可选resolution分别显示。403、timeout、坏resolve回执或Resolver panic只给安全Outcome，不抹去已saved，也不阻both继续slow。Resolve使用原ctx；父取消使后续slow未saved时仍返回原ErrSelfTest/canceled。没有后台补偿。

Resolved=false不是远端必定未改变的证明，响应丢失可能留下结果未知；不回滚Cloud状态或删除本地材料。此操作不写业务Buffer/ACK，既有重复自检仍沿固定Go身份与Cloud累计快照语义。

Host健康命令--resolve=false照常自检，true使用同配置Cloud token尝试。该token需要独立mcp能力；不探测权限、不借用MONITOR_MCP_TOKEN。CLI分别输出saved与resolution warning，403仍可主自检退出0，真正intake未saved则退出1。普通Host bootstrap的其它资源初始化不属于“自检不写Buffer”的保证。

## 实际验证与练习

SDK新HTTP12子类+boundary普通3.002s：真实httptest次序、固定hash/marker、not-saved零resolve、403/坏回执/timeout继续slow、父取消、nil/panic Resolver。未重复该12组。原Run严格saved旧pipeline单项0.419s，验证nil新依赖兼容。Host九模式真实CLI唯一race9.974s，覆盖默认/false/runtime/both/slow-only、403与坏resolve警告退出0、filtered/legacy失败退出1、原Cloudtoken/无MCPtokenfallback、隔离Buffer未创建。没有真实Cloud resolve、实际.env或token调用，也没有startup扫描。

自主练习待完成：区分Applicable/Attempted/Resolved；说明为什么必须先strict saved；解释403为何不令主自检失败；比较Client timeout与父取消；指出为何该CLI不能解决任意业务runtime。
