# TODOS

- [x] 当前Monitor本地业务代码与Host消费已交付；2026-11-03全包race、私有PostgreSQL/Redis及受影响Host业务验证已完成，首失败和复验见[全盘矩阵](../go-moo-engine-skeleton/docs/plans/full-business-verification.md)。
- [ ] 按[执行队列](docs/execution-queue.md)保留真实 MCP todo 写/私有图片权限、Cloud summary/discard/其它操作、目标部署/跨OS、远端发布解析与目标 MySQL/TLS/生产池验证；SQLite/MySQL 本地双入口已验，SQLServer 按用户决定不测试。

- [x] 单条runtime记录的Go/Goravel版本接收、持久化、读回、Markdown/UI已接通并在2026-11-03线上验证（`e9af916`，[兼容设计](docs/plans/cloud-go-compatibility.md)）；迁移已Ran，实际runtimes/get返回go/goravel且php/laravel为空，历史记录不伪造版本。
