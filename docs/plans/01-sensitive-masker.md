# 第01步：M07基础脱敏纯函数

本文保留该节点的设计与分批执行证据；下文“当前、未接、下一步、暂停”须结合节点时点阅读，不能覆盖后续已交付能力。稳定规则仍有效，已替代契约与本轮正文修订见[逐篇校准索引](../design-calibration.md)。旧命令是执行记录，复用前需按现脚本与私密隔离环境核查，不自动启动测试或外部操作。

首批只实现mask.New(keys)(*Masker,error)、ShouldMaskKey、MaskURL与MaskSecrets；不实现SQL脱敏、递归payload、事件/缓冲、Host/Gin接线或Cloud。

实际go mod init gitee.com/charsen/moo-monitor-go后设最低Go1.26.0，只标准库；本次实测工具链Go1.27.3，未实际运行1.26。不导入scaffold/Host，不加入Host workspace。

## 源证据与配置边界

纯PHP脚本tools/probe-sensitive-masker.php只require已审SensitiveMasker精确文件，31组固定假输入输出原stdout保存docs/contracts/fixtures/sensitive-masker-basic.json；无Composer/Host/.env/DB/Cloud。源commit28d424e286222c391b7e64cfb5643e788dfcc456、PHP8.3.35及source文件SHA保存在fixture。

New复制配置，过滤PHP array_filter对应空串与字符串0。有效UTF8 keys支持，非法UTF8明确构造error，是Go配置边界差异；不静默丢pattern或返回未脱敏文本。后续consumer须按M18隔离初始化错误，当前尚无Host消费者。

大小写处理仅ASCII，保留原串捕获前缀与非敏感文字；不用Go Unicode casefold扩大源规则。动态配置regex用Compile返回error，静态固定pattern独立复用，实例不暴露可变配置。

URL按原序/raw编码处理query，不用url.Values重编码；urldecode容忍坏percent/+，只替换命中且有等号的值。MaskSecrets依Authorization→JWT→Bearer→配置键顺序，空keys仍有前三种固有规则。

## 实际验证

2026-10-29，审脚本后php -l通过，真实局部probe写31-case原始fixture；Go独立GOWORK=off定向TestSourceFixture与TestConfiguration最终通过（0.810s），逐项对比源实际输出并验证配置拷贝/非法UTF8拒绝。Bearer/JWT先后与多词文本部分保留两case补齐后只重跑既定两项；gofmt仅本步两Go文件且-d无差异，diff空白通过。

fragment/userinfo/path和无标签自由文本不保证脱敏，源输出中这些假值保留；公开列缺口，不将有限pattern说成全面安全或整个M07完成。SQL/递归/M02事件/日志原body边界仍待后续；没有PHP Host、外部云、全量、race或E2E。

用户自主练习待完成，详细职责与命令见[第01课](../lessons/01-sensitive-masker.md)。Git交付由统一实施单元审查与操作。
