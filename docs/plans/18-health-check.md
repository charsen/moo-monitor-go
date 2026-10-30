# 18：健康心跳与显式自检

范围：SDK健康传输/自检完整功能，对应Host48。第17步自动推送独立提交；本步后暂停，不扩resolve/MCP/summary/discard或真实Cloud。本计划保留实现边界与下方实际结果。

独占cloudsync/heartbeat.go、selftest.go与对应定向测试。复用现HTTPClient私有post和已有records/slowquery Builder，不改增量Sync/ACK/业务Buffer；不改17推送主链。Host健康command真实make后深化，Host48两正文/测试独占，shared配置/注册/入口由writer维护。

Heartbeat(ctx,HeartbeatMeta) POST api/v1/heartbeat，body token/meta，固定sdk=moo-go-monitor；只发Cloud既有接受字段sdk_version/app_env/app_name和六bool。Go/Goravel版本仅本地诊断，不伪PHP/Laravel。timeout按源1..4秒，2次/100ms只连接错误，复用拒redirect/TLS/有界响应/安全错误；接收2xx且ok===true。HTTPClient/http.Client浅副本调整本次timeout，不修改共享client。

每轮实际push后（包括空轮/部分失败）best-effort心跳，结果独立不改push成功；dry-run不发送。与17的Heartbeater消费接口协作，第二步共享接入由writer维护，不并发改17文件。

SelfTest显式both/runtimes/slow_sql；前置heartbeat失败不打intake，成功后已有Builder构造稳定可识别样本，直接HTTP，不写本地记录/ACK。一类失败继续另一类完整返回。必须Legacy=false、1条逐项index0/hash相等/status=saved/retryablefalse且聚合1/0/0才通过；filtered/skipped/拒收/旧receipt不能通过。这比PHP confirmSaved仅看saved===1更严格，正常Sync旧兼容不变。

Host moo:cloud:test命令明确提醒发测试记录，自动Cloud关闭仍可显式执行但URL/token必须配置；本轮只httptest。--resolve明确unsupported，不擅用额外mcp能力。错误与输出不含token、URL、原body或自由文本Cloud错误。

定向验证：真实httptest心跳wire/无数据周期依赖/timeout与取消、严格ok；SelfTest两类/类型门禁、saved/filtered/skipped/legacy/畸形及heartbeat失败零intake、稳定样本与不落盘；Host真实命令调用同一服务；仅受影响旧HTTP测试，不全量/E2E/真实网络。用户自主练习待完成。


## 阶段实际结果

2026-10-30，实际Go1.27.3、GOWORK=off：首轮`Test(Heartbeat|SelfTest)`通过0.459s；补新path重定向/HTTP状态/响应上限/timeout两端后同范围race1.487s。新增AfterPush实际HTTP三个场景fixture首次20s超时失败；先drain body并有界等待修后，唯一`TestHeartbeatAfterPush`race1.317s。失败进程已退出，自有精确临时目录白名单清理，未读record/log内容。稳定时间/身份沿原有请求补断言后唯一SelfTestHTTPPipeline项race1.409s；第17步已独立提交后，Host48共享接线落地，实际10模式CLI一次race通过5.459s；真实bootstrap.Artisan.Run(os.Args,true)于子进程产生exit0/1，不替OsExiter。未配置/非法type/resolve出现含false均零请求，显式Cloud关闭仍可执行；业务Buffer/ACK Root未写。

与PHP差异：runtime自检是Go自己的固定身份，慢SQL沿同raw/location可能同hash；SDKVersion未发布为development，Go/Goravel只用于本地诊断及已有记录Context；旧receipt严格拒用于自检，正常Sync兼容不改。重复自检count1，Cloud按max快照而非累加；未自动resolve/清理。更短caller context优先于1..4秒心跳client timeout。

命令沿原框架返回普通error，真实CLI成功0/失败1；PHP输入/配置INVALID为2，这里明确差异。避免ExitCoder使原urfave在Call(...,false)也触发全局OsExiter，保证显式Call可以返回error；不改全局退出机制。所有--resolve出现（含=false）均拒绝。


最终Host命令（四module workspace，实际Go1.27.3）：

```sh
go test -mod=readonly -race ./tests/feature -run '^TestMonitorHealthCommand$' -count=1 -timeout=90s
```

父进程清空DB/认证/Cloud自动开关/OTel/AI等出口，仅子进程显式设置本机httptest URL和固定假token；日志为相对应用根指向自有临时native.log。健康命令与AfterPush共用Host唯一HeartbeatMetadata，构造不联网；AfterPush本轮用父ctx/owner lifetime，独立于intake轮次deadline。SDK18自身无module依赖变化。受影响旧Host推送消费者由writer单独验证，结果分栏记录。

受影响旧Host消费者由writer执行：metadata配置定向0.437s；原推送manual/partial/missing/heartbeat-failure/auto五子项race8.950s，含空轮/失败轮心跳计数、白名单meta与健康失败只Warning且原Call成功。与本课CLI10模式5.459s分栏，不重复旧全套或SDK生命周期。
