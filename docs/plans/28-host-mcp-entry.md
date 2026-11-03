# 28 Host独立MCP入口

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

对应Host56实际go run ./cmd/monitor-mcp，非普通Artisan注册。只独立stdlib/configlookup owner消费SDK26+27，不import Host业务bootstrap/config/facades或业务owner。专用MONITOR_MCP_*开关/URL/token/TLS/timeout，默认false，不读.env，不回退intake token。

HTTP明确4MiB、loop输出8MiB；EOF/信号关闭input和ownedHTTP，stderr静态、stdout只协议。Darwin/Linux pollable stdin dup保存/恢复共享flags，当前Darwin实测，Linux未真机、其它平台不支持；强杀不声称可恢复。Run任意参数失败也关非nil输入，首Close失败静态ErrInputCleanup可见。

Host初六模式子例通过，整包因真实stdin取消失败（10秒timeout）未整体通过；配置0.727s。修复后仅必要SIGTERM/原flags定向0.402s，Run所有权0.407s。实际2MiB图片完整JSON-RPC已在六模式覆盖，未重复七工具。go-list闭包无业务bootstrap/foundation，生产binarybuild+禁用exit2/空stdout已回读并清ownedbinary。仅假HTTP与测试二进制、无真实Cloud/env/token/全量/E2E。

本步是可部署入口和本机协议验证，不代表已安装到用户AI客户端或目标Cloud权限/版本验收；用户练习待完成。
