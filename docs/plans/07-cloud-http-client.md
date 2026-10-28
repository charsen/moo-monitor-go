# 第07步：生产HTTP传输与本机协议联调

仅Monitor，实现满足06 Client的runtime Send及独立DiscardLocalNoise传输方法；无自动外联/调度，不删除本地数据，不连接真实Cloud。06已提交后维护导航/入口，Git由实施统一。

## 已核源与wire方案

源CloudClient固定POST api/v1/runtimes/intake，body={token,records}；discard固定api/v1/runtimes/discard-local，body={token,env:local}。前者最多3次/200ms、后者2次/100ms，仅连接错误重试；HTTP响应不即时重试、ctx取消立即停止。源实际global retry会捕获callback异常，Go按本次批准范围不重复编码/配置/TLS确定性错误。

已装Laravel asJson→post JSON option→Guzzle Client原生json_encode默认flags，没有INVALID_UTF8_SUBSTITUTE；编码失败抛异常。07编码前完整验证typed Record所有文本/动态容器有效UTF8、数字有限，未知对象/MarshalJSON不调用，坏trace返回ErrWire且零请求。只发Record字段白名单，不发Snapshot digest/bucket/timestamp包装。空数组/有序对象/数字/null/previous按JSON语义对源，不比较转义字节。

Cloud真实RuntimeIntakeController.map接嵌套exception/previous/request/context/trace/snippet/payload，count按max；当前Context只落env/project/php/laravel/occurred_at，go/goravel会忽略。Go发送真实GoContext，不伪造PHP版本，传输成功不等于Cloud版本诊断已兼容。

原生PHP json_encode固定假值probe可读精确Guzzle SHA，但不require/autoload/Host/HTTP；记录成功JSON与失败code/base64输入，作为编码步骤实证，不能冒称全PHP HTTP已运行。

## HTTP配置和安全边界

新增cloudsync.HTTPClient，显式BaseURL/Token/Timeout/TLS验证/MaxResponseBytes和可借用http.Client。缺配置构造失败，不读全局env，不在构造时发送请求；默认自有stdlib Transport校验证书，CloseIdleConnections只关闭自有transport的空闲连接，借用client/transport不被改写或关闭；SDK取client副本并清Jar、覆盖timeout/拒重定向，借用Transport TLS策略归owner，Client与VerifyTLS不能同时设置。请求策略使用client副本，独立timeout及拒重定向，不能把token转发其他host。

BaseURL只http(s)，必须有host，拒userinfo/query/fragment/opaque；动作路径固定，不接受外部自定义URL。默认5秒每次尝试、1MiB响应上限（明确Go保护差异）；只限量读取，不回传原云error/body/token。单次Client.Timeout且caller仍有效时按net.Error.Timeout重试，只有caller取消/截止立即停止；TLS证书确定错不重试。错误文本固定安全分类/状态，保可识别cause供errors.Is，不能依赖底层url.Error自由文本作为对外错误。

Send对2xx调用06严格DecodeReceipt；Discard只接受ok严格true及deleted存在非负整数，不自动协调本地清理。重定向直接拒绝、非连接错误不重试、有限响应与无效UTF8失败属于明确安全适配。

## 验证安排

只httptest和可注入RoundTripper固定假值：完整Record/body.token/两个路径与headers、06 Sync真实client混合回执、连接重试次数、HTTP不重试、timeout/取消/退避可取消、TLS默认失败/显式可信client成功、重定向不发第二host、响应上限、无配置零请求、坏UTF8/NaN零请求和错误不泄原token/body/error。已有06模块只做必要受影响定向，不全量/E2E或真实Cloud。用户练习待完成，实际结果见下节。

## 实际结果与边界

2026-10-29，PHP8.3.35 native json_encode真实采16固定样本，raw fixture与精确Guzzle SHA可重放；未加载PHP Host/HTTP客户端。Go1.27.3/GOWORK=off新增六项首轮普通PASS1.602s；修正单次timeout重试后相关四项普通PASS2.568s。新增八项HTTP定向race PASS4.559s，之后只补timeout测试handler的明确200ms fallback，单项三子例race PASS2.263s，生产代码未再改。

真实httptest覆盖完整Record/body假token、两个路径/headers、06 Sync四结果+失败洞重开、连接/单次timeout重试上限、caller取消、HTTP不重试、TLS默认失败/显式受信成功、307/308第二host零请求、响应限额、坏wire零请求与错误不泄假token。曾发生timeout假服务未消费Body就等取消、Close挂住；仅终止自有测试PID，lsof确认唯一owned目录白名单清理，修夹具后按上述结果重跑，不伪记通过。

源码消费者已核GoContext字段暂被现Cloud忽略；Discard只传输，不清本地。没有真实Cloud/真实token、PHP Host HTTP、采集器、全量/E2E、Linux或最低Go1.26验证。用户练习待完成，详见[第07课](../lessons/07-cloud-http-client.md)。
