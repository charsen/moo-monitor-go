# Cloud Go元信息：心跳与记录级版本已实现

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

## 当前心跳链路

SDK `cloudsync/heartbeat.go` 发送固定sdk=moo-monitor-go及可选go_version/goravel_version；Host `app/monitor/health.go` 使用真实BuildInfo，并以development/unknown表达未发布或缺证据。SDK字段限长与Cloud白名单分别守边界，不伪填PHP/Laravel版本。

| 消费者 | 当前实现 | 保留边界 |
| --- | --- | --- |
| [HeartbeatController](../../../moo-scaffold-cloud/app/Http/Controllers/Api/HeartbeatController.php) | 按Go SDK接收go_version/goravel_version受限字符串，保旧PHP字段 | 身份与project.token:runtimes沿原路由；字段兼容不授予MCP能力 |
| [IntegrationHealthAdvisor](../../../moo-scaffold-cloud/app/Services/Monitoring/IntegrationHealthAdvisor.php) | Go/PHP/unknown分别给建议；Go development不套PHP最低版本 | Go正式semver最低线未由虚构tag替代；unknown不自动当Go |
| [integration视图](../../../moo-scaffold-cloud/resources/views/filament/app/pages/integration.blade.php) | Go展示Go/Goravel，PHP保原标签，unknown中性 | 只证明项目心跳元信息显示，不证明每条runtime保存了同样版本 |

Host命令建议使用实际moo:cloud:test/moo:cloud:push与MONITOR_*配置；Cloud auto owner不是业务SCHEDULE_OWNER_ENABLED。AI与MCP权限不因心跳打开。

2026-11-02第四仓edcaca4交付当时只是本地变更；用户后来部署，根端21:24实际见到Go/Goravel页面、heartbeat和runtime/slow逐项saved。原14:59旧标签观察保留在[Host接入记录](../../../go-moo-engine-skeleton/docs/integration/cloud-connection.md)。不能继续把本地交付时“未部署”当今天现状；也不能将这些获准写入外推为其余Cloud动作已验。

## 记录级Go版本：已贯通（代码，待部署迁移）

四层链路已在代码层接通：

1. SDK `records/records.go` 的GoContext与HTTP wire发送context.go/context.goravel；导入PHP旧记录有独立php/laravel指针，不伪造Go来源。
2. [RuntimeIntakeController](../../../moo-scaffold-cloud/app/Http/Controllers/Api/RuntimeIntakeController.php)把context.go/context.goravel写入独立`go_version`/`goravel_version`列，不回填php/laravel。
3. [Runtime模型](../../../moo-scaffold-cloud/app/Models/Runtime.php)增加fillable与展示；迁移[2026_11_03_000001](../../../moo-scaffold-cloud/database/migrations/2026_11_03_000001_add_go_versions_to_runtimes.php)为既有表加两列。
4. [RuntimeReadController](../../../moo-scaffold-cloud/app/Http/Controllers/Api/RuntimeReadController.php)以context.go/context.goravel读回；AI Markdown按Go/Goravel展示并使用```go；详情页/表单展示同名字段。

Cloud侧[GoRuntimeVersionTest](../../../moo-scaffold-cloud/tests/Feature/GoRuntimeVersionTest.php)覆盖Go写入、PHP不污染、读回与Markdown代码围栏。Cloud本地测试为SQLite；生产/目标环境仍须执行该增量迁移并重启常驻worker，历史记录两列为空，不伪造历史版本。

## 已有验证和未验项

三消费者的定向PHP验证、SDK/Host真实版本wire验证及真实心跳/两saved分栏保留在上述接入记录与第37节点；本轮仅静态校准，不重新调用真实Cloud。真实resolve/discard/summary、MCP七工具全部权限、私有图片和生产部署不由心跳证明；本地fake测试也不替代这些外部结果。
