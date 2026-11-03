# 17：显式与周期增量推送

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

本轮第一功能，对应Host47；第二功能为心跳/saved级健康检查，完成两个功能后暂停。先SDK→Host分别验收commit，不自动push。

复用现Syncer/SlowSyncer与HTTPClient，不新建账本、采集器或全局网络hook。PushTarget为both/runtimes/slow_sql；每类失败继续另一类，保留Busy/Pending/Confirmed/Rejected与明确Skipped。Push手动缺源失败；Tick配置缺源跳过。正常无变化成功。--all/--dry-run/retention本块明确不支持，后续M13继续；当前Confirm resolved立即移除，与PHP retention后prune不同，不冒称完整命令等价。

Pusher仅借Buffer/client，拥有两Syncer的目录句柄。允许并发调用，原每类型gate/flock决定Busy；Close先Stopping、取消在途context、等全部退出才关Syncer，不关借用Buffer/HTTP，取消等待非Closed且可重试。Host独有HTTP transport在其后关闭；自动owner150先于Runtime200关闭，首tick后运行，Boot不HTTP。手动一次结束释放自有句柄/transport，不关本地Buffer。

源证据：PHP CloudPushCommand按type逐类sync，失败仍下一类，成功非skip非dry才prune；每个非dry轮尾best-effort heartbeat，即无变化也发。Provider每分钟withoutOverlapping10分钟；源Sync本身还有type flock。Go不复制scheduler互斥账本，当前依赖已证type gate/flock。第一块仅预留可选轮尾健康callback，无默认假成功；第二块真实接入独立HealthError且不改push结果。

验证限私有目录+httptest：两类型部分回执/后端错误继续、Busy/跨实例锁、空轮、取消/Close重试；实际Host配置/命令/定时owner与disabled无外联。必要定向race，不全量/E2E/真实Cloud写。执行结果见下。

## 2026-10-30最终证据

SDK四个TestPusher命名项race PASS1.657s；HostConfig/OwnerBoundaries race1.645s，六模式真实Bootstrap/Artisan/本机HTTP race9.873s。最终仅CLI存在性修正后，公开cli.IsSet明确拒绝--all=false/--retention=，候选HEAD overlay隐藏48的Host3个/SDK4个Go文件，SDK17独立四项PASS1.103s、Host47唯一六模式PASS2.603s。原文/overlay/父日志均限本次自有目录并已清，工作树未改写；不把18健康验证计入17证据。

首轮SDK Close取消夹具未等异步cancel传播，修为真实等待ctx.Done；Host回执替身漏retryable/reason、误认HTTP503也重试、私有Buffer测试目录未chmod，均按现契约修夹具，未放宽生产HTTP或文件门禁。最后CLI只窄存在性变化，未重复已通过生命周期race。当前未发送真实heartbeat，AfterPush默认nil；48完成后才实际接入。

## 当前契约衔接（2026-11-03校准）

Pusher现有Incremental/All、Stats/Preview与精确resolved维护。disabled类型只关闭传输，健康源仍可本地读取；disabled坏锁静态错误不阻健康推送，明确读坏type才Unknown。enabled源初始化仍fatal，见cloud-source-initialization-isolation。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
