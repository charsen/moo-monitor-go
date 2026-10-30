# 第17课：显式推送与周期Owner

[上一课：16](16-host-schedule.md) · [课程目录](../README.md) · [下一课：18](18-health-check.md)

本课将已有HTTP/ACK内核交给真实消费者，未新建同步账本。范围见[计划](../plans/17-cloud-push-owner.md)，Host对应第47课。

[协调器](../../cloudsync/pusher.go)持已有runtime/slow Buffer与HTTP客户端。Push指定both/runtimes/slow_sql；手动选中已启用但缺失的数据源报错，周期Tick给source_unavailable；push开关false按源给disabled。每类失败继续另一类，TypePushResult保原Scanned/Confirmed/Pending/Rejected/Busy，并有独立Failed，不能因Pending=0就写complete。

Syncer原有每类gate与scope flock继续覆盖整个同步，手动与周期/另一实例相遇返回Busy，不抢写ACK。Push不另加整个round互斥，runtime与slow账本仍分开。部分确认只重试余项，空轮成功且不发intake。

Close先Stopping、取消在途context、等待全部调用退出，再关两Syncer的目录句柄。取消等待不等于Closed，允许重试；借用Buffer/HTTP由外层拥有者管理。Host自动owner150在本地Runtime200前收尾，首tick前没有请求，不依赖业务Schedule开关。

第一块AfterPush默认nil，无假心跳。接口只留一个可选真实健康操作，使用caller+lifetime的round父context，intake独有timeout；因此intake超时不预先取消独立健康尝试，关闭仍取消整个round。第二块才接实际HTTP健康，其错误不改变推送确认结果。

源[CloudPushCommand](../../../moo-monitor-laravel/src/Command/CloudPushCommand.php)逐类sync，成功后retention prune，非dry轮尾heartbeat。当前只实现正常增量：--all/--dry-run/retention明确拒绝，现SDK Confirm resolved立即删除与源retention不同，完整M13仍待。不把本地httptest说成真实Cloud兼容或生产交付。

2026-10-30实际Go1.27.3，GOWORK=off：四个TestPusher最终race PASS1.657s，覆盖两类partial/继续、空轮、独立健康deadline、缺源/disabled、同实例及跨实例Busy、Close取消重试与不关借Buffer。Host配置/owner race1.645、真实六模式race9.873s。候选独立提交闭包与最新CLI负例见Host47计划。无全量/E2E/真实Cloud写。

自主练习：解释ACK与cursor的区别，画出partial失败仍推另一类的顺序；说明Busy不是上传成功，为什么Close不能先关Buffer；区分intake timeout与round取消。用户学习仍待完成。
