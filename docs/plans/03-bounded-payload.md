# 第03步：M07有界递归payload

本步只实现MaskPayload(value,maxStringChars)，不接事件/存储/Host。既有MaskSecrets/New不变，不自动加SQL脱敏。

## 源机制与Go边界

源私有maskRecursive在RuntimeErrorRecorder，真实ctor显式非nullconfig与假absolute basePath可短路Host config/StorageScope，仅赋字段和new Masker。probe只require已审4文件声明，拒额外autoload，仅反射此私有方法；php://memory假resource在finally关闭，无record/build/文件/Host。

depth从0开始，>5返回<deep>；数组每key先--共享budget5000，负数在当前容器写__truncated__后停止。敏感key整值***仍耗1但不遍历。字符串先MaskSecrets，再按Unicode codepoint长度，超cap保max(0,cap-20)前缀并加…<+N chars>；小/负cap沿源，不保证marker总长≤cap。对象/资源只名称占位，不遍历属性、不再截占位。

Go支持map[string]any、[]any、可选OrderedObject、nil/bool/int64/float64/string及显式OpaqueObject/Resource。native map当前层排序，不能恢复PHP原序，预算子集可能不同；OrderedObject保原序。普通int/任意struct/Stringer、非有限float、非法UTF8、重复或规范整数串对象键明确error无partial，不调用String或反射属性。

key结构预检/排序扫描当前容器全部keys；budget限制访问/输出分支，不是总输入分配、排序CPU或字符串扫描的硬上限。不整树预递归转换，未来采集适配仍需统一类型与输入容量策略。

## 实际验证

2026-10-29，--payload受控源probe真实采28固定假case，raw fixture含配置、源SHA/commit、mbstring、输入有序spec、initial/remaining budget；basic/sql stdout与原fixture逐字不变。空object/array源均[]，array预算截断变数字key+marker对象，marker碰撞原位置覆盖。

Go1.27.3 GOWORK=off仅TestPayloadFixture与TestPayloadInputBoundaries最终通过（0.881s）：源码形状/剩余预算、depth、默认5000/5001、字符截断、对象/resource、稳定map与无Stringer/错误无partial边界。首过后前缀截取避免整串[]rune副本，同两项重跑；不改规则。未改旧三个方法，不重复其回归，不跑race/全量/E2E或DB/Host/Cloud。

用户练习待完成，M07仍有未知输入表示与Host适配余项，不宣称支持任意Go请求对象。职责与命令见[第03课](../lessons/03-bounded-payload.md)。
