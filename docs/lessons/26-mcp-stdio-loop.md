# 第26课：让stdio只承载MCP协议

[上一课：24](24-migration-inspection.md) · [课程目录](../README.md)

本课提供独立 `mcp.Server`，具体工具留给Toolset。它能协商版本、列出和调用假工具，不表示七个Cloud工具、分页、私有图片或Host命令已交付。[计划](../plans/26-mcp-stdio-loop.md)记录范围。

## 源与协议各负责什么

PHP `src/Command/McpLoop.php`支持2025-06-18、2025-03-26、2024-11-05。请求命中则回显，否则选择06-18；只有06-18输出outputSchema/structuredContent，旧版保留完整text/image。本课保这份兼容矩阵。

[官方lifecycle](https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle)定义初始化及initialized通知；[stdio transport](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)要求按行UTF8 JSON且stdout不混诊断；[tools](https://modelcontextprotocol.io/specification/2025-06-18/server/tools)定义工具及结构化输出。

Go明确要求initialize→notifications/initialized后才处理tools，ping可随时使用，比PHP源码宽松调用顺序更严格。协议会话局部保存版本，不通过全局变量影响别的客户端。

## 从一行输入读到响应

`NewServer(Toolset,Config)`只配置服务名、版本与限额，不启动服务。`Serve(ctx,input,stdout,stderr)`逐行读取→UTF8/JSON及重复键校验→Request/参数校验→生命周期路由→工具调用→按版本投影→限长编码→单次写入。

Toolset只有Definitions(ctx)、Call(ctx,name,args)和Instructions。参数以json.RawMessage对象交具体工具，循环不自己实现JSONSchema解释器。Definitions和Instructions应是有界本地描述；Call与Definitions必须协作ctx，循环不为忽略取消的工具另开遗留goroutine。

ToolResult含typed text/image和可选structured对象。旧版本去掉结构化部分，但文本与图片仍完整；若Toolset只给structured而未给text，循环补该JSON的文本副本，已有image也保留。空text也保留必须的text键。实际Cloud工具还需要独立输入白名单、权限、分页及图片来源验证，本课不靠Toolset接口冒称这些已完成。

## ID是关联值，不是业务实体ID

RPC ID用RawMessage保存字符串、所有合法JSON数值的原词法与源兼容null；不会经float64丢失大整数或指数精度。业务工具中的异常ID仍须由后续工具独立校验。null沿PHP保留，但官方MCP请求ID约束更严格，不建议客户端使用null。

无ID且结构合法的通知不回包，未知通知静默；notifications/cancelled也沿源忽略。当前是串行循环，没有实现读下一条通知来抢占在途Call；外部生命周期ctx能取消协作工具，不能把它说成完整并发JSON-RPC取消调度。

## 限额与错误也属于协议

默认每行1MiB、每响应4MiB，可配置256字节到64MiB；结构深度64。坏JSON/UTF8给-32700，非法请求/重复键/batch给-32600，参数不合法给-32602，未知方法给静态-32601。Toolset失败或panic只回静态internal error并写静态stderr诊断，不回显任意method、args、cause或panic值。

输出先完整编码再写，不把截半JSON当成功。响应超限尝试同ID的静态internal error；若ID本身已令错误响应超限，则ErrOutput终止且不发送该包。底层短写仍返回ErrOutput，无法撤回已写字节，owner应结束这次会话。

Serve拥有io.ReadCloser输入，EOF和退出都会Close。取消watcher调用Close解除Read并在退出时join；这个保证要求Close及时解除读取，不能套在任意阻塞Reader/Writer上。工具和stdout/stderr本身也需有界/协作关闭。包不导入Host logger，stdout只交给协议。

## 真实验证与自主练习

`GOWORK=off GOTOOLCHAIN=local go test -mod=readonly ./mcp -run '^TestLoop'`首轮因输出限额夹具未真超限失败；修正夹具后0.316s。补大ID错误包限额与工具取消后，同范围race2.359s。终读补stderr短写与structured无text回退后，仅受影响两项通过0.683s，原race是补强前证据，未重复全组。涵盖三版本及fallback、完整旧content、精确ID、生命周期/错误/通知、限额/短写、io.Pipe取消和本测试二进制stdio EOF。只假Toolset，无Host bootstrap、真实Cloud、网络服务、E2E或全量测试。

用户自主练习待完成：说明RPC ID与业务ID区别；比较structured投影和丢失text/image；解释ctx取消与notifications/cancelled当前边界；画出一行输入到stdout唯一JSON的主链，并指出哪些阻塞必须由owner约束。
