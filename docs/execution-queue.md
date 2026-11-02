# 持续执行队列

2026-11-02，用户已撤销“两项后暂停”，当前按已设计功能持续推进；每项设计验收后独立commit/HEAD回读，不自动push。历史课程的暂停是当时安排，不是当前门槛。规则仍以AGENTS为准，本文件只记阶段状态。

| 功能 | 单一状态 | 下一动作 | 外部待验 |
| --- | --- | --- | --- |
| 01–24本地采集、缓冲、确认同步与运维 | 已实现 | 按矩阵继续剩余能力 | 生产数据、迁移与部署 |
| 25受控旧记录转换/幂等导入 | 已实现 | 保源与冷scope限制见课程 | 实际legacy字段与Cloud累计兼容 |
| 26 MCP协议循环 | 已实现 | 消费27/28边界 | 真实MCP协议接入 |
| 27七个Cloud工具 | 已实现 | 真实授权后逐能力验证 | MCP权限、分页、私有图片与状态操作 |
| 28独立Host MCP入口 | 已实现 | 保独立env/stdout/输入所有权 | 目标部署解析与真实MCP |
| 29过滤与HTTP payload | 已实现 | 保默认关闭与有限预算 | 实际业务采集覆盖 |
| 30源码上下文 | 已实现 | 继续下一已设计采集覆盖块 | 目标源码布局与部署权限 |
| 31Postgres慢SQL | 已实现 | 32已补私有实例两路径验证 | 生产TLS/权限、更多类型/所有池 |
| 32 DB facade观察 | 已实现 | 继续矩阵后续能力 | 生产连接配置/TLS/权限与所有池 |
| 33 M05第一阶段崩溃诊断 | 已实现 | 保raw私有与显式slot所有权 | OOM/实际kill、其它OS与目标部署 |
| 34显式诊断导入 | 已实现 | 保持单ID手动导入；PHP无对应startup自动扫描必做 | 真实崩溃材料与部署授权 |
| 35 typed Cloud摘要 | 已实现 | 独立工作台摘要首页按P1后置 | 真实Summary权限与部署 |
| 36日志采集策略 | 已实现 | 保持37真实元信息契约 | 实际业务日志覆盖 |
| Cloud Go元信息三消费者 | 已实现 | 根端21:24已验证Go页面/心跳与两saved，保历史14:59观察 | 其它字段、MCP与剩余操作权限 |
| 37真实构建元信息 | 已实现 | Go/Goravel线上元信息已核，SDK development未评估正确 | 正式发布版本策略及其它字段 |
| 38独立模块解析构建 | 已实现 | 新P1最低线独立报告已交付，后续发布候选再验证 | 远端tag/discovery/鉴权与目标部署 |
| 39固定自检runtime可选解决 | 已实现 | 保saved与解决结果独立，继续M06 | 真实resolve能力与目标部署 |
| 40 M06 MySQL观察 | 已实现 | 官方dialector/Pool与Host接线定向通过 | 实际MySQL/MariaDB版本、TLS/生产池 |
| 41 M06 SQLServer观察 | 已实现 | 固定新三SHA最低线生产build已通过，继续P1模块 | 实际TDS/TLS/OUTPUT触发器/生产池 |
| 原生fatal/CLI与各平台运行机制差异 | 平台差异 | 保33/34诊断与显式导入边界，继续核覆盖 | 其它OS/真实进程退出 |
| 远端发布、Cloud剩余操作与部署 | 外部待验 | 本地实现不冒充外部验证 | tag/discovery/鉴权、MCP操作与生产迁移 |
| 工作台HTTP自动payload隔离 | 已实现 | 持续保All前排除与核心错误观察 | 手工Capture/原生日志仍属业务边界 |
| 默认敏感键pwd/api_key | 已实现 | Host86首节点真实payload/URL/SQL消费已通过 | 业务自定义敏感键覆盖 |
| Slow独立容量/日cap/MaskKeys | 已实现 | Host86 typed override实际消费；nil继承保兼容 | 目标业务配置 |
| Host CLI两skip配置 | 已实现 | 默认true/true不变，类型能力与显式标记进入原Filter | 业务CLI采集覆盖 |
| ORM Fresh新实例观察恢复 | 已实现 | 保原lazy factory/Fresh，静止窗口识别callback registry并有界保留旧代；见Host86 | 生产池/并发Fresh不在本地保证内 |
| Runtime trace字节与独立应用帧配置 | 已实现 | Host86真实消费原TraceMaxBytes/AppFramePrefixes，Source关闭仍可投影 | 目标源码布局与部署 |
| skeleton生成迁移固定挂载 | 已实现 | Host82真实空codegen与完整链，保四手迁移不重复create | 目标应用迁移与发布 |
| 工作台解析前请求守卫 | 已实现 | 原自动payload隔离之外，所有公开HTTP/TLS入口先守form字节/协议预算 | 目标反向代理/部署入口 |
| P1-02设计器核心页面与迁移记录 | 已实现 | 草稿create/rename/delete及v2同名复用已接；历史归属不明保守保护 | 目标业务迁移与部署 |
| P1-03真实路由目录页面 | 已实现 | 真实调试跳转已接，工作台自身prefix不提供无效跳转 | 目标业务登记覆盖 |
| P1-03纯Request字段元数据 | 已实现 | 80真实生成Fields与Transport已接，82标准挂载成功后公布；未知保持unknown | 目标业务额外校验语义 |
| P1-03有界代理与typed编码内核 | 已实现 | 78真实Session/CSRF/单次票据页面已接，四typed请求体保持预算 | 授权owned目标外的联调/生产接口 |
| P1-03 names-only历史 | 已实现 | 76当前可信元数据受控缓存/本人历史与页面已接 | 目标账号/文件部署策略 |
| P1-04 S-Cloud | 已实现 | 81真实状态/预览/显式四动作及安全站点链接已接，不自动重试未知POST | UI实际业务Cloud运维与剩余权限 |
| P1-05设置四区 | 已实现 | 83–85基础六组/Env/私有AI配置/人员真实页面与Host已接，不发AI请求 | 目标部署配置与操作 |
| skeleton新项目初始化/升级/第二业务 | 待实现 | 设置后按[正式计划](../../go-moo-scaffold/docs/plans/template-project-lifecycle.md)实现J1/R1、allowlist新目标/两业务/受保护升级，不伪发布版本 | 发布解析、生产迁移、E2E与部署 |

上述状态限定为**已实现／待实现／平台差异／外部待验**；已实现不代表整个M条目或五模块完成。

## 顺序与接续

23确认resolved维护 → 24只读迁移体检 → 25受控格式转换/幂等导入 → MCP协议loop → Cloud工具/分页/私有图片 → Host显式MCP采用。之后逐块核monitor余矩阵：summary/配置兼容、payload过滤/采集覆盖、旧env/生命周期/部署。不能把列入队列当交付。

monitor可用依赖齐后，转Scaffold一期五模块（数据库设计、安全接口调试、路由、S-Cloud、设置四区），再完善skeleton初始化/发布解析/多业务交付。范围依据[复现计划](port-plan.md)和Scaffold[一期计划](../../go-moo-scaffold/docs/phase-1-plan.md)。不预先执行生产迁移、真实Cloud写或E2E。

实际Cloud仅用户授权的心跳+runtime/slow逐项saved已由root验证，见Host[接入记录](../../go-moo-engine-skeleton/docs/integration/cloud-connection.md)；不扩为MCP或全面字段已验。
