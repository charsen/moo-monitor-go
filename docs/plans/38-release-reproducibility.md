# 38 分层验证独立解析与构建

本步先验证本地HEAD快照的可复现构建，不伪称已发布。用各仓已提交HEAD的git archive，不打包工作区、ignored .env、账号、用户IDE或未提交P1；记录SDK/scaffold/Host SHA与toolchain/平台。

SDK与scaffold使用缓存锁定golang.org/x/mod v0.37.0的zip.CreateFromDir，生成显著fixture prerelease版本的标准Go module zip/mod/info。全部helper/proxy/新cache/temp Host在自有临时根，结束只清该根。临时Host只加两个精确模块的fixture require；记录完整临时manifest差异，不改真实Host require，不用replace/workspace来证明独立解析。

运行时GOWORK=off、GOTOOLCHAIN=local、GONOPROXY=none；GOPROXY只含本地fixture与当前cache/download的file URL，无HTTP/direct fallback。GOSUMDB=off仅本次离线夹具，不作为生产配置建议。外部依赖缺缓存即报错，不为补齐追latest/联网/系统安装。构建Host主程序与独立MCP二进制但不运行，不执行全量/E2E/生产迁移。产品源码缺失/提交闭包问题应报告具体消费者，不临时patch产品冒通过。

[Go Modules Reference](https://go.dev/ref/mod#private-modules)明确GOPRIVATE默认影响GONOPROXY/GONOSUMDB；fixture要显式GONOPROXY=none才不绕过本地proxy。[VCS发现](https://go.dev/ref/mod#vcs-find)与[proxy协议](https://go.dev/ref/mod#module-proxy)分别说明仓发现与版本归档机制。SSHclone成功不代表无.git模块路径的go-get discovery和私有认证已通过；本步不取凭据、不请求真实Gitee、不push/tag。远端发布/解析另需对应授权与实测。

当前Host实际manifest未require两个私有包，依赖workspace；夹具改动仅说明将来的发布形态可否构建，不能把fixture版本复制进真实生产manifest或当远端tag证明。计划起点当时待37提交；实际执行结果见下方，未阻P1并行。

## 实际执行

固定SDK5cceac7241f305c35b9cb67652b973c35dc76b14/scaffolda9c9b46975f5cff342ea5da028e58eef315a56f9/Host8fd3e278f395aac15e6eaccf4f733b224dea42db，archive PAX与SHA256均实核。首工具mac temp alias guard误拒保failed原JSON；规范化root后Go1.27.3 app+MCP build通过。root提供官方已核Go1.26.8绝对工具链后，同图两target floor通过，不运行或复跑测试。

两个passed原始报告+八项比较在docs/contracts/fixtures/release-reproducibility*.json；完整临时mod/sum差异与两requires/MVS分别保存（29/47行），包含flock indirect与xsys0.48→0.49，没有改真实Host manifest。两次ownedroot清理/残留0实核。仅同源同图可构建，不是bit-identical、远端tag、Gitee auth/discovery、生产部署或其它平台验收。38后继续P1，无自动发布副作用，练习仍待用户。

快照边界：上述scaffolda9c9b46/Host8fd3e27早于P1-01的52dc598/b326d77，不能据此称新账户/session已独立floor构建。root新增要求：38提交后固定新三个SHA，另存一次Go1.26生产构建报告，保本两份report原样，不改产品规避失败；路由目录只读准备可并行。
