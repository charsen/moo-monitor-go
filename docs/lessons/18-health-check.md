# 第18课：健康心跳与saved级自检

[上一课：17](17-cloud-push-owner.md) · [课程目录](../README.md) · [下一课：19](19-pending-statistics.md)

本课是两个新功能中的第二项，完成后暂停。它复用现有成熟HTTP、Builder和intake确认，不引入Cloud服务端修改。范围见[计划](../plans/18-health-check.md)；Host采用对应模板第48课，用课号定位，保持两仓提交顺序独立。

| 文件 | 职责 |
| --- | --- |
| [heartbeat.go](../../cloudsync/heartbeat.go) | typed白名单、固定路径、短timeout与安全心跳确认 |
| [selftest.go](../../cloudsync/selftest.go) | 显式类型、前置心跳、固定Builder样本与saved结果 |
| [heartbeat_test.go](../../cloudsync/heartbeat_test.go) | 真本机HTTP、重定向/取消/重试及推送后心跳 |
| [selftest_test.go](../../cloudsync/selftest_test.go) | 两类intake、严格确认、稳定身份与门禁 |

## 心跳是管道活性，不是数据保存确认

PHP的CloudPushCommand每轮真实运行都会心跳，哪怕没有变化。Cloud只刷新项目last_heartbeat_at/meta，不改代表人为操作的updated_at；这样“无异常”不等于“推送程序已停止”。Go通过Pusher.AfterPush消费同一Heartbeater，空轮或intake失败仍尝试，HealthError独立于上传结果，不造ACK。

心跳POST body为token/meta。当前Cloud只接受sdk/sdk_version/app_env/app_name和六bool，Go固定sdk=moo-go-monitor，不提供虚假PHP/Laravel版本。Go/Goravel版本只作本地诊断及已有记录Context，Cloud新平台字段兼容仍待验证。

Heartbeat复制HTTPClient与http.Client值，借用原transport，只修改本次timeout。沿源clamp至少1秒最多4秒，100ms配置也会升为1秒；更短caller context仍可取消。两次/100ms仅重试连接错误，HTTP状态不即时重试；继续原证书策略、拒全部redirect和响应上限。返回安全类别，不带URL/token/原Cloud自由文本。

## 显式SelfTest不会落本地业务记录

NewSelfTest仅构造。Run接受both/runtimes/slow_sql，用户调用时才网络写入；Host命令先提示将发测试记录。自动Cloud关闭不禁止显式测试，但配置和参数必须有效；resolve/MCP不在本块，不能悄悄当成功。

先心跳成功，再用已有records.Builder与slowquery.Builder构造固定可辨识样本，直接发送。没有业务Buffer、cap、journal或ACK写入，也不真的执行示例SQL。一个类型失败不会掩盖另一个类型结果。

Go runtime固定异常类/虚拟file/line决定自己的稳定hash，不会伪造PHP实际文件身份；慢SQL沿相同raw/location可能有相同hash。每次样本count1，时间来自同一Run的一次注入clock；重复运行hash不变、更新时间变化。Cloud计数按累计快照max，不把这些count1重复相加，不自动清理或resolve。

## 为什么legacy、filtered仍失败

仅HTTP200或ok=true不足。自检必须聚合1/0/0、非Legacy、完整唯一逐项index0/hash匹配/status=saved/retryablefalse。filtered代表未证明入库，skipped可能需重试或永久拒收，旧无results的回执没有足够逐条证据。

这是Go更严格的自检边界。PHP confirmSaved当前只看saved===1；正常Sync旧回执兼容仍保留，本块没有扩大修改确认协议。

## 实际证据与练习

Go1.27.3，SDK单module GOWORK=off；首轮命名健康项0.459s；新path安全策略后必要race1.487s。推送→真实HTTP心跳空轮/确认后心跳失败/intake deadline后三场景，首次假handler未读body就等取消导致20秒超时，修为drain与有界退出后唯一同项race1.317s。失败记录不伪写通过，不读或输出原payload；全部使用假值、本机httptest，无真实Cloud/E2E/全量。

稳定hash/每Run共享时间且下次更新时间变化，沿已有请求补断言后单项race1.409s。Host48真实命令10模式一次race5.459s：实际CLI成功0/失败1，不替OsExiter；自动Cloud关闭仍显式执行，业务Buffer/ACK Root不写，非法输入和resolve含false零请求。PHP INVALID为2，Go普通error保Call可返回，原CLI统一失败1；不是完整退出码映射或真实Cloud验收。

用户自主练习待完成：解释心跳与saved的两层证据；对比空轮、上传失败、健康失败的结果；说明clamp和caller deadline；解释稳定身份与不落业务Buffer的意义。代理验证不代做学习验收。

受影响旧Host消费者由writer执行：metadata配置定向0.437s；原推送manual/partial/missing/heartbeat-failure/auto五子项race8.950s，含空轮/失败轮心跳计数、白名单meta与健康失败只Warning且原Call成功。与本课CLI10模式5.459s分栏，不重复旧全套或SDK生命周期。
