# rules

负责规则配置的低耦合工具：

- 环境变量字符串解析。
- JSON map 解析。
- 规则 map 的空值兜底和规范化。
- 字符串规则去重。

完整 `RuleConfig` 类型、`RuleConfigProvider` 接口和加载缓存流程仍在 `internal/query` 根目录，因为它们和 intent、metric、contract、counterparty 等模型耦合较深。
