# 第21课：推送前只看本地候选

[上一课：20](20-local-noise-discard.md) · [课程目录](../README.md)

`--dry-run`回答“按当前落盘版本，正常增量会选中多少条”，不回答Cloud是否接受。它不需要URL/token。具体契约与实际结果见[计划](../plans/21-push-preview.md)。

## 文件职责与调用链

`cloudsync/preview.go`定义SelectionMode、PreviewOptions及typed预览结果。StatusReader.Preview → Stats → type gate/flock → 严格ledger/suppression → ReadOnlySnapshot → 完整digest/time/bucket分类。Syncer/Pusher也消费同一Stats路径；不复制选取规则，不造假Client。

Known时Selected/WouldSend为PendingOpen+PendingResolved；已确认和精确抑制分别显示排除数。Unknown时指针缺席，Busy、坏账本、未决journal、缺源不能显示成成功0。快照只代表读取时刻，下一次真正发送前记录仍可能变化。

## 只读边界

Preview不请求HTTP、不调用AfterPush/heartbeat，不写ACK或suppression、不剪账本、不推进cursor、不恢复journal。构造reader可能建锁，既有Buffer初始化可能恢复，须与Preview本身分开。模式当前只接受Incremental，未实现All不要传任意字符串当默认。

## 练习与实证

自主练习：同毫秒增加一次累计count，观察旧ACK为何失效；解释已抑制版本与已确认版本差异；制造未决journal说明为何应先恢复而非预览修复。练习待用户完成。

2026-11-02，SDK首测0.883s；最终Preview+现Stats race2.098s，后续Sync的实际发送选择与预览一致且预览文件字节不变。Host五模式race7.637s，详见计划。没有真实Cloud或全量/E2E；完整All/retention能力未由本课交付。
