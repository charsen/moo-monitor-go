# 第10课：SDK与Host本地资源所有权

[上一课：09](09-http-capture.md) · [课程目录](../README.md)

完整接线教学在[Host第41课](../../../go-moo-engine-skeleton/docs/lessons/41-monitor-runtime.md)，本课只记录SDK边界。计划见[第10步](../plans/10-host-runtime.md)。

Host默认关闭，不创建本SDK资源；启用时由Host Provider明确拥有Buffer、Collector、日志与HTTP adapter，退出只关闭本地。原native日志通道与GinLogSafety保留，SDK不import Host/scaffold，不读取全局环境，也不因本地采集启用就推Cloud。

真实认证发生在请求全局middleware之后，因此Identity现在接受context.Context，并在日志首次采集/最终5xx事件时懒解析。HTTP state保存owner配置函数和基础method/url/ip，不保存原Request或捕获它的闭包。读取当前typed身份后复制ID/Name/Guard标量，退出清除；匿名保持nil。resolver故障只安全诊断，仍使用基础request，不影响原响应。

这是Host消费者暴露的09后续修正，不能把09原来的beforeNext样本当成新模型的证据。SDK独立GOWORK=off的14项受影响HTTP/Writer race实际PASS3.072s；Host的workspace/bootstrap/认证middleware与私有sink证据见41计划。前者不是Host接入测试，后者不是SDK独立发布解析。

Host本地workspace加入monitor不等于已发布require；当前GOWORK=off部署仍待解析验收。自动payload、queue最终失败/schedule退出、慢SQL及真实Cloud字段兼容仍是后续工作。

## 自主练习（待用户完成）

1. 说明为何beforeNext取身份会丢认证信息，为什么不能保存原Request到后台再补。
2. 区分Host owner、SDK Buffer、EventScope及Cloud Sync的关闭与网络权限。
3. 对比SDK单module和Host workspace两类验证，指出各自未证明什么。

代理测试不代做学习验收。
