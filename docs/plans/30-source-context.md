# 30 有界源码上下文

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

默认关闭，显式应用源码Root与相对AllowedPrefixes，仅读取允许的普通UTF8 Go文件；不把异常File当任意文件读取入口。不改变原Exception.File/Line、Trace.Full或hash；旧PHP导入snippet不经过本采集扩展。

Reader独立拥有os.Root，Darwin/Linux逐目录锚点、SameFile与最终NOFOLLOW/NONBLOCK，拒绝任何下层symlink、隐藏/凭据路径、越界和非.go。根路径自身禁止symlink；其系统父目录别名解析后固定canonical锚点。不强制源码权限0700，0755目录/0644源码可读。其它平台明确配置不可用，不假称跨平台已验。

输入最多扫描64帧，按序最多20个允许帧，相对路径投影。首允许帧提供片段，默认±10行(最多100)、文件1MiB(最多16MiB)、片段16KiB(最多64KiB)。有界分块读取、前后锚点与文件size/mtime核查，拒UTF8/NUL/超限。不是文件系统全事务快照；源码应稳定，不能声称阻止任意并发改写。MaskSecrets仅有限键/pattern，不保证识别所有源码秘密。

Capture借SourceReader，在原exception/trace身份确定后附加展示字段。辅助失败或panic返回静态ErrSourceContext，同时仍Record原事件；非空hash可能已保存，scope严格按Record真实结果登记done，不因辅助错误重复计数。Record失败原因仍可errors.Is识别。失败不添加片段或新投影，保留原Frames与Trace.Full；nil不读源码。调用方先处理真实保存结果与辅助诊断，不把所有error视为未落盘重试。

Host58显式Source配置及owner关闭，合法根运行时不可打开保持核心Collector、静态source_initialization；配置语法无效沿现ErrConfiguration。只同步Builder.AppFramePrefixes，不设置BasePath或改hash。源码Reader在停止采集的Runtime收尾关闭，Close等待当前Read；调用方仍须按owner顺序停止业务采集，不声称任意第三方并发关闭安全。

## 实际证据

2026-11-02，本机Go1.27.3/Darwin。Reader首测0.363s，64输入与Capture接线普通Reader0.732/Capture0.853。必要race Reader1.271s、Capture源码场景1.419s；旧CaptureScope单项race2.124s。首个race正则误用精确Reader/SourceContext名称，Reader输出no tests，不作为新项通过证据；随后正确正则执行新项。

Host纯Runtime五模式首轮因fixture缓冲根未chmod0700而失败；修fixture后0.720s，最终race1.715s。真实bootstrap启用/default-off/不可用三模式race5.422s，父子自有空env与固定假配置，未读真实.env、未联网。没有实际部署/其它OS/权限故障注入或并发恶意路径替换压力验证，不扩全量/E2E。

终读修正：辅助读取失败保留原显式相对Frames，而非清空；只有Read成功才替换展示。必要同Capture单项最终0.815s，新增相对app帧保留断言；上述race为此修正前证据，不重复未影响的Reader/Host范围。
