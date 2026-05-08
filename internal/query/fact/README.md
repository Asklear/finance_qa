# fact

负责查询编排中的事实载荷模型：

- `Fact` 表示一个可归因的指标事实。
- `FactSet` 表示某个来源适配器返回的一组事实。
- `AuthorityLevel` 和 `CoverageStatus` 描述事实权威性和覆盖状态。

根目录的 `fact_facade.go` 保留 `query.Fact`、`query.FactSet` 等兼容类型别名。`AnswerFrame` 暂时仍在根目录，因为它同时依赖 `QuerySpec` 和 `QueryPlan`。
