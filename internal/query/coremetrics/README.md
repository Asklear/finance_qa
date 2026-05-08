# coremetrics

负责核心指标领域中可独立拆分的辅助能力：

- 利润现金桥 `ProfitCashBridge` 的 map 序列化。
- 利润现金桥关键估算值读取。
- 字符串 map 克隆。

核心指标查询、可用性判断和账务/现金取数仍保留在 `internal/query` 根目录，因为当前实现依赖 `Engine`、DB、会计计算器和缓存。
