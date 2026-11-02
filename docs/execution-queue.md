# 持续执行队列

2026-11-02，用户已撤销“两项后暂停”，当前按已设计功能持续推进；每项设计验收后独立commit/HEAD回读，不自动push。历史课程的暂停是当时安排，不是当前门槛。规则仍以AGENTS为准，本文件只记阶段状态。

| 现成 | 待设计 | 待实现 | 需外部授权验证 |
| --- | --- | --- | --- |
| 01–24：本地采集/缓冲/确认同步、Owner、健康、统计/抑制、dry-run与All参考闭环；各自限制见课程 | 其余monitor矩阵逐块设计（summary/配置兼容、采集覆盖与部署） | 25纯转换/离线Plan已定向通过，typed导入/Apply/Host55实现中；保源、cursor不转ACK | 真实Cloud权限/平台兼容、生产数据与迁移、E2E、发布推送及目标部署解析 |
| 26已955ba3e、27已d157f9f、28独立Host入口已本地验证；不等于真实MCP接入 | 其余monitor矩阵与Scaffold一期后续深化 | 28隔离Host MCP入口已本地定向验证；29过滤/payload待实现，25导入继续 | 当前不阻塞本地受控实现；对应动作仍须明确授权 |

## 顺序与接续

23确认resolved维护 → 24只读迁移体检 → 25受控格式转换/幂等导入 → MCP协议loop → Cloud工具/分页/私有图片 → Host显式MCP采用。之后逐块核monitor余矩阵：summary/配置兼容、payload过滤/采集覆盖、旧env/生命周期/部署。不能把列入队列当交付。

monitor可用依赖齐后，转Scaffold一期五模块（数据库设计、安全接口调试、路由、S-Cloud、设置四区），再完善skeleton初始化/发布解析/多业务交付。范围依据[复现计划](port-plan.md)和Scaffold[一期计划](../../go-moo-scaffold/docs/phase-1-plan.md)。不预先执行生产迁移、真实Cloud写或E2E。

实际Cloud仅用户授权的心跳+runtime/slow逐项saved已由root验证，见Host[接入记录](../../go-moo-engine-skeleton/docs/integration/cloud-connection.md)；不扩为MCP或全面字段已验。
