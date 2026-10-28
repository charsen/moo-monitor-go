# 第05步：本地运行时缓冲、三桶与恢复

本步在Monitor独立module内实现buffer，消费04的records.Builder；只显式记录/Get/List/Count，不自动采集、不连接Cloud。当前PHP已移除本地resolve/delete/restore/purge，生命周期权限属于Cloud；测试只在专属目录种已存在resolved/deleted记录来验证复发，不复活已删除的本地API。

## 方案与依赖

Config显式Root/Scope/MaxOpen/DailyCap，Root必须预先创建且私有，SDK不猜业务目录或argv/env。Root下go-runtime-buffer[--scope]隔离记录、锁、overflow与journal；scope按源ASCII小写、非字母数字连续串变-、trim后前64字节。Record/Get/List/Count显式context控制等待文件锁；Close由owner负责，不向Host安装默认绑定。

依赖选择已核官方[flock v0.13.1](https://github.com/gofrs/flock/releases/tag/v0.13.1)（module最低Go1.25.0）与维护方[yaml v3.0.5](https://github.com/yaml/go-yaml/releases/tag/v3.0.5)（Go1.16）；v4仍rc不选。当前module基线1.26.0，实际工具链1.27.3；已真实get/tidy并审mod/sum，x/sys0.49.0直接用于NOFOLLOW，testify仅上游测试图；flock manifest的0.47只是minimum，本次直接unix依赖实际核到稳定0.49.0/Go1.26并固定，不声称0.47此路径存在可利用漏洞。文件锁使用成熟库，Darwin/Linux专用flag；当前仅Darwin实际执行，Linux/Windows/网络FS不作已测承诺。

## 主链与最终图像

Record直接展开：加scope独占锁 → 恢复已持久intent → 按open/resolved/deleted顺序读匹配记录 → 新建max_open/复发重开/同日cap业务判断 → 写固定intent → 写目标记录 → 清其余旧桶 → 设置最终overflow → 清intent。Get/List/Count同锁先恢复，避免看到中途状态；Get/List/Record读坏记录/错误hash/IO错返回error并保原文件，Count按源只数合法文件名不parse正文，不当不存在覆盖。

open在cap内刷新，冻结时原YAML/时间/count不变，overflow持久+1；跨日或resolved/deleted复发时回填count但不加daily，重开清resolved。新建受max_open，复发绕过此容量闸。source热路径无锁、overflow在cache可过期/丢失；Go以scope串行和持久overflow增强恢复能力，明确差异，不伪称逐行复刻。

同一实例以可取消gate保护root生命周期，ctx在gate排队与flock等待两层均生效。结果未知错误保ErrOutcomeUnknown、阶段与安全cause，errors.Is可识别原因为何；不dump正文。

每个intent只存version、固定12hex hash、已知目标桶、最终Record/overflow，不接任意路径或泛化操作；必填字段逐项要求存在且不能用null默认数值，version/overflow须!!int、hash/target须!!str、record只允许mapping或显式null。intent完整写+sync后才改图像；失败可返回结果未知，后续恢复重放相同最终值，不重新计算count。目标已写而旧桶未删时中断，下次在同scope锁下重写同目标、清旧桶、设同overflow，再清intent，因此不会因恢复多加一次。外部调用者无事件ID重复提交仍可能多计，不声称exactly-once。

## 文件与表示安全

根由调用方预先创建，scope0700、文件0600，固定已知文件/桶与12hex校验；os.Root约束相对I/O，检查符号链接，flock以NOFOLLOW打开已创建锁文件并核inode。私有目录内合作进程并发是当前模型，不承诺阻止有权限修改目录的同UID对手。完整写入、同目录临时文件、file sync、rename、目录sync；新建scope/桶也sync其父目录按当前平台实际返回处理；不将“原子rename”等同所有硬件断电保证。

YAML codec不经JSON转换：显式yaml字段标签和Node适配保invalidUTF8二进制字符串、OrderedObject顺序及数组marker。未知字段明确拒绝，保原文件，不静默吞掉未来扩展；当前只接本SDK独立Go缓冲，不导入PHP目录；PHP版本Context不伪改，fixture对照时明确投影为测试GoContext。慢SQL未来复用存储另设计，不放Food/Host。

## 验证与源probe

只新buffer定向：t.TempDir ownership，新建/重复、cap2五次次日count6、两桶复发、容量并发、scope隔离、list/count、坏YAML、无效hash/链接、短写/rename/删除故障、各提交阶段journal中断后恢复、跨进程同目录不丢计、YAML字节/有序payload往返。实际并发可仅本包定向race一次，不扩全量/E2E/Cloud。

2026-10-29真实PHP probe已执行：PHP8.3.35/Symfony YAML7.4.18、五声明/受限PSR4/内存FakeCache、自有0700 temp finally清理gone；raw fixture5步count/daily为1/1、2/2、6/1、7/2、8/3，capacity拒新null，记录/rows除Context明确平台替换外语义相等。原stdout保留并可重放。

Go1.27.3/GOWORK=off：11项普通定向PASS3.667s；新增实际子进程target-durable直接exit17，复原后count2单桶，单项PASS0.902s；含该项12项buffer定向race PASS6.513s。后续回读发现新建scope父目录需同步，补parent Sync后仅目录初始化单项PASS0.846s；当时未重复并发race。错误返回式恢复覆盖intent-durable/target-durable/old-removed/overflow-durable/clear-intent与冻结overflow，IO真实边界注入短写/rename/删除/目录sync，断言恢复最终记录与overflow不双计。不是kill或断电测试，不承诺设备耐久实验。

首轮测试权限未设0700已纠正测试；journal *yaml.Node被KnownFields当普通struct，改value Node；源fixture比对发现假快照code空串，按真实PHP默认0修输入。gate/cause经设计审补齐。最后journal补全presence/null/kind校验并新增缺字段及overflow:null/version:null/record错误类型保原图像测试；新直接x/sys固定0.49.0后，受影响buffer十项普通定向最终PASS3.831s，同范围新图race PASS6.448s。旧6.513s仅早轮证据；未跑govulncheck。

仅新增buffer定向，records仅加yaml标签，不改Builder判定；不扩旧包回归、全量/E2E或Host/Cloud。

用户练习待完成，详见[第05课](../lessons/05-runtime-buffer.md)。
