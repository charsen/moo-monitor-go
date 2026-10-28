# 第02步：M07 SQL值侧脱敏

前置为第01课基础Masker。本步新增MaskSensitiveSQL，迁移源当前完整“比较值→INSERT列位置”两阶段算法；不执行SQL、不引SQL parser，不加入递归payload/事件/Host接线。

## 算法与配置

New为有效UTF8 keys动态Compile SQL比较pattern，错误明确返回；固定常量pattern与运行配置分开。比较支持=、!=、<>、>=、<=、>、<、LIKE、IN后的引号/括号/非负数字字面量，再按INSERT敏感列位置处理多个tuple。

splitSQLList保源quote/backslash状态机，cleanSQLIdentifier取最后点段并用源ASCII trim集合清引号/bracket；普通列/值保留，命中INSERT时tuple逗号空格归一也沿源。MaskSecrets仍是另一入口，不暗自动追加。

有限pattern未覆盖负数、NULL、裸词和嵌套函数等所有语法；源部分遮盖可能留下值或改变诊断文本形状，如实fixture，不擅自宣称全面安全或修成另一套SQL语义。

## 实际验证

2026-10-29，先审新增--sql脚本路径并php -l通过；只require精确SensitiveMasker，对30组固定假SQL输入采raw stdout到sensitive-masker-sql.json。默认basic输出与原fixture逐字一致，PHP源commit28d424e/PHP8.3.35及文件SHA保留。

Go1.27.3、GOWORK=off，仅TestSQLSourceFixture加New变化影响的原TestSourceFixture/TestConfiguration通过（1.183s），SQL30例与原basic31例对真实源输出相同。gofmt只本步三Go文件；无DB、Host、Cloud、race、全量或E2E。

用户练习待完成；递归深度/预算/UTF8截断和事件采集后续单独推进，整个M07与原日志泄露预防尚未完成。职责/命令见[第02课](../lessons/02-sql-masker.md)。
