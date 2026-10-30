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

[完整复现计划](port-plan.md) · [PHP源码入口](php-surface-inventory.md)
