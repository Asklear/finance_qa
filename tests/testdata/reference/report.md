# 财务 AI 查询引擎最终测试报告

本次测试全面覆盖了多维度自然语言财务问题，重点测试了**数据推断、双口径计算、实体解析及时间意图识别**。使用真实数据库南京优集2026年数据。

| 编号 | 测试问题 | 系统回答核心内容 | 耗时 |
| --- | --- | --- | --- |
| 1 | 三月收入 | ✅ **2026-03 月度汇总查询成功（双口径对比）**<br>• **period**: 2026-03<br>• **[业务现金流口径(看钱)]**<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 净现金流: 0.00<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 现金流入: 0.00<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 现金流出: 0.00<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 说明: 业务现金流（实收实付）<br>• **现金流入**: 0.00<br>• **营业收入**: 0.00<br>• **[财务做账口径(看利润)]**<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 营业总成本: 0.00<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 营业收入: 0.00<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 说明: 2026年3月 权责发生制（从序时帐计算）<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 账面利润: 0.00 | 897 ms |
| 2 | 飞未云科客户今年销售额多少 | ✅ **飞未云科 income 金额查询成功**<br>• **entity**: 飞未云科<br>• **metric**: income<br>• **period_from**: 2026-04<br>• **period_to**: 2026-04<br>• **total**: 0.00 | 498 ms |
| 3 | 这个月整体支出多少 | ✅ **2026-02 月度汇总查询成功（双口径对比）**<br>• **[业务现金流口径(看钱)]**<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 净现金流: 1432168.44<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 现金流入: 3940965.11<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 现金流出: 2508796.67<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 说明: 业务现金流（实收实付）<br>• **现金流出**: 2508796.67<br>• **营业总成本**: 2474054.52<br>• **[财务做账口径(看利润)]**<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 营业总成本: 2474054.52<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 营业收入: 2485230.88<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 说明: 2026年2月 权责发生制（从序时帐计算）<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 账面利润: 11176.36<br>• **period**: 2026-02 | 104 ms |
| 4 | 人力成本多少 | ✅ **2026-04 应付职工薪酬查询成功**<br>• **account**: 应付职工薪酬<br>• **history**: [map[amount:46500 period:2026-02]]<br>• **period**: 2026-04<br>• **total**: 0.00 | 99 ms |
| 5 | 阿里云计算有限公司供应商支出多少 | ✅ **2026-04 月度汇总查询成功（双口径对比）**<br>• **[业务现金流口径(看钱)]**<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 现金流入: 0.00<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 现金流出: 0.00<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 说明: 业务现金流（实收实付）<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 净现金流: 0.00<br>• **现金流出**: 0.00<br>• **营业总成本**: 0.00<br>• **[财务做账口径(看利润)]**<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 营业收入: 0.00<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 说明: 2026年4月 权责发生制（从序时帐计算）<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 账面利润: 0.00<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 营业总成本: 0.00<br>• **period**: 2026-04 | 94 ms |
| 6 | 飞未云科 3月数据出来了吗 | ✅ **2026-03 数据可用性检查完成**<br>• **available**: false<br>• **entity**: <br>• **period**: 2026-03<br>• **rows**: 0.00 | 134 ms |
| 7 | 内部研发项目1月收入多少 | ✅ **2026-01 月度汇总查询成功（双口径对比）**<br>• **period**: 2026-01<br>• **[业务现金流口径(看钱)]**<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 现金流出: 7167000.93<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 说明: 业务现金流（实收实付）<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 净现金流: 2500143.01<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 现金流入: 9667143.94<br>• **现金流入**: 9667143.94<br>• **营业收入**: 5243422.58<br>• **[财务做账口径(看利润)]**<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 说明: 2026年1月 权责发生制（从序时帐计算）<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 账面利润: 9752.81<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 营业总成本: 5233669.77<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ 营业收入: 5243422.58 | 133 ms |
| 8 | 内部研发项目1月成本多少 | ✅ **** | 137 ms |
| 9 | 1月销项税额是多少 | ✅ **2026-01 税务查询成功**<br>• **net_vat**: 31013.43<br>• **period_end**: 2026-01<br>• **period_start**: 2026-01<br>• **total_input**: 283591.92<br>• **total_output**: 314605.35 | 148 ms |
| 10 | 1月进项税额是多少 | ✅ **2026-01 税务查询成功**<br>• **period_end**: 2026-01<br>• **period_start**: 2026-01<br>• **total_input**: 283591.92<br>• **total_output**: 314605.35<br>• **net_vat**: 31013.43 | 136 ms |
| 11 | 1月总成本是多少 | ✅ **2026-01 总成本查询成功**<br>• **period**: 2026-01<br>• **total_cost**: 0.00 | 125 ms |
| 12 | 1月应收账款多少 | ✅ **2026-01 应收账款查询成功**<br>• **period**: 2026-01<br>• **total**: 0.00<br>• **type**: 应收账款 | 111 ms |
| 13 | 1月应付账款多少 | ✅ **2026-01 应付账款查询成功**<br>• **period**: 2026-01<br>• **total**: 0.00<br>• **type**: 应付账款 | 106 ms |
| 14 | 研发项目的应收 | ✅ **2026-04 应收账款查询成功**<br>• **total**: 0.00<br>• **type**: 应收账款<br>• **period**: 2026-04 | 136 ms |
| 15 | 公司财务健康度怎么样 | ✅ **2026-04 账龄分析成功**<br>• **company**: 模拟财务科技有限公司<br>• **health_score**: 100.00<br>• **payable_buckets**: [map[amount:0 label:0-30天] map[amount:0 label:31-60天] map[amount:0 label:61天以上]]<br>• **payable_total**: 0.00<br>• **period**: 2026-04<br>• **receivable_buckets**: [map[amount:0 label:0-30天] map[amount:0 label:31-60天] map[amount:0 label:61天以上]]<br>• **receivable_total**: 0.00 | 105 ms |
