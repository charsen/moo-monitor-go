# 持续执行队列

2026-11-02，用户已撤销“两项后暂停”，当前按已设计功能持续推进；每项设计验收后独立commit/HEAD回读，不自动push。历史课程的暂停是当时安排，不是当前门槛。规则仍以AGENTS为准，本文件只记阶段状态。

| 功能 | 单一状态 | 下一动作 | 外部待验 |
| --- | --- | --- | --- |
| 01–24本地采集、缓冲、确认同步与运维 | 已交付参考实现 | 按矩阵继续剩余能力 | 生产数据、迁移与部署 |
| 25受控旧记录转换/幂等导入 | 已交付 | 保源与冷scope限制见课程 | 实际legacy字段与Cloud累计兼容 |
| 26 MCP协议循环 | 已交付 | 消费27/28边界 | 真实MCP协议接入 |
| 27七个Cloud工具 | 已交付 | 真实授权后逐能力验证 | MCP权限、分页、私有图片与状态操作 |
| 28独立Host MCP入口 | 已交付 | 保独立env/stdout/输入所有权 | 目标部署解析与真实MCP |
| 29过滤与HTTP payload | 已交付 | 保默认关闭与有限预算 | 实际业务采集覆盖 |
| 30源码上下文 | 实现中 | Reader已首测，接Capture与Host58 | 目标源码布局与部署权限 |
| 31Postgres慢SQL | 已交付参考实现 | 真实连接验证须另授权 | 实际PG、Host连接与部署 |
| monitor余矩阵 | 待设计 | summary/配置兼容、采集覆盖与生命周期逐块核实 | 按具体外部副作用授权 |
| Scaffold一期五模块与skeleton交付 | 待实现 | monitor依赖齐后按一期计划推进 | 发布解析、生产迁移、E2E与部署 |

## 顺序与接续

23确认resolved维护 → 24只读迁移体检 → 25受控格式转换/幂等导入 → MCP协议loop → Cloud工具/分页/私有图片 → Host显式MCP采用。之后逐块核monitor余矩阵：summary/配置兼容、payload过滤/采集覆盖、旧env/生命周期/部署。不能把列入队列当交付。

monitor可用依赖齐后，转Scaffold一期五模块（数据库设计、安全接口调试、路由、S-Cloud、设置四区），再完善skeleton初始化/发布解析/多业务交付。范围依据[复现计划](port-plan.md)和Scaffold[一期计划](../../go-moo-scaffold/docs/phase-1-plan.md)。不预先执行生产迁移、真实Cloud写或E2E。

实际Cloud仅用户授权的心跳+runtime/slow逐项saved已由root验证，见Host[接入记录](../../go-moo-engine-skeleton/docs/integration/cloud-connection.md)；不扩为MCP或全面字段已验。
