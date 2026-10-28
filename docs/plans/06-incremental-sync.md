# 第06步：增量快照、严格回执与持久确认

仅Monitor。生产能力为显式cloudsync.Sync+窄Client接口，测试只fake client/httptest，不连接真实Cloud、不做自动调度/采集，不宣称HTTP adapter或真实云兼容完成。Git仍由实施统一。

## 已审API和状态机

buffer新增Snapshot(ctx)与Compare(ctx,snapshot,Decision{Disposition,Reason})、只读ScopeRoot边界。锁内先恢复05intent，再扫描open/resolved，Snapshot包含hash/bucket/timestamp/实际存储YAML SHA256/typed Record。网络前释放buffer热锁。Compare重新读同bucket并比时间戳+digest；Confirm保open累计anchor、只删除resolved；Quarantine先完整原子落hash-digest.yaml和hash-digest.receipt.json、核已有字节一致，再删源，失败保源、重试幂等，冲突error不覆盖。deleted不扫描不清，不增加本地Resolve/Delete。

永久拒收旁车固定version/hash/digest/status=skipped/retryable=false/reason。source只做ASCII safeReason替换且无cap，无法确保自由文本无凭据；Go只保已证分类码invalid_record/ingest_filter/upsert_failed（大小写原样），未知或自由文本统一rejected。旁车与公开Result共用同一规则，不持久/输出原reason；Receipt仅内存用于严格类型/非空判定。

cloudsync.Client为Send(ctx,[]buffer.Snapshot)(Receipt,error)，Sync每批再次验证typed Receipt，Decoder另严格验证原wire存在/类型。顶层2xx+ok true+saved/filtered/skipped非负整数/数量闭合；results缺席仅skipped0旧成功兼容，present null/标量/错shape拒；按实际PHP数组或对象values两形态读取，不依赖对象键顺序，最终按index排序。逐项index唯一合法/hash一致/status已知/retryable bool/reason存在null|string，非skipped不得retryable、skipped须非空reason，status数量闭合后按index排序。畸形整批无新确认。

同scope/runtime独立非阻塞同步锁，state文件sync-runtimes.json。账本version1、cursor、confirmed:map[hash]{digest,timestamp,bucket}，每hash仅一个权威确认；只有当前fingerprint/bucket精确匹配才跳过，永不仅因timestamp<=cursor跳过。同毫秒count变化、时间回退的新内容仍会发送。清理不live的旧确认防幽灵账本。

cursor是本SDK当前扫描的连续安全处理前缀，按timestamp分组：同组有未确认洞就不能越过；新洞回退到旧cursor前时允许水位回落/空。无洞保安全历史水位并取max。它不是PHP时间-only单调水位协议原样复刻，账本才是确认判据。网络前可持久GC/安全水位回落，坏receipt不能产生确认或推进未处理记录。

Sync顺序为独立push锁→读严格state→buffer短锁快照/释放→筛精确账本→Send→整批回执验证→逐项Compare→短写检查/原子替换state。saved/filtered匹配成功确认；retryable留洞；非retryable同版本隔离。每批持久成功项，洞不阻塞后项。HTTP过程中允许Record更新，旧版本ack不能删/隔离新文件，下一轮重扫新digest。

## 持久性与范围

ScopeRoot为真实包间消费边界，需核os.Root身份、私有权限、拒链接、NOFOLLOW锁及inode，不能仅信路径字符串。cloudsync固定JSON state原子写/短写校验/目录sync，坏state拒绝并保原。不会把05journal当跨云事务：云已确认但本地记账失败可能重发；resolved同版本删除后state写失败也可能已无记录，不能凭旧ack删新事件。至少一次而非exactly-once。

06仅runtime。测试HTTP adapter只在_test.go中使用，真实鉴权/连接异常重试/非法UTF8 JSON编码策略另步；Cloud累计count按快照max、不求sum，open始终留作累计anchor。

## 验证安排

源纯probe仅SafelyLogs/CloudClient/CloudSync三声明、拒autoload、显式假cfg/cursor，反射parseItemResults/recordTimestamp/isAcknowledged/epochFloat，无HTTP/Host/文件写，脚本先审后采raw fixture。Go定向含混合四结果、失败洞后成功/重开、同毫秒新count与时间回退、网络中Record、版本变更不删、坏receipt无确认、scope/锁、open保留、旁车半完成/冲突/重开原因、状态失败至少一次恢复。只受影响buffer/cloudsync定向，必要并发race；不跑全量/E2E/外部服务。用户练习待完成，未知结果不填通过。

## 实际结果

2026-10-29真实源pure probe已采18receipt、3空wire形状、5版本比较与3timestamp选择，PHP8.3.35/source28d424e；raw fixture保持原stdout并重放逐字cmp。results对象values与sent0空[]/{}实际接受，null拒；meta空串不fallback。只调用已审三声明/private组件，不调用send/sync/HTTP/Host。

新增buffer五项最终race PASS2.441s、cloudsync十项最终race PASS3.073s（Go1.27.3/GOWORK=off）。覆盖同毫秒发送中count变化旧ack不确认、resolved复发不误删、失败洞后成功持久/重开只发失败、畸形整批零确认、水位回退、scope与非阻塞push锁、网络不占热锁、旁车各写入/删除故障及冲突保源、resolved删除后state写失败新event不跳过、短写/坏state/链接与错误cause。之后仅文档修改；不是全量、真实Cloud或HTTP生产adapter验证。

原YAML/旁车两文件不是事务，重复拒收reason不同会因旁车bytes冲突保源。Result.Pending=0仅本轮快照，不覆盖扫描之后新事件。详课含调用链、重放命令、早轮/最终证据与用户练习（待完成）。
