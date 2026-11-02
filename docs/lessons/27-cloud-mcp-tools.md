# 第27课：把七个Cloud工具接到同一协议循环

[上一课：26](26-mcp-stdio-loop.md) · [课程目录](../README.md)

第26课只有协议，本课才实现七项实际工具。范围见[计划](../plans/27-cloud-mcp-tools.md)，Host显式stdio命令留下一独立功能，不把SDK通过当Host已接。

## 三个职责，沿一次调用读下去

`mcp.Server`校验JSON-RPC、生命周期与版本；`mcpcloud.Toolset.Call`白名单解析参数并在七个switch分支直接编排；`cloudsync.HTTPClient`的七个typed方法投影固定body、发送请求、验证回执。没有通用pipeline、任意URL工具或日志字符串解析。

工具分别是list_open_runtimes、get_runtime、resolve_runtime、list_open_todos、get_todo、get_todo_image、update_todo_status。Definitions给出输入/输出schema和Annotations，状态工具明确要求先获授权、修复验证后才解决/完成；可调用不代表获准操作当前真实业务。

源码依据是PHP `src/Command/CloudToolset.php`、`src/Cloud/CloudClient.php`及Cloud `routes/api.php`、RuntimeReadController、TodoReadController、McpPagination与Todo::toAiMarkdown。七条路由均为固定POST，token只在body，要求独立mcp ability；上报权限不隐式授予读取/状态修改。SDK不改Cloud权限，也不把本机替身当权限验证。

## 输入到网络的门槛

列表limit为1..50，offset非负整数；runtime状态open/in_progress/resolved，Todo列表open/in_progress/done，回写只in_progress/done。Go对越界列表值拒绝，区别于PHP工具将范围钳位；不静默请求另一个范围。hash为12位小写hex，Todo ID是20..32字母数字字符串，attachment_id为1..32数字字符串，不ParseFloat，保精确大ID。

工具拒绝未知键、重复键、错误类型及超长输入。with_payload/with_network_payloads按源 `filter_var(..., FILTER_VALIDATE_BOOL, FILTER_NULL_ON_FAILURE) ?? false` 保布尔及安全字符串解析，未知字符串按false；schema仍推荐真正boolean。非法输入返回静态isError且零HTTP，不把它伪装成协议崩溃。

七HTTP方法复用原post的TLS、拒重定向、响应限长和安全错误。浅复制client把交互timeout钳到1..4秒，连接级最多2次/100ms，HTTP失败不即时重发；caller取消优先。不会改变共享client timeout或关闭借用transport。图片也尊重调用者MaxResponseBytes；要读2MiB图片，调用者需显式给足base64响应预算，例如4MiB，低cap会明确失败。

## 回执与分页不能猜

列表验证rows数组/对象、可选count、offset/has_more/next_offset完整且相互一致。has_more=true要求next_offset大于当前offset；false要求null。旧Cloud完全不返回分页元数据时，仅第一页允许pagination_supported=false、has_more/next_offset=null；offset>0失败，防止反复拿第一页却称翻页成功。

详情和状态回执核对象身份。状态保持Cloud already_resolved/already_done/already_in_progress及原记录；请求in_progress却回done仅already_done=true时允许，互斥标记/状态不闭合会失败，不声称本次note一定覆盖旧审计。工具结果同时保完整text和structured，26负责按旧版本去掉structured而保text/image。

## 哪些现场默认不返回

get_runtime默认不含payload；客户端即使遇到意外payload字段也在关闭时去掉。

get_todo只向Cloud传id，with_network_payloads仅控制本地投影。默认context_requests只保方法、URL、状态、耗时等已核白名单，专门headers/body字段不进入structured或追加的JSON文本；context_errors始终只保已核错误字段。显式true才保网络payload字段。

Cloud原markdown和discussion仍保留。已核Todo::toAiMarkdown只显示失败method/url/status和JSmessage，不直接渲染专门请求headers/body；但自由描述或评论可能人为粘贴内容。本步不是对任意自由文本秘密的全内容清洗，不解析markdown猜语义，不为去掉现场字段而丢讨论上下文。

## 图片只走私有接口

get_todo_image只提交Todo ID和attachment_id，不fetch返回URL、不生成公开链接、不写临时图片。响应要求strict/canonical base64、解码不超过2MiB及png/jpeg/webp/gif MIME。metadata去掉data，保text+MCP image；当前沿源检查MIME/base64/大小，不声称执行图像内容鉴定。Cloud负责项目/附件归属，真实权限仍待目标Cloud验证。

## 实际验证与练习

五组首轮真HTTP假服务定向PASS0.421s。新增固定路径307/308第二host零请求、交互重试/clamp与取消后，本范围race：mcpcloud五组1.744s、cloudsync新两组1.479s。涵盖全部七端点、输入零HTTP、分页/legacy第一页、默认/true裁剪及讨论、私有图片/低cap，以及真实26stdio三版本消费完整text/image/structured。

终读补状态跨转移回执与布尔字符串兼容，仅对应TestStatusReceiptAndBooleanCompatibility通过0.084s；原race为补强前证据，没有重复全组。状态输出只保实体及已核flag，不透传ok=true回执夹带的任意error。

所有token和状态写都是本机httptest假值，没有读取Host真实.env或当前真实token，没有真实Cloud写、全量/E2E。SDK调用接口存在不等于Host已装配，正式MCP采用仍待后续。

用户自主练习待完成：区分mcp与intake权限；解释legacy第一页为何只能unknown；预测默认/true现场投影；说明already_done为何不等于本次note已写；列出私有图片不允许做的外部URL动作。
