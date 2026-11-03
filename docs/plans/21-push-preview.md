# 21：只读增量推送预览

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

2026-11-02，第一功能。复用19的type gate/flock、严格ACK/suppression与ReadOnlySnapshot，21节点初始仅Incremental；22已增加All，当前Preview与实际Sync使用同一选集且保exact suppression，未知模式拒绝。StatusReader/Syncer/Pusher.Preview返回Known/Busy、nullable Counts/Cursor/Selected/WouldSend，不输出记录payload。Selected为本地增量候选，不证明Cloud配置、权限或接收结果。

Stats与Preview共用同一判据，不另建账本。Preview操作不HTTP/heartbeat/save/GC/recover；Reader构造可能准备锁、既有Bootstrap可能恢复，不能把整个进程称绝对零写入。无源/坏状态/journal/Busy不伪装有效零。没有支持all或retention，本轮第二功能再接All。

## 实际验证

SDK新TestPreviewReadOnlyAndSelection首测0.883s，含1ACK/1suppression/1pending，预览文件SHA不变、零发送，后续真实Sync选择同一记录；不支持mode拒绝。最终Preview与现Stats定向race2.098s。Host51五模式真实Bootstrap/Artisan命令race7.637s，Cloud关闭/无token/禁推开关仍预览本地，零HTTP与文件字节不变，坏ledger/journal/Busy/无源明确Unknown，dry-run=false走正常命令，all包括false仍拒绝。全部私有目录/本地HTTP spy，无真实Cloud/全量/E2E；不重复前轮测试。

本步当时独立提交并暂停；22显式All和23精确resolved回收现已完成。Tick仍Incremental，不将All当重发所有被抑制记录，也不按天龄删除未确认项。
