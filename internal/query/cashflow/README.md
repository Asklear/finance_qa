# cashflow

负责现金流方向模型：

- 判断银行/现金类科目。
- 将会计借贷方向转换为现金流入/流出。
- 汇总往来单位层面的流入、流出和净额。

根目录的 `cashflow_facade.go` 保留原 `query` package 的兼容入口。
