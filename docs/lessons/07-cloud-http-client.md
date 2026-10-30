# 第07课：显式HTTP传输、字节策略与有限重试

[上一课：06](06-incremental-sync.md) · [课程目录](../README.md) · [下一课：08](08-runtime-capture.md)

06有增量同步与严格回执，但只通过Client接口和替身发送。本课新增[HTTPClient](../../cloudsync/http_client.go)，把真实本机HTTP请求接入同一个Sync；不连接真实Cloud，不自动采集或调度。范围见[本步计划](../plans/07-cloud-http-client.md)。

## 两个动作的源契约

[PHP CloudClient](../../../moo-monitor-laravel/src/Cloud/CloudClient.php)本课涉及两个公开动作，路径固定：

| 动作 | POST路径 | JSON body | 回执 |
| --- | --- | --- | --- |
| Send | api/v1/runtimes/intake | token、records数组 | 06严格逐项/旧完整回执 |
| DiscardLocalNoise | api/v1/runtimes/discard-local | token、env=local | 2xx、ok严格true、deleted非负整数 |

认证在body.token，不是Authorization。真实[ResolveProjectToken](../../../moo-scaffold-cloud/app/Http/Middleware/ResolveProjectToken.php)还检查token长度、字母数字与能力；客户端只要求非空/可编码，不把服务端认证规则复制成另一套校验。示例和测试只使用固定假token。

Discard方法只完成远端传输协议，返回deleted数量，不删除本地记录或修改06账本。真实[LocalNoiseDiscardController](../../../moo-scaffold-cloud/app/Http/Controllers/Api/LocalNoiseDiscardController.php)只处理项目env=local的open/in_progress，resolved和其它环境不动；后续本地pending清理协调器另行设计。本课没有执行该真实服务。

## 构造不联网，调用才发送

```go
client, err := cloudsync.NewHTTPClient(cloudsync.HTTPConfig{
    BaseURL: cloudURL,
    Token: projectToken,
    Timeout: 5 * time.Second,
})
if err != nil {
    return err
}
defer client.CloseIdleConnections()
syncer, err := cloudsync.New(store, client, cloudsync.Config{})
if err != nil {
    return err
}
defer syncer.Close()
result, err := syncer.Sync(ctx)
```

配置明确传入，不读MOO环境变量或全局Host容器；默认自有Transport也不采用ProxyFromEnvironment。BaseURL允许明确前缀，随后拼固定API路径；只http/https、有host，拒userinfo/query/fragment/opaque。构造错误不回显原URL或url.Parse自由文本，不接受外部传来的自定义动作URL。

Token不放日志、header或URL。错误只有安全分类、operation、status和attempt；body或云端error自由文本不透传。HTTPError通过errors.Is识别ErrWire/ErrHTTPTransport/ErrHTTPStatus/ErrHTTPReceipt/ErrResponseTooLarge，Unwrap保留底层cause供程序识别；对外记录错误应使用其安全Error文本，不自行dump底层请求或数据。

## client与transport由谁拥有

默认创建stdlib http.Client/Transport，证书验证开启、TLS最低1.2；VerifyTLS显式false才应用源可配置的关闭验证能力，SDK不会自动降级。Timeout默认5秒，是每次尝试上限；整个多次尝试仍受caller ctx控制。

可以注入Client用于明确的连接策略或测试。SDK复制client，配置自己的timeout和拒重定向策略，不改原client；清掉副本Jar以免把不相关cookie带入token API。借用Transport的TLS策略归owner，Client与VerifyTLS同时配置会报错，不能假装覆盖一个不透明RoundTripper的证书策略。

CloseIdleConnections只关SDK自己创建Transport的空闲连接，不关闭借来的Transport；借用者自行管理它的退出。使用自有client的对象仍需在生命周期末尾调用此方法，不生成后台worker或自动绑进Host。

所有重定向默认拒绝，不能仅依赖net/http跨host删除Authorization：本协议的凭据在JSON body，307/308可以重放POST。定向测试分别让307/308指向第二个httptest host，第二host请求数均为0；这是明确安全差异。

## 发送的是Record，不是Snapshot包装

[http_wire.go](../../cloudsync/http_wire.go)逐项取Snapshot.Record，body只有token与records。版本Digest/Bucket/Timestamp包装不发送；Record自己的hash/status等仍按字段白名单输出。这样06的存储版本保护不会污染Cloud记录协议。

完整Record包括nullable resolved字段、exception.previous、Request、真实GoContext、trace/app_frames、source_snippet、payload和meta。大int64不经过float64中转；JSON空数组、null和有序对象形状保持。空object与array在JSON编码层可以区分，M07 Builder此前把空payload对象规范成[]的规则不因此改变。

真实[RuntimeIntakeController](../../../moo-scaffold-cloud/app/Http/Controllers/Api/RuntimeIntakeController.php)消费嵌套字段，count按快照max合并；重复传输不能当增量sum。当前该消费者只读取Context env/project/php/laravel/occurred_at，go/goravel会被忽略。Go仍发送真实GoContext，不伪造PHP版本；传输或回执成功不证明云端Go版本字段已落库，后续需独立兼容范围。

## 为什么不能让encoding/json自动修坏字节

04/05的trace按字节截断，YAML可用!!binary保留非法UTF8；这不等于JSON可以原样承载。当前Laravel asJson→post的json option进入已装Guzzle Client，实际调用原生json_encode默认flags，没有INVALID_UTF8_SUBSTITUTE，编码失败会抛异常。

Go encoding/json会把坏UTF8替成U+FFFD，因此SDK在编码之前完整验证typed Record所有字符串、指针、previous、frame、meta/payload动态容器及key。数字必须有限，未知struct/Stringer/MarshalJSON不会被调用；动态树深度上限32，已生成payload本来已有五层预算规则。

坏trace、坏metadata文本、Inf与未知Marshaler都返回ErrWire，整批零HTTP请求，不局部丢记录，也不把替换后的文本称为源原样。这里验证wire，不负责重新采集/脱敏；仍应先通过Builder/Buffer得到Snapshot，源码片段仍有此前明确的内容策略边界。

[原生PHP编码probe](../../tools/probe-cloud-json.php)只对固定假值调用json_encode，记录精确Guzzle源码SHA，不require/autoload/Host/HTTP。[raw fixture](../contracts/fixtures/cloud-json.json)有16例：空数组/对象、有序对象、大int64/最大int、float、Unicode、null/previous、PHP/GoContext两表示、非法UTF8/切断trace、NaN/Inf。

这证明编码步骤，与已读Guzzle调用点分开记录；没有冒充整套PHP HTTP客户端测试。Go比较JSON语义，不要求PHP的Unicode/斜杠转义与Go字节完全相同。

## 只对连接失败有限重试

Send最多3次外层Do，每次间隔200ms；Discard最多2次、100ms。每次重建相同body的request，Accept/Content-Type均application/json。stdlib Transport自身也有其连接恢复机制，SDK这里描述的是外层有限尝试，不宣称控制每一次底层TCP拨号。

只对已审网络类重试：net.OpError、DNS、连接EOF/UnexpectedEOF或net.Error.Timeout；任意error、编码/配置错误、确定性证书验证错误不重试。HTTP4xx/5xx与拒绝的重定向已有响应，直接返回安全status错误，不即时重发POST。

单次Client.Timeout发生而caller ctx仍有效时可以重试；caller ctx已取消/截止则立即停止，不继续睡或重发。两者不能仅凭错误里有deadline文字混为一谈。退避使用timer+ctx选择，取消不需等满200ms。

源global retry实际会捕获callback异常；Go按本次已审范围缩为连接类，不对无法编码的数据反复尝试。即使连接响应丢失后重发，仍属于至少一次；06的账本和云端快照幂等契约继续适用，不增加跨云事务。

## 有界响应与严格回执

默认最多读取1MiB响应body，配置MaxResponseBytes可显式改变；读取limit+1便能判断超界并关闭body，不无限读。这个上限是Go保护差异，不冒称源已有相同限制。

Send把2xx响应交06 DecodeReceipt，坏回执不被确认；Discard独立检查ok严格true和deleted存在/整数/非负，null、1.0、负值、缺字段和坏UTF8均拒绝。HTTP错误不输出云端error文本，即使里面含假token也不会进入HTTPError文本。

## 实际验证与曾经失败的夹具

2026-10-29，Go1.27.3/GOWORK=off首次新增六项普通测试1.602s。设计审查指出单次timeout应按源重试，修正后新增timeout三分支与真实HTTPClient+Sync混合回执、门禁相关四项普通通过2.568s。

中间timeout假服务未读request.Body就等待r.Context.Done，导致httptest.Close挂住。本次只终止该测试子进程，并按lsof确认的唯一自有目录逐项验证后清理；随后先消费Body、再等取消，另加200ms fallback，避免测试夹具收尾无界。该失败不是业务传输通过，未吞掉日志或伪标成功。

新增八项HTTP定向race最终通过4.559s；之后只给timeout夹具补明确fallback，单独该项三子例race再通过2.263s，生产代码未再变。覆盖：

- 真POST捕获假token、固定路径、headers与完整Record，巨大整数/null保持，Snapshot包装不发送。
- 06 Sync通过真实HTTPClient处理saved/filtered/retryable/permanent混合回执，重开后仅发送失败洞，open锚点不重复发。
- 连接失败次数、任意error/HTTP不重试、单次timeout后成功/连续timeout上限、caller截止和退避取消。
- TLS默认失败/显式受信client成功，借用client未改；307/308第二host零请求。
- 响应上限、坏Discard回执、缺配置与URL门禁、坏UTF8/Inf/未知Marshaler零请求，错误文本无假token。

实际定向命令：

```sh
GOWORK=off GOTOOLCHAIN=local "$HOME/Library/Caches/go-moo/toolchains/go1.27.3/bin/go" test -mod=readonly -race ./cloudsync -run '^(TestHTTPPostWireAndDiscard|TestHTTPWireSourceFixture|TestHTTPRetriesAndSafeErrors|TestHTTPContextAndLimits|TestHTTPRedirectAndTLS|TestHTTPNoConfigOrBadWire|TestHTTPAttemptTimeouts|TestHTTPSyncMixedReceipts)$' -count=1 -timeout=30s
```

编码源probe重放：

```sh
php -l tools/probe-cloud-json.php
php tools/probe-cloud-json.php ../moo-engine-skeleton/engine/vendor/guzzlehttp/guzzle/src/Client.php
```

没有运行真实Cloud/真实token、PHP Host HTTP、全量/E2E、Linux或最低Go1.26。Discard只向本机假服务发送协议；本地清理协调器、Cloud Go字段兼容、自动采集/调度、M18真实Host隔离仍未完成。

## 用户练习（待完成）

先区分单次client timeout与caller截止的尝试次数，说明为什么HTTP500和任意error不能跟着重发。再画307/308的body重放路径，解释为什么不看Authorization也必须拦重定向。

手算一条含巨大int、空payload、previous与非法trace的记录应在哪一步失败或发送。解释传输成功、回执确认与真实Cloud Go字段持久化之间的区别，以及Discard为什么不能顺便清本地。代理测试通过不代做这些练习。
