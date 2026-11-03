# 27 Cloud七工具

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

新增cloudsync/mcp*.go七个typed HTTP入口与mcpcloud Toolset，复用26stdio循环。范围为list_open_runtimes/get_runtime/resolve_runtime/list_open_todos/get_todo/get_todo_image/update_todo_status，Host桥留28。只本机httptest假token/假状态，不用真实.env/token/Cloud。

源PHP CloudToolset/CloudClient及Cloud RuntimeReadController/TodoReadController/routes已核：七路均POST body token，独立mcp ability；交互timeout clamp1..4秒，连接级2次/100ms，HTTP错误不即时重试，拒redirect。输入严格白名单、精准字符串ID/整数分页，非法零HTTP；不解析日志。返回完整text/structured/image。分页offset/has_more/next_offset闭合，旧Cloud只offset0允许unknown分页。

get_todo只传id，with_network_payloads仅本地裁剪context_requests/errors并附相同JSON。保Cloud已核markdown/discussion，它不直接渲染专门headers/body字段；不宣称能清除自由描述/评论任意秘密。图片只固定id+attachment端点，严格base64/2MiB及四MIME，不取外部URL/不落盘。副作用工具描述须先授权再执行，仅假服务验证。

先编译typedHTTP与7定义，再定向真实HTTP/26stdio消费者，实际结果见下方。SDK26已交付loop不等于本步工具；后续Host owner/命令仍需独立接线。用户练习待完成。

## 实测与边界

HTTP与7定义首编译成功只记compile-only（cloudsync0.402s、mcpcloud无testfiles），不当验证通过。随后五组真实假服务定向0.421s；新范围race mcpcloud1.744s/cloudsync新两组1.479s。仅本次工具与26真实stdio消费，不运行旧包全量/Host，也不读真实.env/token。

图片不得提高显式response cap，调用者自行配置足够预算（测试4MiB，低cap负例确认失败）。列表Go越界拒绝不同于PHP工具clamp；自由markdown/discussion保原，不声称任意秘密全清洗。状态回执保幂等标记与原记录，只本机假修改。用户练习待其完成。

终读补强：请求in_progress/返回done仅already_done=true允许；互斥flag与状态须闭合，状态输出投影不传原任意error。源布尔字符串FILTER_VALIDATE_BOOL/FILTER_NULL_ON_FAILURE??false保留并说明schema推荐boolean。仅对应定向TestStatusReceiptAndBooleanCompatibility PASS0.084s，原race在补强前，不重复全组。

## 当前契约衔接（2026-11-03校准）

七工具由成熟Cloud客户端权限分别约束；runtime/slow intake成功不代表mcp ability。真实七工具完整权限与图片访问没有被心跳/两saved验收代替。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
