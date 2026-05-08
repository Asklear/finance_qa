# arap

负责应收应付领域中可独立拆分的辅助能力：

- open item 余额和冲销说明的消息格式化。

实际应收应付查询仍保留在 `internal/query` 根目录，因为当前实现直接依赖 `Engine`、DB 和 open-item 缓存。
