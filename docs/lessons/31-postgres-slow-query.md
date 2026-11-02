# 第31课：保留Postgres原始身份，另算展示SQL

[上一课：30](30-source-context.md) · [课程目录](../README.md)

本课扩展既有GORM插件到明确Postgres dialect，使用成熟postgres1.6.0/GORM1.31.2和sqlmock验证执行回调。[计划](../plans/31-postgres-slow-query.md)给出范围。31先并行提交，30交付后已补数字导航；此课没有安装或连接真实PG。

## 不能只放开一个driver名字

真实Postgres Dialector.BindVarTo写$n，而原Slow fillBindings沿PHP只按?顺序替换。如果只放开Install白名单，采集成功也可能没有正确的展示值。

现在Install/Initialize只明确支持sqlite或postgres，既有六callback、生命周期和原Logger调用不变。postgres collect在核心SQL执行后、原Statement清理前同步读SQL/Vars；先用已有bindings把值归一为不可变scalar/time，拒绝任意Valuer/Stringer，再调用dialector.Explain。原native Trace仍收到一次调用，监控同步工作仍可能增加其原观测耗时。

pgx.QueryExecMode只在首项且为锁定五枚举时剥除；这是已核dialector的特殊前导选项，不把它当业务值或调用String。未知模式、其它位置的模式仍安全拒采。普通业务SQL、RowsAffected和error不会被监控失败替换。

## Raw与RenderedSQL只各有一份职责

Input新增可选RenderedSQL。Raw始终是原$n语句用于既有hash归一，不换成带值展示SQL；RenderedSQL仅决定SQL.Last展示，与Bindings必须互斥。SQLite仍nil，继续原?路径。持久Record/Cloud结构没有增加字段。

Accept在threshold/skip之后、进入dailycap冻结之前检查RenderedSQL，坏UTF8/空文本/超4MiB或双真值拒绝；不能因今天已冻结就把坏输入算进overflow。Build直接消费时也检查。之后仍统一SanitizeStoredSQL双mask及4096码点存储cap，不因成熟dialector已转义就跳过脱敏。

展示前有保守预算：限制raw/绑定数量，字符串转义按扩大上界、每个重复$n单独计入，先与剩余额度比较再做算术，超限不调用Explain。预算是监控展示保护，不改数据库实际请求。

## 成熟formatter也不是wire反编译

锁定GORM Explain会在引号中的$n也做替换。例如原select '$1', $1可能展示成select ''value'', 'value'。本课按成熟行为实测并公开，不造SQL解析器伪装修复；任何展示SQL都绝不执行。

重复$1和$10按对应位置展示，Raw/hash仍保原参数化输入。不同绑定值不该通过RenderedSQL改变同一原语句的累计身份。这里不承诺PHP的?与Go的$n跨SDK天然同hash，也不宣称展示与真实PG协议、类型编码完全相同。

真实服务还需验证权限、TLS/连接、类型/数组扩展、事务/Rows迭代等；本课没有用mock冒充数据库服务。Row/Rows的后续Scan/迭代耗时与后发错误仍超出同步执行窗口。成功执行但后来事务回滚的SQL已发生，仍可被记录，不伪称只统计提交后的动作。

## 实际验证与依赖

固定依赖由writer真实get/tidy并回读：gorm postgres1.6.0、[sqlmock1.5.2](https://github.com/DATA-DOG/go-sqlmock/releases/tag/v1.5.2)、pgx5.7.2对齐Host既有图；GORM1.31.2/framework1.18/xsys0.49保持。go.mod+6行、sum+13行，无旧版本漂移；本机Go1.27.3，不当最低版真机证明。

纯Rendered边界首测0.707s。新GORM/sqlmock普通0.816s，六真实GORM执行callback、事务回滚、WithContext身份、错误/取消/Close、Valuer仅native一次且监控不二次调用均有断言。曾误把框架-env参数也传纯slowquery包，整条命令失败；0.816只指独立通过的adapter输出，不遮掩该失败。

正确分包后，旧Slow源fixture/安全规则+Rendered0.409s；旧SQLite callbacks/window/安装回滚+新增冻结Rendered无写1.331s。新Postgres范围race1.936s。只有sqlmock执行层，没有真实PG或Host池验证，不运行全量/E2E，不读真实.env/凭据。

用户自主练习待完成：解释Raw/hash和展示值为何分离；预测$1重复与$10索引；说明为何先归一再Explain；给出事务回滚仍可记录的理由；列出mock通过还不能证明的真实PG行为。
