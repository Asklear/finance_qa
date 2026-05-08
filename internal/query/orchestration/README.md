# orchestration

负责编排结果组装时的低耦合辅助函数：

- 按来源查找 `FactSet`。
- 从 `FactSet` 中提取指标值和 trace 字符串。
- 把来源能力列表转成字符串。
- 提供结果组装中常用的轻量类型转换。

主编排器和业务结果组装仍保留在 `internal/query` 根目录，因为它们依赖 `QuerySpec`、`QueryPlan`、`SourceRegistry` 和多个业务域。
