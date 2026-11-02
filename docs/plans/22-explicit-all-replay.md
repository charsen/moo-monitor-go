# 22：显式--all重推当前版本

本轮第二功能，对应Host52；第一功能dry-run已独立SDK a714bdf / Host e414f7a，分别提交后暂停。不做retention/prune、不真实Cloud/E2E/全量，不push/merge/tag或新分支/worktree。

## 源与已确认规则

PHP CloudSync209–246在all时忽略cursor/ACK，读取open/resolved；未带all仍增量。Go保持此显式重推，但新增的exact suppression是用户discard意图，All仍保护（包括同版本同时有历史confirmed与suppressed的保守重叠）。不能用Stats confirmed优先分类反推All选集，Preview.Selected按同一operation selection逐快照计算。

Sync(ctx)保原增量，新增SyncWithOptions(ctx,SyncOptions{Mode:Incremental|All})共用主链；手动Pusher.PushWithOptions，自动Tick固定Incremental。Host现moo:cloud:push启用--all=true，false仍增量；--all+--dry-run按All预览零HTTP/heartbeat/ACK/抑制写。不是--all就删除或重置历史ACK/cursor，也不解除suppression。

Result.Pending表示当前快照未确认实际待推；新增Remaining表示本次选中版本尚未取得合法处置。已有历史ACK的版本本次retryable可能Pending=0，但Remaining>0必须失败。saved/filtered只有合法receipt且Compare精确版本成功才本次完成；永久拒收沿原隔离规则，不能复用旧ACK把新失败抹成成功。cursor仍只认真实ACK，普通状态/批次持久化/同版本保护保持。

## 文件分工与验证

独占cloudsync/replay.go（模式/精确选择谓词）、replay_test.go、22两正文，Host52独立replay测试与两正文；公共Sync/Pusher/Preview/pending和既有push命令由writer统一按精确方案整合，不复制一套同步器或新并行命令。

最小本机httptest：confirmed再次发送、suppressed与重叠仍排除、全量预览选集一致且不联网、retryable/HTTP失败/坏receipt时Remaining与Pending分开且历史ACK不抹、合法部分处置/重启重试、同毫秒新版本不旧确认、runtime/slow类型和Tick仍增量；Host实际--all/false/combined dry-run与安全输出。只本块定向与必要旧消费者，不真实Cloud/全量/E2E。

实际结果见下方，用户自主练习待完成。本轮两功能结束暂停，不开启第三功能。


## 最终接口与实际结果

复用SDK21的SelectionMode，replay.go仅新增All与SyncOptions；selectionReason统一模式选集/排除原因，正常统计confirmed优先不变。SyncWithOptions/PushWithOptions在原主链运行，Tick固定Incremental；Host仍原moo:cloud:push，不造平行命令。All excluded_confirmed为0，任何exact suppression（含重叠）不越过。

All只处理当前open/resolved，不复原已删除resolved/deleted/rejected；不会通过清空匹配历史ACK强迫重发。Remaining为本次余项，Pending为既有快照实际待推；Remaining0后ACK持久化失败仍error/Failed。不存在新的持久operation账本；再次All再选全部eligible，不承诺只重试失败项。

2026-11-02，Go1.27.3，SDK GOWORK=off：四组新增TestAll功能真HTTP首测1.717s，最终race2.787s。首compile因测试unused import失败，删未用import后通过；没有改生产逻辑绕测试。失败历史ACK/partial/new-version/state-failure、精准抑制、预览和两类型/自动Tick均有断言。Host新八模式race11.254s由writer并行执行；旧共享SDK选定race2.973s、旧HostPreview五模式race7.636s、旧invalid子例0.847s分栏。只本机假Cloud与自有buffer/log，未真实Cloud或全量/E2E。

```sh
GOWORK=off go test -mod=readonly -race ./cloudsync -run '^TestAll' -count=1 -timeout=45s
```

这是四组新增All功能定向，不是全仓测试。Host新测试为TestMonitorAllReplay，八隔离模式详见Host52计划。框架/HTTP/Compare的原语义保持，已通过项无新疑点不重复。完成本轮21/22后暂停，用户练习仍待用户完成。
