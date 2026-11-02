# 第29课：先过滤事件，再保存有界现场

[上一课：28](28-host-mcp-entry.md) · [课程目录](../README.md) · [下一课：30](30-source-context.md)

本课补明确的错误过滤和默认关闭的HTTP payload快照，Host57提供显式开关。[计划](../plans/29-filtered-http-payload.md)记录范围；不改已有hash、codec或迁移格式。

## 过滤不是猜错误文字

源PHP ExceptionDispatcher过滤CLI实验/Console输入错误，RuntimeErrorRecorder.shouldReport过滤HttpException的<500状态。Go不伪造Symfony类或tinker：只接受HTTPStatusCode() int、ConsoleInputError() bool能力，以及调用方显式Options.CLIExperiment。

过滤发生在scope登记/TagSource之前；跳过返回空hash+nil，不占ID、不增加count。HTTP有效状态为100..599，<500跳过，5xx保留；非法值与未知类型保留。Console/实验仅Method空或CLI时生效，nil Config.Filter默认两项skip，显式FilterConfig可调整。Log字符串里写404不会变成HTTP错误。

只跟单Unwrap链最多8层，支持常规fmt包装。errors.Join混有真实业务失败不能因为另一个4xx就整条丢掉，因此多错误不展开。外层有效5xx优先保留；能力方法panic沿现ErrExtraction隔离，不留下scope记录。没有自动识别所有CLI用法错误，也没有新增全局dontReport注册器。

## 快照只在请求入口取一次

HTTPConfig.Payload=nil默认关闭。开启时NewHTTP通过现mask.New固定MaskKeys，默认200字符、负cap拒绝；仅POST/PUT/PATCH/DELETE入口取Request.All，经过既有MaskPayload，存到httpState。日志首次采集、5xx和显式Recover均消费同一脱敏副本，关闭时清空，不保原Request或Entry。

真实Gin v1.18 Request.All只合并已解析body/query/route，body优先；它自身不会再读Body。但lazy Request初始化的getHttpBody已经按框架机制读取并解析body。SDK不额外读取流，不能声称框架没有解析。JSON数字已成float64，超过精度的原词法不可恢复；All包含route也不同于PHP输入集合。

顶层重复字段[]string映射为[]any。已知FileHeader/[]FileHeader只映射静态OpaqueObject类型名，不读取Filename/Header/Size/temp路径，更不Open。业务handler仍能读取自己的上传文件，监控仅保存类型标记。未知对象不反射、不Stringer，安全诊断并省略本次payload。

## 两层预算各有职责

归一前先检查根keys与已知slice元素总数，最多5000；超限不先复制巨大重复值数组，而是安全省略payload并诊断。之后JSON嵌套交原MaskPayload的5000共享预算、深度和截断规则。预算并非框架All分配、全部键预检/排序CPU的硬上限，不能当请求大小防火墙。

归一失败不会消费/替换业务Body，也不会改变原响应。主日志delegate仍按原逻辑写；监控失败只静态诊断。已脱敏快照后来仍由Builder按其已有规则保护，不新造另一份mask算法。

## 实测与练习

过滤普通定向0.861s；adapter首轮unused import编译失败，修后新payload/边界0.858s。必要新旧消费者race为capture4.844s、adapter5.278s，含真实Writer、JSON/form/multipart/GET、首日志+5xx同ID及Recover共用现场。终读补单unwrap/合法状态和归一预预算后，仅受影响过滤0.630s、adapter边界0.338s；race是补强前证据，没有重复全组。

Host57七模式首轮仅bad-json断言错误而失败1.634s；源decoder仅log并继续handler，测试误设强制500。按真实原503/body修正后唯一bad-json子例0.900s；其它六子例不重跑。父子均指定自有空-env，清真实MONITOR继承，DB/Cloud/MCP/queue/schedule关闭，native日志只在自有目录。无真实.env/token/Cloud、E2E或全量。

用户自主练习待完成：解释join为什么不能按任一4xx过滤；说明filtered为何不占scope ID；区分框架body解析与监控读文件；预测默认关闭/GET/上传的payload形状；指出float64与预算不能保证的边界。

Host终读统一有效cap：开启payload时显式0归一默认200，并把同一值同时传HTTP与records.Config.StringMaxChars，避免Builder二次保护又截回200。仅large-cap(500完整保320字符)/zero-cap两实际落盘子例最终0.933s；此前七模式与SDK不重复。SDK直接组装者也须协调HTTP与Builder的各自限制，不把更大的入口cap当绕过最终Builder保护。
