# Monitor有序课程

按编号阅读，参考实现通过不代表用户自主练习完成。

| 序号 | 课程 | 当前范围 |
| --- | --- | --- |
| 01 | [基础脱敏纯函数](lessons/01-sensitive-masker.md) | M07-A键/URL/自由文本对源fixture，SQL/递归另见02/03，Host未接入 |
| 02 | [SQL值侧脱敏](lessons/02-sql-masker.md) | M07-B比较/INSERT有限算法对源fixture，递归另见03，事件未实现 |
| 03 | [有界递归payload](lessons/03-bounded-payload.md) | M07-C深度/共享预算/UTF8截断与显式类型边界，未知请求表示/Host适配待接，显式事件构建见04 |
| 04 | [运行时记录构建与刷新](lessons/04-runtime-records.md) | M02显式Snapshot、脱敏/hash/累计/source，采集/存储/Cloud待接 |
| 05 | [运行时缓冲与恢复](lessons/05-runtime-buffer.md) | M08私有scope三桶/cap/持久overflow/固定intent，采集与Cloud待接 |
| 06 | [增量同步与确认账本](lessons/06-incremental-sync.md) | 指纹快照/严格receipt/部分确认/持久水位，只有fake传输与受控HTTP替身 |
| 07 | [Cloud HTTP传输](lessons/07-cloud-http-client.md) | runtime intake/discard传输、wire校验与本机真实HTTP联调，未接真实Cloud |
| 08 | [显式runtime采集与日志旁路](lessons/08-runtime-capture.md) | EventScope/原子tag/真实Writer消费，未自动Cloud或替换Recover |
| 09 | [HTTP生命周期与Recover委托](lessons/09-http-capture.md) | 请求scope/有限快照/5xx与显式owner回调，未改Host全局恢复 |
| 10 | [Host本地运行时](lessons/10-host-runtime.md) | late identity与Host默认关闭资源接线，真实教学见Host41 |
| 11 | [显式慢SQL完整链](lessons/11-slow-query-pipeline.md) | 独立typed SQL/共享durable内核与SQL Cloud协议；无自动DB hook |
| 12 | [真实GORM慢SQL回调](lessons/12-gorm-slow-query.md) | 已存在SQLite实例显式安装、六core窗口与旁路隔离；Host预热接线后续 |
| 13 | [Host慢SQL消费边界](lessons/13-host-slow-query.md) | 当前请求标量/标准类型、真实Food目标写链与Host启动边界 |
| 14 | [队列最终失败与调度完成](lessons/14-queue-schedule.md) | 真实Worker/成熟DB Driver/cron，显式owner；当时Host采用待接，现见15/16 |
| 15 | [Host队列Owner生命周期](lessons/15-host-queue-owner.md) | 单Run/首N屏障/可重试关闭/DisableCapture，Host45实证 |
| 16 | [Host受管理Schedule](lessons/16-host-schedule.md) | 原生单实例/专用Artisan与Event观察、可取消排空，Host46采用 |
| 17 | [显式与周期增量推送](lessons/17-cloud-push-owner.md) | 原ACK/type锁、显式命令与首tick Owner；完整M13未完 |
| 18 | [健康心跳与saved级自检](lessons/18-health-check.md) | typed心跳/严格逐条saved/真实命令，Cloud版本/resolve等仍待 |
| 19 | [精确待推只读统计](lessons/19-pending-statistics.md) | client-free读取、完整ACK版本匹配与Unknown |
| 20 | [Cloud确认后的local噪音抑制](lessons/20-local-noise-discard.md) | 独立suppression/严格版本/保累计锚点，不造ACK或删除record |
| 21 | [只读推送预览](lessons/21-push-preview.md) | 无凭据/零网络、严格版本增量候选与Unknown |
| 22 | [显式All重推](lessons/22-explicit-all-replay.md) | 忽略历史ACK选集、保exact抑制、本次Remaining与持久化错误分开 |
| 23 | [确认resolved安全维护](lessons/23-confirmed-resolved-maintenance.md) | store-only/exactACK/拒恢复，无按天龄清open |
| 24 | [只读迁移体检](lessons/24-migration-inspection.md) | 显式PHP源/游标/旧env检查，非导入或ACK证明 |
| 25 | [受控旧记录导入](lessons/25-legacy-record-import.md) | 完整typed转换/摘要漂移/新scope归属与可续作，源只读 |
| 26 | [MCP stdio协议循环](lessons/26-mcp-stdio-loop.md) | 三固定版本/精确ID/限长与取消，七Cloud工具及Host桥未完成 |
| 27 | [Cloud七工具](lessons/27-cloud-mcp-tools.md) | typed协议/分页/私有图片，假服务闭环，Host入口后续 |
| 28 | [独立Host MCP进程](lessons/28-host-mcp-entry.md) | 不启业务bootstrap、不读dotenv，协议stdout与输入生命周期隔离 |
| 29 | [事件过滤与HTTP现场](lessons/29-filtered-http-payload.md) | 显式能力/默认关payload/预算与同cap，原业务响应不改 |
| 30 | [有界源码上下文](lessons/30-source-context.md) | 显式根/前缀与预算、辅助失败保原事件，不改异常身份 |
| 31 | [Postgres慢SQL展示](lessons/31-postgres-slow-query.md) | 成熟dialector/sqlmock回调，原raw/hash不变；未实际PG |
| 32 | [DB facade原生观察](lessons/32-db-facade-observation.md) | 同pool/原Query与Tx，私有真实PG两路径补证；非生产部署 |
| 33 | [私有崩溃诊断](lessons/33-crash-output.md) | M05第一阶段raw spool/有界Inspect，不自动导入或上传 |
| 34 | [单ID诊断导入](lessons/34-crash-diagnostic-import.md) | 永久receipt/固定v2恢复/真实容量，raw保留 |
| 35 | [typed Cloud摘要](lessons/35-cloud-summary.md) | 显式summary、有限投影与安全CLI，未做首页UI |
| 36 | [日志采集策略](lessons/36-log-capture-policy.md) | typed来源/消息独立开关与六级别，原日志透传 |
| 37 | [真实构建元信息](lessons/37-go-build-metadata.md) | bounded BuildInfo/可选平台wire/四实际producer |
| 38 | [独立解析与构建](lessons/38-release-reproducibility.md) | 固定HEAD/file-only proxy/Go1.27与1.26，非远端发布 |
| 39 | [自检runtime可选解决](lessons/39-self-test-resolution.md) | strict saved后best-effort/结果分离/同Cloud token |
| 40 | [MySQL双入口慢SQL](lessons/40-mysql-slow-query.md) | 安全Explain/原Pool Query Tx/Host可选Provider |
| 41 | [SQLServer双入口](lessons/41-sqlserver-slow-query.md) | 实际替换callback/编号预算/OUTPUT有限mask |

[完整复现计划](port-plan.md) · [PHP源码入口](php-surface-inventory.md)
