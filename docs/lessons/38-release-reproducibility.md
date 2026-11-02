# 第38课：分层验证独立解析与生产构建

[上一课：37](37-go-build-metadata.md) · [课程目录](../README.md) · [下一课：39](39-self-test-resolution.md)

[计划](../plans/38-release-reproducibility.md)验证的是同源同图可构建，不是发布、远端认证或逐字节一致的二进制证明。

## 文件与真实顺序

`tools/reproduce-release.py`要求三份完整固定SHA、Go工具链、只读现有download cache和新报告路径。它从SDK、scaffold、Host的commit tree生成git archive，核tar PAX comment确为该SHA并记archive SHA256；不复制工作区、index、ignored .env或未提交P1代码。固定快照不会随writer继续工作改变。

工具用缓存锁定x/mod v0.37.0的zip.CreateFromDir生成标准module zip/mod/info。`v0.0.0-repro.20261102`是显著测试夹具，不是远端tag。临时Host补两个精确fixture require，以`go list -mod=mod -deps`仅加载主App与独立MCP生产依赖，再readonly build两target；不运行产物、迁移、Provider或测试套件。两个私包必须来自自有module cache且没有Replace。

GOWORK=off、GOTOOLCHAIN=local，GOPROXY只有两个file源（自有fixture与现有cache），GONOPROXY=none防私有模块绕过proxy；GOSUMDB=off/GOVCS=*:off仅本次离线子进程。不改真实manifest/全局Go设置，不追latest或请求真实仓库。

## 实际结果与完整差异

固定输入为SDK `5cceac7241f305c35b9cb67652b973c35dc76b14`、scaffold `a9c9b46975f5cff342ea5da028e58eef315a56f9`、Host `8fd3e278f395aac15e6eaccf4f733b224dea42db`。首轮工具因macOS temp路径别名未统一而误拒正确的owned cache；原始[失败报告](../contracts/fixtures/release-reproducibility-initial-failed.json)保留，不计成功。修的是工具路径guard，没有patch产品代码。

[Go1.27.3报告](../contracts/fixtures/release-reproducibility.json)记录darwin/arm64、CGO=1：Host106272066字节、MCP10080546字节，均编译成功未执行。root准备并核官方来源的隔离Go1.26.8后，同固定SHA/fixture再生产构建：[最低线报告](../contracts/fixtures/release-reproducibility-go126.json)的Host108353394字节、MCP9169202字节同样通过。只称验证了1.26 minor线中的1.26.8，不替代每个历史patch或其它OS证明。

[实际比较](../contracts/fixtures/release-reproducibility-comparison.json)中八项源/图完全相同：commit、archive hash、fixture、私包版本/checksum/noReplace、完整go.mod/go.sum差异及显式require/MVS分栏。报告不是预填两行：除新增fixture require，实际还补flock indirect、使临时Host x/sys0.48升到0.49，完整go.mod差异29行、go.sum47行。此前workspace有效图已受SDK选择0.49；本步证明临时发布图也独立得到该版本，真实Host manifest仍未被改成伪版本。

两次owned root、helper、cache与二进制都已清理，回读同前缀残留0。二进制hash/大小不同是编译器/构建环境差异，不能把本课叫bit-for-bit reproducible。

## 外部门槛与练习

本地file proxy不能证明私有Gitee discovery、tag或凭据权限。[官方模块规则](https://go.dev/ref/mod#vcs-find)中无.git路径通常还要go-get发现；SSHclone成功不自动提供HTTP发现认证。真正发布、真实生产require、目标机下载与部署需要对应授权和独立验证。本步不push/tag，不请求真实Cloud。

下一动作按P1主线持续推进；发布不是默认副作用。自主练习待完成：比较workspace与fixture解析；解释为什么需要GONOPROXY=none；追踪两require之外的MVS变更；说明编译成功与远端可安装的距离；解释固定SHA为何排除并行未提交P1。

快照范围补充：scaffolda9c9b46和Host8fd3e27早于P1-01已提交的52dc598/b326d77，本课结果不覆盖新增账户/session。root已要求38提交后以新固定三SHA另存Go1.26生产两target回归；不会把新结果倒写旧快照，也不为本课再跑测试。

## P1-01新消费者追验

38提交后按root新增授权，用新固定SDK `4ba95a6b1ca4e533a526fc27ca08a5199acaa9de`、scaffold `52dc59850a2c11c1d8a634c84eeaad454cc59c59`、Host `5ca7f26ce29ead4f0de94a77c4aa2b3b1ef38269`另执行仅Go1.26.8生产构建。[独立新报告](../contracts/fixtures/release-reproducibility-p1-go126.json)中主App108555586字节/MCP9169202字节成功、未运行，P1账户/session实际新依赖已进入快照；ownedroot清/残留0。原38两报告未改，不能把新结果倒写到旧快照。此为编译/独立解析证据，账户行为仍按P1-01自己的定向/race结果，不是Go1.26功能全回归或远端发布认证。
