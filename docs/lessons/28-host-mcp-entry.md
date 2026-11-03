# 第28课：让Host的MCP进程不启动业务系统

[上一课：27](27-cloud-mcp-tools.md) · [下一课](29-filtered-http-payload.md) · [课程目录](../README.md)

SDK26和27已有协议与工具，Host56新增独立engine/cmd/monitor-mcp实际入口。普通Artisan启动会先触发配置及Provider/Runner初始化，所以本步不假称注册了moo:cloud:mcp。[计划](../plans/28-host-mcp-entry.md)给出证据和限制。

实际运行从Host engine目录执行go run -mod=readonly ./cmd/monitor-mcp；部署构建专用binary，不导入Host bootstrap/config/facades或业务owner。纯lookup配置只读MONITOR_MCP_ENABLED/URL/TOKEN/TIMEOUT_SECONDS/VERIFY_TLS，默认disabled，不自动读.env、不借intake token。Cloud需要独立mcp ability，配置可用不等于已获真实工具操作授权。

Run直接配置→HTTPClient→Toolset→Server→Serve。显式HTTP响应4MiB、loop输出8MiB，图片data仅image block，不重复进structured。业务后台push/heartbeat/队列/DB均不启动；stdout只有JSON-RPC，错误只静态stderr，禁用/配置失败非0。主业务main保持原样。

真实stdin取消首轮失败：os.Stdin.Close没有解除阻塞Read。Darwin/Linux入口改pollabledup，并保存/恢复共享原flags；scope只在专用进程，不能假定父shell不受共享flags影响。Run捕获首次Close错误且非法参数也关闭输入，静态ErrInputCleanup可见。当前Darwin实测，Linux机制未真机、其它平台明确不支持；SIGKILL/断电不会执行清理，不能声称恢复保证。

Host六模式子例通过但初轮整包因SIGTERM超时失败；配置包0.727s。修复后仅SIGTERM/父fd flags定向0.402s、Run资源错误/非法参数0.407s。2MiB图片完整JSON-RPC已验证；依赖闭包无Hostbootstrap/foundation，真实生产binarybuild及disabled exit2/空stdout通过，ownedbinary已清。未重跑七工具/全量，未读真实.env/token或访问真实Cloud。

用户自主练习待完成：比较独立stdio进程与业务Artisan初始化；解释为什么MCP token不能默认为intake token；区分Close/取消与强杀的资源保证；先用假服务检查AI客户端的stdout解析。目标部署和真实Cloud权限/版本仍需授权后的独立验证。

## 2026-11-03 真实 MCP 只读补证

使用独立 `cmd/monitor-mcp` 进程、专用 `MONITOR_MCP_*` 和已部署 Cloud 完成 `initialize`（2025-06-18）、`tools/list`（7 个工具）与 `get_runtime`（hash `ae899b31f313`）；结果 `isError=false`，context 中 `php/laravel` 为空、`go=go1.27.3`、`goravel=v1.18.0`。未调用 resolve/update_todo_status/get_todo_image 等写操作，未验证真实权限与图片；token/URL 未进入文档或提交。

## 2026-11-03 MCP resolve 写路径补证

在受控测试 runtime `deadbeef0001`（source=self_test）上通过真实 MCP stdio 调用 `resolve_runtime`，返回 `isError=false`，随后 `get_runtime` 状态为 `resolved`。这覆盖真实鉴权/传输/状态回执；未调用 `update_todo_status`、未验证私有图片或全部权限。测试记录保留为 resolved，不作为业务数据。
