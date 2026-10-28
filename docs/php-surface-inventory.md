# moo-monitor-laravel PHP 源码入口清单

2026-10-28 从本机源码自动提取，提交 `28d424e`。本清单穷举所列目录的文件及静态命令/路由声明，用于完整复核防漏；未执行这些入口，也不代表已审查每个方法内部的全部行为。

动态注册、依赖包入口与 Host 调用需结合 Provider 和真实消费者继续核对；源码声明数不等于运行时实际注册数。

## CLI 命令

当前静态声明共 4 项；参数、选项、环境 gate、交互和退出码在逐项行为复核时记录。

| 命令 | 源文件 | 源码描述 |
| --- | --- | --- |
| `moo:cloud:mcp` | [src/Command/CloudMcpCommand.php](../../moo-monitor-laravel/src/Command/CloudMcpCommand.php) | 以 MCP server 形式把云端 runtime 错误与待办暴露给本仓库的 AI（拉取 / 查看 / 处理回写） |
| `moo:cloud:push` | [src/Command/CloudPushCommand.php](../../moo-monitor-laravel/src/Command/CloudPushCommand.php) | 把本地 runtime / 慢 SQL 记录推送到 moo-scaffold-cloud（增量、幂等） |
| `moo:cloud:test` | [src/Command/CloudTestCommand.php](../../moo-monitor-laravel/src/Command/CloudTestCommand.php) | 自检：推送一条 runtime + 一条慢 SQL 到云端，验证接入配置是否真的有效 |
| `moo:monitor:migrate` | [src/Command/MigrateCommand.php](../../moo-monitor-laravel/src/Command/MigrateCommand.php) | 从 moo-scaffold ≤3.8 迁移本地监控数据与推送游标到 storage/moo-monitor/（幂等） |

## 文件覆盖索引

源文件、模板、前端与测试分别列出；测试存在不代表本轮已经运行或通过。src 下视图和专用 Request 同样列入，避免只盘点顶层类。

### src

文件数：22。

- [src/Cloud/CloudClient.php](../../moo-monitor-laravel/src/Cloud/CloudClient.php)
- [src/Cloud/CloudSync.php](../../moo-monitor-laravel/src/Cloud/CloudSync.php)
- [src/Cloud/HeartbeatMeta.php](../../moo-monitor-laravel/src/Cloud/HeartbeatMeta.php)
- [src/Cloud/SyncStateException.php](../../moo-monitor-laravel/src/Cloud/SyncStateException.php)
- [src/Command/CloudMcpCommand.php](../../moo-monitor-laravel/src/Command/CloudMcpCommand.php)
- [src/Command/CloudPushCommand.php](../../moo-monitor-laravel/src/Command/CloudPushCommand.php)
- [src/Command/CloudTestCommand.php](../../moo-monitor-laravel/src/Command/CloudTestCommand.php)
- [src/Command/CloudToolset.php](../../moo-monitor-laravel/src/Command/CloudToolset.php)
- [src/Command/McpLoop.php](../../moo-monitor-laravel/src/Command/McpLoop.php)
- [src/Command/MigrateCommand.php](../../moo-monitor-laravel/src/Command/MigrateCommand.php)
- [src/Concerns/SafelyLogs.php](../../moo-monitor-laravel/src/Concerns/SafelyLogs.php)
- [src/ExceptionDispatcher.php](../../moo-monitor-laravel/src/ExceptionDispatcher.php)
- [src/LoggedErrorMessage.php](../../moo-monitor-laravel/src/LoggedErrorMessage.php)
- [src/MooeenMonitorLaravelServiceProvider.php](../../moo-monitor-laravel/src/MooeenMonitorLaravelServiceProvider.php)
- [src/Recorder/BucketedYamlRecorder.php](../../moo-monitor-laravel/src/Recorder/BucketedYamlRecorder.php)
- [src/Recorder/RuntimeErrorRecorder.php](../../moo-monitor-laravel/src/Recorder/RuntimeErrorRecorder.php)
- [src/Recorder/SensitiveMasker.php](../../moo-monitor-laravel/src/Recorder/SensitiveMasker.php)
- [src/Recorder/SqlSlowListener.php](../../moo-monitor-laravel/src/Recorder/SqlSlowListener.php)
- [src/Recorder/SqlSlowRecorder.php](../../moo-monitor-laravel/src/Recorder/SqlSlowRecorder.php)
- [src/ScheduledTaskExit.php](../../moo-monitor-laravel/src/ScheduledTaskExit.php)
- [src/SelfTestException.php](../../moo-monitor-laravel/src/SelfTestException.php)
- [src/StorageScope.php](../../moo-monitor-laravel/src/StorageScope.php)

### config

文件数：1。

- [config/moo-monitor.php](../../moo-monitor-laravel/config/moo-monitor.php)

### tests/Feature

文件数：34。

- [tests/Feature/Capture/CaptureMatrixTest.php](../../moo-monitor-laravel/tests/Feature/Capture/CaptureMatrixTest.php)
- [tests/Feature/Cloud/CloudClientTest.php](../../moo-monitor-laravel/tests/Feature/Cloud/CloudClientTest.php)
- [tests/Feature/Cloud/CloudScheduleTest.php](../../moo-monitor-laravel/tests/Feature/Cloud/CloudScheduleTest.php)
- [tests/Feature/Cloud/CloudSyncTest.php](../../moo-monitor-laravel/tests/Feature/Cloud/CloudSyncTest.php)
- [tests/Feature/Cloud/MonitorSnapshotContractTest.php](../../moo-monitor-laravel/tests/Feature/Cloud/MonitorSnapshotContractTest.php)
- [tests/Feature/Command/CloudMcpCommandTest.php](../../moo-monitor-laravel/tests/Feature/Command/CloudMcpCommandTest.php)
- [tests/Feature/Command/CloudPushCommandTest.php](../../moo-monitor-laravel/tests/Feature/Command/CloudPushCommandTest.php)
- [tests/Feature/Command/CloudTestCommandTest.php](../../moo-monitor-laravel/tests/Feature/Command/CloudTestCommandTest.php)
- [tests/Feature/Command/MigrateCommandTest.php](../../moo-monitor-laravel/tests/Feature/Command/MigrateCommandTest.php)
- [tests/Feature/HostSafety/CaptureHookSafetyTest.php](../../moo-monitor-laravel/tests/Feature/HostSafety/CaptureHookSafetyTest.php)
- [tests/Feature/HostSafety/NeverThrowsIntoHostTest.php](../../moo-monitor-laravel/tests/Feature/HostSafety/NeverThrowsIntoHostTest.php)
- [tests/Feature/Recorder/AuthGuardsConfigTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/AuthGuardsConfigTest.php)
- [tests/Feature/Recorder/ConsoleRequestTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/ConsoleRequestTest.php)
- [tests/Feature/Recorder/ExceptionDispatcherTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/ExceptionDispatcherTest.php)
- [tests/Feature/Recorder/HttpException5xxHookTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/HttpException5xxHookTest.php)
- [tests/Feature/Recorder/LogMessageHookTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/LogMessageHookTest.php)
- [tests/Feature/Recorder/RecorderCountTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/RecorderCountTest.php)
- [tests/Feature/Recorder/RecorderDailyCapTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/RecorderDailyCapTest.php)
- [tests/Feature/Recorder/RecorderHashMaskTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/RecorderHashMaskTest.php)
- [tests/Feature/Recorder/RecorderLeanPathTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/RecorderLeanPathTest.php)
- [tests/Feature/Recorder/RecorderOverflowBackfillTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/RecorderOverflowBackfillTest.php)
- [tests/Feature/Recorder/RecorderPayloadTruncateTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/RecorderPayloadTruncateTest.php)
- [tests/Feature/Recorder/RecorderPreviousChainTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/RecorderPreviousChainTest.php)
- [tests/Feature/Recorder/RecorderReopenBucketTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/RecorderReopenBucketTest.php)
- [tests/Feature/Recorder/RecorderSourcePriorityTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/RecorderSourcePriorityTest.php)
- [tests/Feature/Recorder/RecorderWriteFailureTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/RecorderWriteFailureTest.php)
- [tests/Feature/Recorder/ScheduleExitHookTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/ScheduleExitHookTest.php)
- [tests/Feature/Recorder/SensitiveMaskerTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/SensitiveMaskerTest.php)
- [tests/Feature/Recorder/SqlBindingEscapeTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/SqlBindingEscapeTest.php)
- [tests/Feature/Recorder/SqlConnectionAndAppFramesTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/SqlConnectionAndAppFramesTest.php)
- [tests/Feature/Recorder/SqlSlowListenerTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/SqlSlowListenerTest.php)
- [tests/Feature/Recorder/SqlSlowMaskUrlTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/SqlSlowMaskUrlTest.php)
- [tests/Feature/Recorder/StorePathTraversalTest.php](../../moo-monitor-laravel/tests/Feature/Recorder/StorePathTraversalTest.php)
- [tests/Feature/StorageScopeTest.php](../../moo-monitor-laravel/tests/Feature/StorageScopeTest.php)

### tests/Fixtures

文件数：3。

- [tests/Fixtures/cloud-intake-partial-response.json](../../moo-monitor-laravel/tests/Fixtures/cloud-intake-partial-response.json)
- [tests/Fixtures/cloud-monitor-snapshot-contract.json](../../moo-monitor-laravel/tests/Fixtures/cloud-monitor-snapshot-contract.json)
- [tests/Fixtures/mcp-todo-detail-response.json](../../moo-monitor-laravel/tests/Fixtures/mcp-todo-detail-response.json)

### scripts

文件数：1。

- [scripts/smoke-lower-laravel.sh](../../moo-monitor-laravel/scripts/smoke-lower-laravel.sh)

## Manifest 与质量入口

- [composer.json](../../moo-monitor-laravel/composer.json)
