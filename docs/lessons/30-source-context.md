# 第30课：在原异常身份之外附加源码展示

[上一课：29](29-filtered-http-payload.md) · [下一课：31](31-postgres-slow-query.md) · [课程目录](../README.md)

[计划](../plans/30-source-context.md)限定只读取显式允许的应用源码。Source上下文不是新异常定位器：Exception.File/Line与hash仍来自原CallerFrames，不能为了显示相对路径而重建身份，也不能把Builder.BasePath改成开发机根。

## 职责与调用链

sourcecontext/reader.go拥有源码根与预算；open_unix.go使用NOFOLLOW/NONBLOCK，逐段SameFile锚点防止跟随替换后的symlink。每次Read只扫描输入前64帧、最多投影20个允许Go帧，首匹配帧取片段；输出相对路径。只有Root下AllowedPrefixes可访问，隐藏/secret/credential/password/token路径、非Go、链接、坏UTF8/NUL、超限拒绝。普通0755/0644源码合法，buffer的0700规则不搬到源码目录。

默认±10行，文件1MiB、片段16KiB；可在有限上限内配置。读取有分块预算和前后属性核查，仍不承诺恶意同inode并发写入的事务快照；应用源码应稳定。最终片段复用MaskSecrets，有限匹配不等于源码全面保密识别。无允许帧返回空片段，不读取任意Exception.File。

Capture先建立原snapshot身份与完整Trace，再调用可选借用SourceReader。成功只换展示Frames/Snippet；失败或reader panic仅静态ErrSourceContext，保留原基础Frames/Trace且不添加片段，继续Record原事件。返回hash+辅助error可能表示已经落盘；scope按真正Record成功登记done，因此重复相同EventID不再增计。真实Record错误仍被保留，调用者不能按辅助失败重新计一次异常。SourceReader=nil沿旧链完全不读源码。

Host58解析默认关闭的source配置，Runtime持有并关闭reader。合法根暂时不可读会记录source_initialization、保留核心采集；原事件不消失。配置语法错误仍拒绝该监控Runtime，不影响原业务/native日志。SDK直接组装时Builder.AppFramePrefixes要与Reader显示前缀一致；不设置BasePath。采集owner先停用后Close，Reader.Close会等待在途Read。

## 实测、限制与练习

Reader首测0.363s；加扫描预算与Capture真实文件消费者后普通0.732/0.853s。新Reader/Capture race分别1.271/1.419s，旧scope必要单项race2.124s；曾误用精确正则使Reader no tests，后续正确新范围已执行，不能把no tests当验证。

Host58 Runtime五模式修正私有buffer目录权限后0.720s/race1.715s；真实Bootstrap三模式race5.422s。固定假源码/假敏感值、普通权限、拒绝链接/预算/UTF8/关闭/辅助失败仍落盘与scope去重均有断言。没有真实Cloud、E2E、其它系统或目标部署证明。此前31并行先交付，本课提交后补齐29→30→31数字导航。

自主练习待完成：解释为什么不能重写Exception.File；区分hash+ErrSourceContext与真实Record失败；预测第65帧才匹配的结果；说明0755源码与0700缓冲权限为何不同；列出有限mask、前后SameFile检查仍不能保证的事情。

终读修正：辅助读取失败保留原显式相对Frames，而非清空；只有Read成功才替换展示。必要同Capture单项最终0.815s，新增相对app帧保留断言；上述race为此修正前证据，不重复未影响的Reader/Host范围。
