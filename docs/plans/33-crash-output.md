# 33 显式Go崩溃诊断所有权

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

M05第一阶段：私有raw spool及只读脱敏诊断，不是异常记录导入或fatal全部覆盖。使用锁定Go公开runtime/debug.SetCrashOutput，不改Recover、不在fatal路径跑Builder/mask/网络。只有显式Install才占进程全局slot，所有真实hook测试均隔离子进程。

## 接口与存储边界

crashoutput.New(Config{Root,MaxReadBytes,MaskKeys})只打开显式已存在的绝对0700私有根；不创建文件或安装hook。Install创建随机O_EXCL新0600文件，并由runtime复制fd。本包mutex仅防本包多个owner；Go没有getter，不能识别第三方先前/后来的sink。启用即由Host独占该全局slot，禁止与其它SetCrashOutput使用者并行；Close不恢复第三方旧sink。

Owner.Close先SetCrashOutput(nil)解绑，再关自己的文件，不关Manager诊断根；Manager.Close只关闭自己active owner再关根。均幂等，不关闭其它Manager的owner。原stderr永保，并发崩溃可能在解绑后仍向旧sink写部分字节，不能承诺关闭瞬间原子切断。

List/Inspect(ctx,id)只认固定随机hex文件名，不接受任意路径。NOFOLLOW/NONBLOCK、普通文件/0600和SameFile/size/mtime检查；读取全量预算内后校验UTF8再MaskSecrets，错误不返回部分原文。默认读1MiB、上限16MiB；List每次目录枚举上限1000项，超出静态ErrLimit，不伪装空成功。Info含Bytes/Digest/OutputPresent/MayBePartial，后者始终true，不把内容完整或退出原因推断成事实。

原raw文件本身未脱敏，留本机私有目录；Inspect只给MaskedText，有限pattern仍可能遗漏秘密。runtime直接写fd，读预算不是写quota；原stderr也仍可能包含敏感输出。文件数量/磁盘容量由操作者管理，不自动删非空/转Buffer/ACK/Cloud。现阶段不把raw合成为已确认异常/count。

## Host与验证

Host59默认MONITOR_CAPTURE_CRASH=false，Monitor总开关关闭也不安装；启用时显式现成私有root。合法配置但IO/Install失败仅安全Status，保原Collector；语法非法沿现监控配置失败，不改业务。普通HTTP/Artisan启动都可能构造Runtime，显式开启后CLI也会建空spool；常规采集停止后Monitor owner收尾Close。

2026-11-02/Go1.27.3/Darwin：纯spool0.774，抽取helper曾actual变量漏声明编译失败，修后0.688；真实四子进程0.392，Inspect只读0.375。新范围race2.495；真实panic原stderr+raw两证据、Close后不再写、recover/Exit不产生报告、ErrBusy、UTF8/限额/权限/链接/raw字节不变均有断言。Host普通四模式1.055，必要配置五子例race1.508、Bootstrap四模式race4.335。全部owned空env/假配置，不读真实.env/Cloudtoken。

未执行SIGKILL/OOM/磁盘满/其它平台/生产部署或全量E2E，不把mock或本机证明扩成全部fatal。下一M05阶段须单独设计显式手动/startup诊断导入Collector/Buffer：隐私、幂等、cap、身份/count，当前不自动实现。

## 当前契约衔接（2026-11-03校准）

34已接显式单ID诊断导入，不自动startup扫描。raw仍私有，永久receipt/Unknown/容量协议另列；无法可靠捕获全部OOM/kill或跨OS退出。 具体当前边界见[总设计](../port-plan.md)，已执行结果见[全盘矩阵](../../../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。上方原始测试、失败与未做事项保留其节点语境，不声称本轮重跑。
