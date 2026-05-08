# bridge

负责给宿主模型和 MCP 包装层使用的兼容摘要：

- 从查询结果 `data` 中构造 `host_summary_contract`。
- 生成 `final_answer` 文本。
- 兼容合同严格缺失、合同维度汇总和回款子期间摘要。

根目录的 `bridge_facade.go` 保留 `query.BuildHostSummaryContract` 和内部 final-answer 兼容入口。
