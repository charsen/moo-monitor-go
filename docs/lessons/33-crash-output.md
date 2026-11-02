# 第33课：把崩溃原文与安全事件记录分开

[上一课：32](32-db-facade-observation.md) · [课程目录](../README.md) · [下一课：34](34-crash-diagnostic-import.md)

本课是M05第一阶段。[计划](../plans/33-crash-output.md)只交付显式私有spool与只读诊断，不自动导入Buffer或Cloud；32原并行推进，现已补齐数字导航。

## 真实调用链

crashoutput.New验证并打开已存在的绝对0700根，只准备Manager。Install才创建随机新0600文件并调用Go公开debug.SetCrashOutput；runtime复制fd，原stderr依旧输出。不可恢复panic不经过普通业务Collector，也不能在低层fatal写路径同步跑脱敏/Builder/HTTP。

本包只允许一个active Owner，重复Install返回ErrBusy。这个锁无法约束第三方：Go没有getter，应用必须约定独占全局slot，不与其它SetCrashOutput owner并用。Owner.Close先nil解绑再关自己文件；Manager.Close先收尾自己owner再关诊断根，不恢复旧sink，不删报告。并发崩溃仍可能写旧fd，Close不是崩溃写入事务。

List/Inspect只接受本包固定随机ID，根/文件权限、链接、普通类型和前后锚点都检查。读预算默认1MiB、最多16MiB，List最多枚举1000目录项；超限或坏UTF8不给部分文本。只读结果给Bytes/Digest/OutputPresent与始终true的MayBePartial；有输出不等于完整报告，没输出也不证明正常退出。

Inspect在完整有界读取后调用MaskSecrets给MaskedText；原文件不改。**raw文件本身尚未脱敏**，原stderr也未被全局净化。有限pattern不能识别全部秘密；没有HTTP公开接口或自动上传。runtime写fd无法套本读取上限，不能称有限读预算为磁盘quota，也不伪装已实现自动retention。

## Host采用与已验边界

Host59将开关默认设false；只有显式配置的私有root才安装，IO失败记录静态crash_initialization/crash_installation、核心Collector仍可记录业务事件。启用后普通Artisan/HTTP bootstrap都可能创建空spool，操作者需管理磁盘。Monitor owner在常规采集停止后关闭Manager；SDK本身不改变业务response/logger或Recover。

真实四子进程0.392s证明unhandled panic同时写原stderr与新raw、解绑后panic只留原stderr、recover/显式os.Exit无报告。Inspect边界0.375s，新范围race2.495s。pure helper首0.774s，抽取编译缺变量失败后修正0.688s，未掩盖失败。Host四模式普通1.055s；配置五子例race1.508、真实Bootstrap四模式race4.335s。父进程从未install，所有Hook只在本次owned子进程，自有空-env、固定假配置，不读真实.env，不联网。

未模拟OOM/磁盘满/真实kill，也没有其它平台、最低工具链、生产或E2E证明。下一阶段显式诊断导入须重新设计隐私、幂等、cap与原count；此处不把raw标为已确认异常或ACK。

自主练习待完成：解释Raw与MaskedText为何并存；说明读取预算为何不等于写quota；推断另一个库覆盖slot会发生什么；比较Owner.Close与Manager.Close；解释非空文件和空文件都不能证明的事情。
