# 原设计文档全面校准（2026-11-03）

这份索引记录逐篇原文扫描与处置，不替代原设计正文。先区分可执行契约、当前实现、历史步骤、后置能力和用户学习；不把所有“待”机械改成“完成”，也不把历史测试数字改成后来整轮结果。

## 权威依据与修订范围

- 当前原设计入口：[port-plan.md](port-plan.md)；配套契约：[plans/cloud-go-compatibility.md](plans/cloud-go-compatibility.md)。
- 核验的源码边界：records/records.go；cloudsync/heartbeat.go、pusher.go、status_reader.go、selftest.go；adapters/gorm与db；Host app/monitor/health.go及原Cloud intake/read映射。代码、manifest与真实消费者优先；计划不是实现证据。
- 已执行验证见[完整矩阵](../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)及四份安全报告；原固定Go1.26构建仍只证明其来源SHA。
- 全部非lessons/fixtures的Markdown设计、阶段与契约文档都列在下表，共50篇；源清单保指定PHP提交，不重新生成或修改PHP项目。课程正文、JSON/源fixture与历史报告不在本轮重写范围，用户练习不代做。
- 本轮仅静态核对；唯一新增执行是修复初始化拥有清单回归后的真实两次init→strict upgrade preview，见[独立修正](../../go-moo-scaffold/docs/plans/initializer-upgrade-ownership.md)。未重复业务全量/E2E、真实Cloud、DDL或部署。

## 关键结论

1. 模板初始化只有新目录O_EXCL和失败partial检查，无receipt/resume；CLI init跨命令不绑定预览，upgrade才要求expected-digest。拥有清单只有摘要，不含Git baseSHA；reference可复制但不能纳入升级拥有区。该回归已独立修复，新工程定向预览通过；旧错误清单不会自动修复。
2. Scaffold五模块、JSON/R1子集、初始化/保护升级、资产与工单两例已交付；完整PHP类型/关系、Actor、TS/Vue、文档/日志/计划门户及AI调用仍后置。能力表逐行区分当前子集，不以一期完成抹掉backlog。
3. Monitor四driver/DB façade/静止Fresh、queue/schedule/运维/MCP本地代码已接。Go心跳元信息已实际兼容；单条runtime的Go版本仍在Cloud映射/存储/读回链丢失，明确待设计，不能拿Project心跳补历史记录。
4. 迁移生成、binary已注册和DB已执行是三份事实；GUI不执行DDL，YAML语义往返不承诺注释原字节保留。受保护业务文件不因新生成模板自动覆盖。
5. 三仓main/dev已推远端、旧任务分支已清理是上轮交付事实；本轮只恢复同名任务分支校准，未再合并或推送。稳定tag、真实远端Go发现/认证、生产MySQL/SQLServer、跨OS及完整真实MCP权限仍未由本地测试证明。

## 逐篇处置

“保留节点设计”表示核对其时间/步骤语境后保留原始设计和验证边界；不是把该节点的“下一步”当今天待办。当前实现与支持范围由上方原设计入口及源码负责。下列短SHA256是本次扫描正文的校验摘要，不是Git提交或执行验证。

| 原文 | 处置 | 核对边界 | 正文摘要 |
| --- | --- | --- | --- |
| [docs/README.md](../docs/README.md) | 正文/边界已修订 | 按当前范围、实现与未验边界校准，不新增实现授权 | `b518d5addd7f` |
| [docs/contracts/explicit-once-http.md](../docs/contracts/explicit-once-http.md) | 保留稳定契约与报告边界 | 稳定协议按源码保留；测试/构建仅证明对应固定来源 | `68a13db2578d` |
| [docs/contracts/gorm-generation-identity.md](../docs/contracts/gorm-generation-identity.md) | 保留稳定契约与报告边界 | 稳定协议按源码保留；测试/构建仅证明对应固定来源 | `d0b2749761f7` |
| [docs/execution-queue.md](../docs/execution-queue.md) | 正文/边界已修订 | 按当前范围、实现与未验边界校准，不新增实现授权 | `bcefd87481ae` |
| [docs/php-surface-inventory.md](../docs/php-surface-inventory.md) | 保留有日期/源SHA的资料基线 | PHP指定提交的静态入口清单，不冒当前动态路由/全部行为 | `7d673cdb2a02` |
| [docs/plans/01-sensitive-masker.md](../docs/plans/01-sensitive-masker.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `24d4ecb42b1d` |
| [docs/plans/02-sql-masker.md](../docs/plans/02-sql-masker.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `1f759ffb6b10` |
| [docs/plans/03-bounded-payload.md](../docs/plans/03-bounded-payload.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `347c515b9c4b` |
| [docs/plans/04-runtime-records.md](../docs/plans/04-runtime-records.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `877c847f9a1c` |
| [docs/plans/05-runtime-buffer.md](../docs/plans/05-runtime-buffer.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `46505922760f` |
| [docs/plans/06-incremental-sync.md](../docs/plans/06-incremental-sync.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `85bfae15dee4` |
| [docs/plans/07-cloud-http-client.md](../docs/plans/07-cloud-http-client.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `c7d1efde3a71` |
| [docs/plans/08-runtime-capture.md](../docs/plans/08-runtime-capture.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `b9b841866e86` |
| [docs/plans/09-http-capture.md](../docs/plans/09-http-capture.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `e4b4c532c9e3` |
| [docs/plans/10-host-runtime.md](../docs/plans/10-host-runtime.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `9f64c4c8eaf7` |
| [docs/plans/11-slow-query-pipeline.md](../docs/plans/11-slow-query-pipeline.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `1cc9b78b6309` |
| [docs/plans/12-gorm-slow-query.md](../docs/plans/12-gorm-slow-query.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `5e072d0e5361` |
| [docs/plans/13-host-slow-query.md](../docs/plans/13-host-slow-query.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `106556cd543f` |
| [docs/plans/14-queue-schedule.md](../docs/plans/14-queue-schedule.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `0150f921ef3e` |
| [docs/plans/15-host-queue-owner.md](../docs/plans/15-host-queue-owner.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `cabbd98430f6` |
| [docs/plans/16-host-schedule.md](../docs/plans/16-host-schedule.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `6fbe8077161e` |
| [docs/plans/17-cloud-push-owner.md](../docs/plans/17-cloud-push-owner.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `f09e100af082` |
| [docs/plans/18-health-check.md](../docs/plans/18-health-check.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `3ac0451655b8` |
| [docs/plans/19-pending-statistics.md](../docs/plans/19-pending-statistics.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `6fbd0786f1d2` |
| [docs/plans/20-local-noise-discard.md](../docs/plans/20-local-noise-discard.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `3f221e7610f6` |
| [docs/plans/21-push-preview.md](../docs/plans/21-push-preview.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `500b4810c865` |
| [docs/plans/22-explicit-all-replay.md](../docs/plans/22-explicit-all-replay.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `54393cc30fb8` |
| [docs/plans/23-confirmed-resolved-maintenance.md](../docs/plans/23-confirmed-resolved-maintenance.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `16ceffd30c5b` |
| [docs/plans/24-migration-inspection.md](../docs/plans/24-migration-inspection.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `af0c177217eb` |
| [docs/plans/25-legacy-record-import.md](../docs/plans/25-legacy-record-import.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `71bccd14fbb6` |
| [docs/plans/26-mcp-stdio-loop.md](../docs/plans/26-mcp-stdio-loop.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `83e505cb38f2` |
| [docs/plans/27-cloud-mcp-tools.md](../docs/plans/27-cloud-mcp-tools.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `48593a0a72f6` |
| [docs/plans/28-host-mcp-entry.md](../docs/plans/28-host-mcp-entry.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `156fdd9a699b` |
| [docs/plans/29-filtered-http-payload.md](../docs/plans/29-filtered-http-payload.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `8ed7053c60b3` |
| [docs/plans/30-source-context.md](../docs/plans/30-source-context.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `b635ca6624d6` |
| [docs/plans/31-postgres-slow-query.md](../docs/plans/31-postgres-slow-query.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `279e9b22f319` |
| [docs/plans/32-db-facade-observation.md](../docs/plans/32-db-facade-observation.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `0c452cae51f5` |
| [docs/plans/33-crash-output.md](../docs/plans/33-crash-output.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `ab79a2afa9b4` |
| [docs/plans/34-crash-diagnostic-import.md](../docs/plans/34-crash-diagnostic-import.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `2aa77ad00c63` |
| [docs/plans/35-cloud-summary.md](../docs/plans/35-cloud-summary.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `fd4caf2aff90` |
| [docs/plans/36-log-capture-policy.md](../docs/plans/36-log-capture-policy.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `4d3c5d069e81` |
| [docs/plans/37-go-build-metadata.md](../docs/plans/37-go-build-metadata.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `c710b4b26817` |
| [docs/plans/38-release-reproducibility.md](../docs/plans/38-release-reproducibility.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `b235f5b24476` |
| [docs/plans/39-self-test-resolution.md](../docs/plans/39-self-test-resolution.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `32e7b1c9686f` |
| [docs/plans/40-mysql-slow-query.md](../docs/plans/40-mysql-slow-query.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `64da5a46477c` |
| [docs/plans/41-sqlserver-slow-query.md](../docs/plans/41-sqlserver-slow-query.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `fe738c6d75f8` |
| [docs/plans/cloud-go-compatibility.md](../docs/plans/cloud-go-compatibility.md) | 正文/边界已修订 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `1623554ba14b` |
| [docs/plans/cloud-source-initialization-isolation.md](../docs/plans/cloud-source-initialization-isolation.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `912e268665ba` |
| [docs/plans/workbench-payload-exclusion.md](../docs/plans/workbench-payload-exclusion.md) | 仅补节点时点；原设计/证据保留 | 节点范围与分批结果保原；当前支持/后续替代见下方权威入口 | `6314690d026c` |
| [docs/port-plan.md](../docs/port-plan.md) | 正文/边界已修订 | 按当前范围、实现与未验边界校准，不新增实现授权 | `1bdf620c6e5a` |

## 本轮验证与交付

Markdown围栏、涉及的1,407处本地链接及三仓diff --check均通过；逐篇摘要用于回读，本索引自身不纳入摘要表。仅新增执行拥有清单修正的真实初始化/升级预览消费者，结果PASS且owned目录清理。原课程、JSON/source fixture、固定构建和全盘机器报告未改；用户IDE stage保留。

初始化拥有清单代码修正独立提交d5f417b；本轮设计校准另按三仓提交，暂不合并、推送、打tag或部署。原实现/测试报告来源SHA保持原义，不能因这轮文档或窄修正而声称重跑全盘。
