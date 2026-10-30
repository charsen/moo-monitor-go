# 第13步：真实Host慢SQL消费边界

第12步已提交2eea063，保留当时验证与类型范围。本步对应Host43生产启动接线暴露的真实SDK消费需求，SDK与Host分别验收/提交；先SDK再Host，不把本机workspace当已发布依赖。

## SDK精确范围

新增包级adapters/goravel.HTTPRequestSnapshot(ctx)，委托现copiedHTTPRequest单一实现，返回当前生命周期的复制标量/late identity。非HTTP/closed明确exists=false，无原Request/全局猜测，不造setter/第二份请求缓存。Host将其映射为SQL Request五键，不加Guard协议字段。

adapters/gorm bindings扩充实际原生整型宽度与有限float32；uint/uint64超过MaxInt64拒采。不反射任意alias，不调用Valuer/Stringer。真实Food Store201但目标INSERT未采的类型探针进一步核出framework carbon.DateTime（实际泛型LayoutType别名）及gorm.DeletedAt。只读锁定实现的Carbon状态/StdTime与DeletedAt.Valid/Time，映射nil/time.Time；Carbon错误从严拒采，不放开所有Valuer。源码Value本身的空/错顺序不能被概括为任意坏对象也可自动适配。

真实消费四层：①private bindings/SQL插件与copiedHTTPRequest；②Host新slowRequest/Slow.Attach与真实Food WithContext；③实际HTTP middleware私有state/late typed Subject，以及Preload Provider/Runtime组合；④新增定向、旧Writer/HTTP与六callback必要消费者和本步教学。没有删除公开契约，不用额外全仓删除符号审计替代真实调用核查。

## Host43生产采用与重要基线

业务DB_PRELOAD_CONNECTIONS默认[]，严格唯一已知名字、非空默认必须首项，独立于monitor。实际单线程Boot协调原ORM query后才按slow allowlist Attach；不复制ORM、造平行池/自动发现全部连接。源码发现原DatabaseProvider.Boot注册命令会MakeSchema/MakeSeeder，已配置默认DB时原生会初始化；按root/design选A，监控开关不新增/预取连接，不假称整个进程零DB。未知名单仅在本新增预热动作前拒绝，不改变原框架默认行为。

Host Slow/Runtime停止新采集与资源完全关闭分开：插件等待取消/超时保可识别原因，SlowBuffer不提前释放，可重试排空后关闭。Owner不关业务池、不flush网络。Slow Builder使用实际app root作BasePath，SDK仍是唯一相对路径/hash规则；开发验证不证明所有trimpath部署构建相同。

## 实际验证（2026-10-30）

SDK单module，GOWORK=off，实际Go1.27.3、最低1.26，无manifest/sum变化。新snapshot+旧late identity/Writer早轮PASS0.796s；数值2项早轮0.891s；已知日期加入后binding3项0.517s。真实unknown Valuer SQL单项首轮1.544s，随后简化复用同一私有fixture；最终统一6项GORM race PASS6.719s覆盖binding/真实Valuer/原六callback/16并发，4项Goravel race PASS2.345s覆盖snapshot/late identity/Writer/HTTP并发。

Host workspace证据分栏：实际Authentication+Food Store/Update/Show分别目标INSERT/UPDATE/SELECT及身份/连接，active/capture-failure两模式PASS2.071s；发现原生默认DB后修正10负例+旧41stack组合PASS5.547s；真实flock在途采集取消/保Buffer/重试单模式race PASS7.606s。最后BasePath仅受影响active必要复验PASS1.769s，断目标SQL当次方法/身份/连接及app/food/...相对File；不能把前race说成此后重跑。

首次未用import/错误payload字段名、Store201但INSERT未采、原生默认DB物理文件负例失败都保留为修正证据，没有吞失败或用SELECT数量替代写链断言。测试仅私有SQLite/临时Buffer/受控日志，未真实Cloud/生产DB/全量/E2E。

## 未完成范围

DB façade、其它驱动/占位符、未安装/Fresh后实例、Rows迭代、真实Cloud字段接收、发布解析与完整M矩阵仍待；自动HTTP payload/queue/schedule不是本步内容。用户练习待其完成。本SDK提交不链接尚未进HEAD的Host43文件，Host同批提交后统一核引用。
