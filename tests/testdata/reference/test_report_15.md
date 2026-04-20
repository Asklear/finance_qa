# 财务问答测试报告（15题）

- 测试时间: 2026-04-10
- 测试命令: `financeqa query --company 南京优集 "<问题>"`
- 说明: `xxx客户` 替换为 `C0147CF000KKYKZ`（来自 `entities` 表的真实 customer）；`xx月` 替换为 `2月`。项目维度暂无可用真实项目编码。

- 总题数: 15
- 成功: 8
- 失败: 7
- 平均耗时: 154.67 ms
- 最快: Q5 40.52 ms
- 最慢: Q1 1488.76 ms

| 编号 | 测试问题 | 回答 | 耗时(ms) | 状态 |
|---|---|---|---:|---|
| 1 | 三月收入 | 2026-04 月度汇总查询成功 { "income": 0, "period": "2026-04" } | 1488.76 | 成功 |
| 2 | C0147CF000KKYKZ客户今年销售额多少 | no matching account found for question "C0147CF000KKYKZ客户今年销售额多少" | 50.03 | 失败 |
| 3 | 2026年2月整体支出多少 | 2026-02 月度汇总查询成功 { "expense": 2508796.67, "period": "2026-02" } | 95.93 | 成功 |
| 4 | 2026年2月人力成本多少 | no matching account found for question "2026年2月人力成本多少" | 58.54 | 失败 |
| 5 | 供应商多少 | no matching account found for question "供应商多少" | 40.52 | 失败 |
| 6 | C0147CF000KKYKZ 3月数据出来了吗 | no matching account found for question "C0147CF000KKYKZ 3月数据出来了吗" | 47.22 | 失败 |
| 7 | C0147CF000KKYKZ项目2月收入多少 | 2026-02 月度汇总查询成功 { "income": 3940965.11, "period": "2026-02" } | 46.61 | 成功 |
| 8 | C0147CF000KKYKZ项目2月成本多少 | no matching account found for question "C0147CF000KKYKZ项目2月成本多少" | 42.06 | 失败 |
| 9 | 2月销项税额是多少 | 2026-02 税务查询成功 { "net_vat": 0, "period_end": "2026-02", "period_start": "2026-02", "total_input": 0, "total_output": 0 } | 50.06 | 成功 |
| 10 | 2月进项税额是多少 | 2026-02 税务查询成功 { "net_vat": 0, "period_end": "2026-02", "period_start": "2026-02", "total_input": 0, "total_output": 0 } | 52.35 | 成功 |
| 11 | 2月总成本是多少 | no matching account found for question "2月总成本是多少" | 169.27 | 失败 |
| 12 | 2月应收账款多少（已开发票未收款） | 2026-02 应收账款查询成功 { "period": "2026-02", "total": 6450139.77, "type": "应收账款" } | 47.44 | 成功 |
| 13 | 2月应付账款多少（已收发票未付款） | 2026-02 应付账款查询成功 { "period": "2026-02", "total": 14564151.21, "type": "应付账款" } | 41.22 | 成功 |
| 14 | C0147CF000KKYKZ项目的应收/应付 | 2026-04 应付账款查询成功 { "period": "2026-04", "total": 0, "type": "应付账款" } | 42.05 | 成功 |
| 15 | 公司财务健康度怎么？ | no matching account found for question "公司财务健康度怎么？" | 48.01 | 失败 |
