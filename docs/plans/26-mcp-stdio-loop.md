# 26 MCP stdio协议循环

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

独立mcp包消费Toolset，不导入Host/Cloud/Buffer。本步只有协议循环，不表示七个工具、图片分页或Host命令已完成。

依据PHP src/Command/McpLoop.php与官方固定2025-06-18 lifecycle/transports/tools。固定支持2025-06-18、2025-03-26、2024-11-05，命中回显否则回06-18；只有06-18输出outputSchema/structuredContent，旧版保text/image。

NewServer接Toolset与ServerName/Version/输入输出上限；Serve接拥有的io.ReadCloser及stdout/stderr，输入必须Close可及时解除Read。EOF/取消都关闭，不用遗留读取goroutine伪称任意Reader可取消；工具串行，需遵守ctx。stdio只单行JSON，诊断静态stderr。初始化→initialized→工具请求，ping可用；与PHP宽松顺序区别明确。ID保RawMessage精确字符串/JSON数值原词法/null（null沿源兼容，非推荐MCP ID），不经float64丢精度。重复键/batch/UTF8/结构/参数/行限错误安全可见，不回显原文。默认1MiB输入/4MiB输出，深度64，输出超限安全internal error，不输出截半JSON。

定向测试假Toolset：三版本/旧text-image、生命周期与静默通知、精确大整数与各种非法输入、静态错误、限长/短写、真实pipe取消/EOF与子进程stdio。不启动Cloud/网络、不全量，实际结果待执行。

## 实际证据

定向首轮因256字节夹具未实际超限失败，调整为真实超限响应后PASS0.316s；补大ID fallback仍限长和工具取消后，TestLoop范围race最终2.359s。只独立SDK/假Toolset/本测试二进制，不读Host真实.env、不继承Hostbootstrap、不联网。

输入拥有契约要求Close及时解除Read；任意工具/Writer仍须协作。notifications/cancelled沿源静默，当前不是并发RPC取消调度；null ID沿源兼容，非推荐MCP客户端值。所有合法JSON数值ID保原词法。初始化顺序比PHP更严格。响应fallback本身若超限返回ErrOutput，不发送越限包。用户练习待用户完成。

官方依据：[生命周期](https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle)、[stdio](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)、[tools](https://modelcontextprotocol.io/specification/2025-06-18/server/tools)。后续实现七工具/Host桥后才形成实际Cloud MCP业务闭环。

终读补强：stderr短写nil返回ErrOutput；structured无text时补JSON text兼容回退且保image，三版本断言；仅TestLoopStructuredFallback/DiagnosticShortWrite最终0.683s。原race2.359s在此前执行，不声称补后全组重跑。

## 当前契约衔接（2026-11-03校准）

协议循环与27七工具、28独立Host入口均已实现并有本地消费者；真实MCP服务/权限/私有图仍待验，不能用fake协议loop成功外推外部互通。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
