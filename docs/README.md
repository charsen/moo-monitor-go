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

[完整复现计划](port-plan.md) · [PHP源码入口](php-surface-inventory.md)
