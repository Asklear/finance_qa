# stringset

负责查询层常用字符串集合操作：

- 追加并去重字符串。
- 按 trim 后的文本判断集合包含关系。
- 从候选指标中选择第一个非空值，否则使用默认值。

根目录的 `stringset_facade.go` 保留原 `query` package 的兼容入口。
