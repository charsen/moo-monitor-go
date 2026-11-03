# 39 自检saved后的可选runtime解决

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

依据PHP CloudTestCommand133–146：严格runtime saved后，--resolve仅尝试解决刚生成的固定自检hash；失败只警告，不把已saved改为失败，both继续slow。Go保原Run，新增RunWithOptions与SelfTestOptions.ResolveRuntime，配置可选窄RuntimeResolver复用现HTTPClient.ResolveRuntime严格回执，Resolution单独Applicable/Attempted/Resolved/Hash/安全Outcome。

filtered/retry/legacy/坏回执不resolve；slow-only标not_applicable，false零新请求/依赖。固定note/by，不接受任意业务hash/文本。父ctx原样传递，无background补偿；取消若影响后续intake仍按原ErrSelfTest，resolve自身失败不篡改saved/整体error。结果不含Cloud自由error/body/URL/token。

Host现健康命令--resolve=false普通自检，true使用同配置Cloud client能力，仅mcp权限提示不探权限/不读取独立MCP token；分别打印saved结果和resolution警告。只fakeCloud+真实CLI必要定向，不读实际.env/token/不真实操作/不做startup扫描。先SDK首闭环再Host，Host68已预留；实际结果见下方，练习待用户。

## 实测

SDK新2Test/HTTP12子类普通3.002s（不重复），old Run严格saved pipeline单项0.419s；Host68九模式真实CLI唯一race9.974s。fakeCloud/白名单emptyenv，RuntimeResolver unavailable仅warning且Attempted=false，Resolved需严格返回；新结果DTO不含自由原因。零真实resolve/无新增依赖，不以本课覆盖MySQL/SQLServer待实现驱动或P1-04页面。用户练习待完成。

## 当前契约衔接（2026-11-03校准）

MySQL/SQLServer观察器后来已实现；真实服务/TLS/所有池另待验。本课只证明固定自检hash可选resolve协议，不授权任意业务runtime解决。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
