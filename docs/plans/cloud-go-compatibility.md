# Cloud Go元信息兼容方案（本地范围已批准）

本方案保存时仅待授权；2026-11-02用户已批准第四仓moo-scaffold-cloud三个消费者及必要本地测试，由root独占实现、唯一Git writer交付。不含部署、Token能力或真实MCP操作。已有真实Cloud证据仍限授权heartbeat与runtime/slow两项saved，见Host[接入记录](../../../go-moo-engine-skeleton/docs/integration/cloud-connection.md)。

## 三个真实消费者

| 消费者 | 当前事实 | 最小变更 |
| --- | --- | --- |
| [HeartbeatController](../../../moo-scaffold-cloud/app/Http/Controllers/Api/HeartbeatController.php) sanitizeMeta | 白名单只有sdk/sdk_version、PHP/Laravel版本、应用与六布尔；字符串最多120字符 | 仅追加go_version/goravel_version受限字符串（最多120字符，缺失/不可用保未知），不伪造PHP/Laravel；保旧字段兼容 |
| [IntegrationHealthAdvisor](../../../moo-scaffold-cloud/app/Services/Monitoring/IntegrationHealthAdvisor.php) | 提示固定php artisan/MOO_MONITOR_*、版本最低0.1.8；dev/x跳比较 | 按sdk决定平台、命令与配置建议；Go不套PHP的最低版本。development/空/非法版本为未知开发态，不伪装semver或自动判过旧；Go已发布版本策略单独定义后才比较 |
| [integration视图](../../../moo-scaffold-cloud/resources/views/filament/app/pages/integration.blade.php) | 宿主固定PHP/Laravel标签 | Go显示Go/Goravel，已知PHP保原标签，unknown中性；SDK名/版本、应用与开关仍安全转义 |

Heartbeat沿[现route](../../../moo-scaffold-cloud/routes/api.php)的project.token:runtimes，已有JSON元信息列足够，不新增migration、不更新Token权限/updated_at语义或MCP授权。仅字段兼容，不扩大管理动作。

## 平台与向后兼容

sdk=moo-go-monitor为Go；已知PHP SDK及有PHP版本证据的旧客户端保持PHP建议；无法辨认时用平台未知/中性排查，不把缺sdk一律当Go。旧PHP版本门槛保原含义，Go开发版本不套0.1.8、不生成假发布版本。

Go建议使用Host实际moo:cloud:test/moo:cloud:push入口，配置指向MONITOR_ENABLED、MONITOR_SLOW_ENABLED、MONITOR_CLOUD_*。Go心跳schedule表示独立Cloud auto owner，不混为业务SCHEDULE_OWNER_ENABLED；PHP原MOO_MONITOR_*与scheduler建议保留。

当前SDK HeartbeatMeta仅发送已接受字段，sdk固定moo-go-monitor，Host SDKVersion=development；Go/Goravel版本目前只本地诊断。Cloud消费者实现并定向通过后，另批窄扩SDK/Host真实版本发送，不在本方案提前改wire或宣称服务端已兼容。

## 获准后的最小验证与交付

仅定向验证三消费者：新增字段限长/非字符串与缺失、PHP/Go/unknown平台、Go development/非法版本/独立发布策略、原PHP0.1.8门槛、视图转义与标签。保原JSON存储/权限与PHP回归；不运行全量/E2E，不用真实Token或真实MCP操作。第四仓代码及SDK发送扩展分别审diff/commit，是否推送、部署与真实联调仍按用户另行授权。

本地实现已由root完成并经设计验收，第四仓提交edcaca4：三消费者与必要测试已交付。未部署、未改token权限；Go发送端真实构建字段由37继续接入。
