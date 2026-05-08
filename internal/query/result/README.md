# result

负责查询结果的基础返回结构：

- `Result` 是 CLI、MCP 和测试共同消费的查询返回模型。
- `WithTraceData` 统一补齐 trace、process、executed_sql 和 calculation_logs 字段。

根目录的 `result_facade.go` 保留 `query.Result` 类型别名，避免外部调用方改 import path。
