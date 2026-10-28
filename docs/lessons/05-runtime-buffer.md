# 第05课：运行时缓冲、三桶与可恢复提交

[上一课：04](04-runtime-records.md) · [课程目录](../README.md) · [下一课：06](06-incremental-sync.md)

04已经能从显式Snapshot构建Record，本课让记录在独立本地目录中累计、冻结和恢复。主链仍由调用者显式触发，不自动采集请求，不向Cloud发送任何内容。范围见[本步计划](../plans/05-runtime-buffer.md)。

## 先纠正当前源的生命周期边界

PHP [BucketedYamlRecorder](../../../moo-monitor-laravel/src/Recorder/BucketedYamlRecorder.php)保留open/resolved/deleted三桶，但当前公开本地动作只有记录、get/list/count；旧resolve/reopen/delete/restore/purge已移除，处置交给Cloud。不能看到桶名便重新造一套本地管理接口。

本课只提供Record、Get、List、Count与owner调用的Close。测试在自有目录写入固定resolved/deleted样本，验证后续复发自动回open；这只是测试种数据，不是生产Resolve/Delete API。Cloud回执如何应用本地状态，后续按真实消费者设计。

桶目录是状态真值，查找顺序open→resolved→deleted；读取返回的Status以桶覆盖。为避免吞掉坏旧文件，Go在同hash三桶中都检查内容，低优先桶损坏也返回error。源find遇坏YAML可能当不存在；本课拒绝这种覆盖风险。

## 一个独立SDK怎样打开缓冲

在本仓独立使用，不加入Host workspace：

```go
builder, err := records.New(records.Config{MaskKeys: []string{"password", "token"}})
if err != nil {
    return err
}
cap, maxOpen := 2, 500
store, err := buffer.New(buffer.Config{
    Root: privateRoot,
    Scope: "example-testing",
    DailyCap: &cap,
    MaxOpen: &maxOpen,
}, builder)
if err != nil {
    return err
}
defer store.Close()
hash, err := store.Record(ctx, snapshot, now)
```

Root必须由调用方预先创建，绝对路径、目录本身不是symlink、权限不向group/other开放；例如使用os.MkdirTemp并显式os.Chmod(root,0700)，检查每一步error。SDK不猜Laravel storage路径，也不自动收集环境变量或argv。Root下独立stem为go-runtime-buffer[--scope]，不会直接使用源runtimes；Root若已有open/resolved/deleted会拒绝，防止误指PHP桶根。

Scope是明确字符串。沿源normalize的ASCII小写、非字母数字连续串变-、去两端-、最多64字节；空规范结果不加后缀。不自动解释PHP auto/--env选择器；不同原串可能规范成同slug，调用方须使用明确且唯一的scope。记录、锁、overflow、intent都在同一个scope目录，不能只把记录路径隔离而共用计数/锁。

MaxOpen与DailyCap为可选指针，nil取500/10；显式DailyCap<=0不限制每日写入，MaxOpen<=0按源拒绝新建，并非“无限容量”。已有记录复发绕过新建容量闸。

文件锁使用[flock v0.13.1](https://github.com/gofrs/flock/releases/tag/v0.13.1)，YAML使用维护方[yaml/v3 v3.0.5](https://github.com/yaml/go-yaml/releases/tag/v3.0.5)；v4当前仍rc，未选预发。真实get/tidy后锁定go.mod/sum；x/sys0.49.0用于Darwin/Linux NOFOLLOW：flock manifest的0.47只是minimum，收口实际核到0.49.0/Go1.26，直接依赖据此固定；不声称0.47此路径有可利用漏洞，未跑govulncheck。Go下限仍1.26.0，实际验证1.27.3。sum中的testify是flock上游测试图，不是SDK新增测试框架。

## 文件职责与主调用链

| 文件 | 职责 |
| --- | --- |
| [buffer.go](../../buffer/buffer.go) | API、scope配置、可取消gate/文件锁、Record业务顺序 |
| [files.go](../../buffer/files.go) | rooted路径、完整原子写、目录sync、扫描与列表 |
| [codec.go](../../buffer/codec.go) | typed Record/YAML、payload Node与overflow编码 |
| [recovery.go](../../buffer/recovery.go) | 固定intent格式与相同最终图像重放 |
| [lock_unix.go](../../buffer/lock_unix.go) | Darwin/Linux成熟锁库打开flag |
| [测试](../../buffer/buffer_test.go) | 临时目录、故障边界与进程样本 |

主Record直接展开：可取消gate→scope独占文件锁→恢复intent→读旧记录/overflow→新建或cap/复发判断→写intent→写目标→清旧桶→设置overflow→清intent。没有通用Pipeline、回调注册器或跨业务事务框架。

gate保护同一Buffer实例及Close生命周期，排队也可按ctx取消；scope文件锁串行不同实例/进程。只有文件锁而先用sync.Mutex排队，会让第二个同实例调用进不到TryLockContext，无法及时遵守deadline；定向测试让首调用停在intent持有阶段，证明第二调用短deadline可以先退出。

Get/List/Count也持同scope锁并先恢复，所以不会把恢复中短暂双桶状态作为正常结果返回。Close由owner在使用结束时调用，可等待当前操作退出。这里没有后台goroutine或自动打开Host目录。

## 新建、冻结与次日回填

无旧记录时先数open合法文件，未达MaxOpen才调用04 Builder.Build，count/daily各1；已有open则检查当日daily。

DailyCap=2时，同hash前两次写入count2/daily2；第3、4、5次只增加持久overflow3，原record YAML逐字不变，last_seen和meta.updated_at也不变。次日首次事件交Builder.Refresh回填overflow，count=2+1+3=6，daily只记本次写入为1，overflow清0。

resolved/deleted复发：清resolved_at/by/note、设open、回填overflow、刷新，绕过新建容量闸，并保证成功后同hash只留open。源是在refresh前搬旧文件且未检查move返回；本实现用恢复intent提交最终图像，避免搬失败后悄悄产生重复。

Go overflow是同scope私有文件，不使用有TTL的外部cache，也不在次日之前自动过期。源cache不可用/清空/TTL到期会丢回填；本步以持久overflow增强恢复，是明确设计差异。它仍受int64范围限制，耗尽返回error，不静默回绕。

## 用一个中断例子理解journal

假设deleted里count=1的记录复发，最终应变成open count=2、resolved字段清空、overflow0。intent.yaml只保存这个最终值和固定hash/target，不保存“再加一次”的命令，也不能指定任意路径。

```text
1. intent完整写入、sync、rename、目录sync
2. open/<hash>.yaml写好并sync
3. 删除resolved/deleted旧文件
4. 设置最终overflow
5. 删除intent并sync目录
```

如果第2步后进程退出，磁盘可能同时存在open新记录、deleted旧记录和intent。下一次同scope操作先持锁恢复：把同一个open count=2再写一遍、清旧桶、设置同一个overflow0，最后清intent。不会调用Build/Refresh，也不会把count改成3。

本课真实子进程在target-durable执行os.Exit(17)，跳过defer退出；父进程先确认双桶+intent，然后重开恢复为一桶count2，第二次读取仍2。这是进程骤退恢复证据。其他每个阶段以故障返回error后Close/New恢复；没有做真实断电、设备缓存或网络文件系统实验。

写intent之后的失败返回ErrOutcomeUnknown并保留安全cause/阶段；errors.Is既能判断结果未知，也能识别底层注入原因。不要把它当作“肯定没提交”直接重试同事件。后续先恢复/读取状态；由于没有事件ID，外部重复提交仍可能多计，这是at-least-once边界，不是exactly-once。

intent逐项校验version/hash/target/record/overflow存在：整数必须!!int而非null，hash/target必须!!str，record只能mapping或显式null。不能让缺失或overflow:null被Go整数默认0悄悄清计数；定向损坏测试保原intent/record/overflow。

坏intent、坏记录、错误hash或IO失败会阻断操作并保原文件。恢复也不因为有intent便覆盖损坏文件。当前不自动修坏文件、不删除未知条目，暂留的原子写临时文件不作为记录扫描；后续需要有界维护策略。

## YAML必须保留字节与payload形状

[records.go](../../records/records.go)只补与JSON键一致的yaml标签，Builder逻辑不变。codec直接操作yaml.Node，不经json.Marshal中转；否则04可能按字节截断的trace会被JSON替换非法UTF8。

定向往返验证非法UTF8 trace编码为!!binary并读回相同字节、有序payload保z→a顺序、5001项数组预算截断为数字key+marker对象、空数组仍[]。OrderedObject不是Entry数组直接落盘，而是明确的mapping Node；数值读回int64，避免再次交Builder时变成不支持的普通int。

只接本SDK独立Go记录，未知字段拒绝并保原文件；不直接导入PHP目录。GoContext没有假php/laravel字段，原PHP YAML样本不是已兼容迁移格式。未来慢SQL、扩展字段、历史格式和真实Cloud接收要分别设计，不为了方便读写把未知字段静默丢掉。

Get/List读坏YAML返回error；Count按源只数合法文件名、不parse正文，所以损坏正文仍计数。List按last_seen字符串降序；limit0为空，负值沿PHP array_slice排除尾部。列表状态来自桶，message_first_line沿源strtok/trim与160字节截断，不用它当完整异常内容。

## 文件安全与当前耐久边界

Root由调用方选定，scope目录0700、文件0600。固定桶/12hex文件名、os.Root相对I/O、路径Lstat拒link、锁文件NOFOLLOW和持锁后inode复核共同限制越界。测试覆盖record/bucket/lock symlink与非法hash。不是针对能修改私有目录的同UID对手的完整安全边界。

每次原子写检查完整字节数，file.Sync后同目录rename，再sync受影响目录；删除也sync目录；后续回读补上新建scope/桶的父目录Sync，调用者自己新建Root的父目录耐久仍由owner负责。注入真实短写、rename、删除、目录sync失败，验证旧记录或intent保留且恢复最终图像。并非所有fsync返回成功都等价所有硬件断电持久；当前仅Darwin本机实际运行，Linux同实现尚未运行，Windows和网络FS不承诺。

SDK错误只带阶段、固定文件操作与安全cause，不dump payload/YAML/journal。它会向调用方返回error；如何让真实Host继续原业务并保留有界诊断，仍属于后续M18接线，不因本课没有网络调用就宣称全链隔离完成。

## 原始PHP证据与实际验证

[受控probe](../../tools/probe-runtime-buffer.php)真实调用源protected persistAggregated及原private build/refresh、read/write/list/count；不调用record/shouldReport/lean，也不bootstrap Host。显式config/假路径/固定clock，五个源声明含StorageScope；Symfony YAML只允许审核目录内合法PSR4类，拒其他autoload。仅内存FakeCache，无真实Redis/DB/Cloud。

temp由Python tempfile.mkdtemp专有前缀创建并设0700，script拒非空/link/与源目录重叠，随机.owner确认ownership；初始化也包进finally。清理先验证全树白名单，未知条目保留报错；.owner最后删除。实际PHP8.3.35、Symfony YAML7.4.18，已生成[原始fixture](../contracts/fixtures/runtime-buffer.json)，源commit28d424e，实际temp已gone。fixture全是假值，保5步完整记录/列表、new/frozen YAML和YAML加载文件SHA。

重放时先审脚本再创建自有temp：

```sh
php -l tools/probe-runtime-buffer.php
python3 - <<'PY'
import os, subprocess, tempfile
root = tempfile.mkdtemp(prefix='go-moo-php-buffer-')
os.chmod(root, 0o700)
subprocess.run(['php', 'tools/probe-runtime-buffer.php',
    '../moo-monitor-laravel/src/Recorder/RuntimeErrorRecorder.php',
    '../moo-engine-skeleton/engine/vendor/symfony/yaml', root], check=True)
assert not os.path.exists(root)
PY
```

Go对fixture逐字段比较，只有来源Context中的php/laravel明确替换为测试提供的go/goravel；原stdout不改，不声称字节级PHP YAML互通。5步count/daily为1/1、2/2、6/1、7/2、8/3，capacity拒新null。

2026-10-29实际验证：早轮11项普通定向3.667s、进程骤退单项0.902s、当时12项race6.513s；随后父目录Sync初始化单项0.846s。最后补journal必填/类型检查、固定直接x/sys0.49.0后，只跑受影响buffer十项普通定向通过（3.831s），同范围新图race通过（6.448s）。其余规则未改，旧图数字不混作最终图race结果。命令仅本包：

```sh
GOWORK=off GOTOOLCHAIN=local "$HOME/Library/Caches/go-moo/toolchains/go1.27.3/bin/go" test -mod=readonly -race ./buffer -run '^TestBuffer(Recovery|IOFailures|CorruptionAndPaths|Concurrent|Process|CancellationAndCause|FailureCause|ProcessInterruption|IncompleteIntent|IntentTypes)$' -count=1
```

初轮失败与修复：测试Root显式设0700以遵守生产门；journal的*yaml.Node被KnownFields当普通struct，改value Node；Go假快照code空串与PHP默认"0"不同，修真实测试输入后再对源。设计审查补同实例可取消gate与错误cause，修正后按实际受影响范围执行对应定向/race。未跑全量/E2E、真实Host/Gin/Cloud、Linux或Go1.26最低版。

## 用户练习（待完成）

画出cap2五次到次日的record/overflow变化，解释daily为何不加overflow。再手推“目标已写、旧桶未删”的中断状态，以及恢复为何不能重新调用Refresh。

自行说明同scope文件锁与单实例gate各解决什么问题；比较ErrFull、损坏文件错误与ErrOutcomeUnknown的调用方处理。解释为什么当前没有本地Resolve/Delete、为什么PHP原目录不能直接交给这个Go缓冲。代理参考测试通过不代做这些练习。
