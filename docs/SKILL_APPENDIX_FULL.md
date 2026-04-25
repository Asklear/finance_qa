---
name: "finance"
description: "Use when OpenClaw or Claude needs finance_qa to answer老板财务问题、读取结构化财务结果，或在不能直接精算时切到宿主LLM兜底。"
---

# finance_qa Agent 调用手册（桥接暴露 + 仓库功能全景）

本文档目标：把“当前已通过 bridge 暴露给 OpenClaw / Claude 的能力”和“仓库内已实现但不适合直接注入老板问答上下文的能力边界”同时说清，避免宿主误判可调用范围。

## 0. 附录状态

1. `appendix_doc_version`: `2026-04-25.3`
2. `skill_contract_version`: `2026-04-25.3`
3. `bridge_protocol_version`: `v2`
4. `last_updated`: `2026-04-25`
5. 当前规范文件名：`docs/SKILL_APPENDIX_FULL.md`

## 1. 能力定位

`finance_qa` 是老板财务助理引擎，能力分为五层：

1. Bridge 暴露层：当前 bridge 注册 5 个工具：`finance-query`、`finance-host-data`、`finance-upload`、`finance-sync`、`finance-dimensions`。
2. 数据层：初始化库、导入报表、目录同步。
3. 规则层：自然语言意图识别、实体识别、账期识别。
4. 计算层：现金收付、经营确认、合同维度台账、税额、应收应付、项目收支等。
5. 兜底层：输出 `llm_payload` 全量财报上下文给上层 Agent 或宿主模型做最终判别。

说明：

1. 本附录面向宿主问答与财务规则，默认只注入回答老板所需的上下文。
2. 若人类操作者明确要求仓库维护细节，应另查 `README.md` 或对应说明，不要默认把这些内容注入老板问答上下文。

## 2. 过程暴露要求

所有查询响应都必须暴露以下字段，不可只返回结果值：

1. `success`
2. `message`
3. `answer_method`
4. `boss_reply`
5. `host_summary_contract`
6. `host_summary_supplier_payments`
7. `data`
8. `executed_sql`
9. `calculation_logs`
10. `data.trace.executed_sql`
11. `data.trace.calculation_logs`
12. `data.intent_trace.router_version`
13. `data.intent_trace.matched`
14. `data.intent_trace.scores`
15. `data.intent_trace.final_intent`
16. `data.intent_trace.confidence`
17. `data.query_spec`
18. `data.route_decision`
19. `data.route_decision.probe_results`
20. `data.query_pipeline`
21. `data.source_plan`
22. `data.fact_sets`
23. `data.source_catalog`
24. `data.source_note`
25. `data.source_documents`
26. `data.primary_source_tables`
27. `data.supporting_source_documents`
28. `data.extraction_errors`
29. `data.contract_fallback_reason`
30. `data.contract_fallback_target`
31. `data.exposed_fields.intent_trace`
32. `data.tax_inclusion`
33. `data.tax_inclusion_note`
34. `bridge_meta.skill_contract_version`
35. `bridge_meta.protocol_version`
36. `bridge_meta.capabilities`
37. `bridge_meta.capabilities.exposed_tools`
38. `bridge_meta.capabilities.result_structures`
39. `bridge_meta.skill_appendix_relative_path`
40. `bridge_meta.skill_appendix_path`
41. `bridge_meta.skill_appendix_exists`

说明：即使结果无法直接回答，也要尽量保留完整中间过程。若底层已经产出更完整的 trace、证据等级、规则链路或 SQL 解析结果，接口层应原样透出，不要裁剪。
补充：如果 `data.source_note` 已存在，宿主摘要时优先直接引用它，不要自行改写来源说明，以免打乱“主要来源 / 补充来源”的顺序。

## 3. 宿主运行接口（按需）

宿主当前可调用的 bridge 入口如下：

1. `finance-query`
   - 老板财务问答主入口
2. `finance-host-data`
   - 当 `finance-query` 不能稳定直答时，提供 `llm_payload`
3. `finance-upload`
   - 单文件导入财务报表或合同台账 Excel
4. `finance-sync`
   - 批量同步目录下财务文件
5. `finance-dimensions`
   - 维度管理入口，承载 `dimensions` 子命令

这里的“只需要”是 bridge 当前暴露范围，不代表仓库 CLI 只有这三项能力。

仓库内已实现、但默认不通过 bridge 暴露给宿主上下文的 CLI 能力包括：

1. `init-db`
2. `config show`
3. `keywords intents`
4. `sync`
5. `dimensions list`
6. `dimensions add-dimension`
7. `dimensions add-member`
8. `dimensions mapping-stats`
9. `dimensions seed-standard`
10. `dimensions export-package`
11. `dimensions import-dimensions`
12. `dimensions import-members`
13. `dimensions import-rules`
14. `dimensions preview-import`

导入落库约定（强制）：

1. PostgreSQL 导入目标表固定为 `fin_*` 实表，不依赖兼容 view。
2. 余额表导入到 `fin_balance_detail` 时，`opening_period` 必须写“会计期间第一个月份（期初月）”，`period` 写“会计期间第二个月份（期末月）”。
3. 序时账导入到 `fin_journal` 时，`period` 必须由 `voucher_date` 自动归一为 `YYYY-MM`，不允许空值。
4. 合同类 Excel 识别后写入：
   - `fin_contracts`
   - `fin_cost_settlements`
   - `fin_fund_income`
   - 客户合同的结算额/开票额/回款额统一落到 `fin_fund_income`
   - `fin_contracts` 保存合同主信息：`customer_name`、`contract_content`、`contract_start_date`、`contract_end_date`、`settlement_cycle`
   - `fin_cost_settlements` 保存成本侧行项目字段：`quantity`、`settlement_amount`、`invoice_amount`、`paid_amount`、`contract_start_date`、`contract_end_date`、`settlement_cycle`、`settlement_unit_price`
   - `fin_fund_income` 保存收入/回款侧行项目字段：`quantity`、`settlement_amount`、`received_amount`、`invoice_amount`、`contract_start_date`、`contract_end_date`、`settlement_cycle`、`settlement_unit_price`
   - 两张行项目表额外保存 `source_report_type`、`source_sheet_name`，作为来源分区键；合同 Excel 做全量重传时，只覆盖相同来源分区，不整表清空
   - 除 `year_month` 会按规则推断外，其他扩展字段默认保留源 Excel 原值；源单元格为空时，数据库也保持为空，不做硬补
   - 合同月度结算表的 `year_month` 必须动态推断：先按每行合同开始/终止日期匹配月份年份；若行内日期缺失，再按同 sheet 同月份的多数年份或 sheet 年份补齐
   - 合并单元格产生的空客户名不能直接跳过，要继承上一条非空客户名
   - 资金到账表要兼容任意 `xx年Qn收入明细` sheet，不允许只支持固定季度名称
   - `fin_revenue_settlements` 已废弃，仅保留历史兼容，不再作为查询来源
5. 每张业务表都应把“来源 Excel / sheet / 报表类型”写进表注释：
   - 注释格式统一为结构化 `financeqa_source`
   - 查询收口阶段统一从表注释提取 `source_note/source_documents`
   - 历史遗留的纯文本注释会在 bootstrap/query/import 时自动升级

## 4. 查询响应契约（Agent 必须按此解析）

## 4.1 顶层结构

```json
{
  "success": true,
  "message": "...",
  "answer_method": "sql|llm_payload",
  "data": {},
  "executed_sql": ["..."],
  "calculation_logs": ["..."]
}
```

## 4.2 `answer_method` 含义

1. `sql`: 规则与SQL计算得到结果。
2. `llm_payload`: 系统无法直接准确回答，转交上层 Agent 基于全量数据推理。
3. `success=false` 且存在 `data.extraction_errors`：表示宿主数据包抽取不完整，不能把当前 `llm_payload` 当完整事实继续下结论。

补充：当前默认意图路由为 `Intent Router V2`，会返回 `intent_trace` 说明命中的规则、得分和最终意图，便于审计与排错。

## 4.3 失败兜底结构（`success=false` 常见字段）

1. `data.fallback_attempted`
2. `data.hint`
3. `data.available_accounts`
4. `data.counterparty_sample`
5. `data.llm_payload`
6. `data.extraction_errors`
7. `data.contract_fallback_reason`
8. `data.contract_fallback_target`

## 5. 问题类型与处理模块

系统会根据老板的问题，自动分配到以下处理模块：

1. 原始数据包输出：把全量财报与过程数据打包给上层 Agent。
2. 主体身份识别：判断某个名字更像客户、供应商、员工，还是混合往来。
3. 应收应付查询：查应收账款、应付账款、项目应收应付。
4. 大额流水查询：查最大流入对手方、最大流出对手方、单笔大额流水。
5. 税额查询：查销项税、进项税、净税额。
6. 月度经营总结：查当月收入、成本、利润、支出、经营情况。
7. 经营分析：查账龄、健康度、差异原因分析。
8. 兜底查询：处理供应商数量、人力成本、整体支出、项目收入成本、某主体金额等问题。
9. 精确余额查询：查货币资金、银行存款、指定科目期末余额。
10. 合同维度查询：查客户合同结算/到账/开票，或供应商合同成本/实际付款。

补充规则：

1. 意图识别必须先按功能模块分流，再决定口径，不允许把“季度/半年/全年/累计”的区间问题误当成单月问题。
2. 总量型核心指标问题（收入/成本/利润/销售额）先尝试合同/项目汇总口径：`fin_contracts + fin_fund_income + fin_cost_settlements`；是否能回答以 `data.route_decision.probe_results` 的真实数据覆盖探测为准，只有合同汇总答不全时，才回退到现金收付 + 经营确认。
3. 如果合同汇总答不全并触发回退，响应中应保留 `data.contract_fallback_reason`，宿主必须显式告诉老板“合同台账当前不能直接回答，以下改按财务账/流水口径说明”。
4. `利润` 与 `净利润` 必须分开：
   - `利润` 默认按 `收入 - 成本及费用 + 营业外收入 - 营业外支出`
   - `净利润` 单独回答，不得和“利润”混说
5. 序时账汇总结果必须带“是否含税”说明；默认解释为按凭证入账金额统计、不主动剔税，若税额未单独拆分通常视为含税口径。
6. 如果问题里带有明确的真实主体，优先回答这个主体的金额或状态，不强行改成整月汇总。
7. 当直接规则无法稳定回答时，自动降级输出 `llm_payload` 给上层 Agent 继续判断。
8. 如果响应里出现 `query_pipeline=orchestrator`，宿主应视为后端已经完成多源聚合，不要再自行重排主口径。

当前已接入多源编排器的主查询族：

1. `core_metric`：收入 / 成本 / 利润 / 销售额，支持“合同/项目汇总优先，失败后回退到现金+经营/财务”。
2. `arap`：应收 / 应付，优先官方余额，再补开放项证据。
3. `supplier_payments`：按期间统计外部供应商付款名单与金额。
4. `contract_dimension`：客户合同与供应商合同，默认先现金后财务台账。
5. `readiness`：某主体 / 项目数据是否已出。

bridge 对这些查询族当前额外暴露的宿主摘要结构为：

1. `boss_reply`：老板口径结论/原因/建议
2. `host_summary_contract`：合同/项目维度及合同汇总结构化摘要
3. `host_summary_supplier_payments`：供应商付款期间汇总摘要，含：
   - `count`
   - `total`
   - `suppliers`
   - `top_supplier`
   - `excluded_counterparties`
   - `exclusion_reasons`
   - `supporting_evidence_used`
4. `data.route_decision`：主口径选择与轻量探测结果，含 `selected_source`、`primary_tables`、`fallback_reason`、`probe_results`；宿主只能用来判断口径和回退原因，不要原样展示给老板

## 6. 已支持问题能力清单（老板问法）

以下问题当前都已支持，且会返回中间过程：

1. 月度收入/成本/利润（老板问汇总时先尝试合同/项目汇总；答不全时再回退现金口径 + 经营口径，并保留差异桥字段）。
2. 某客户/供应商/主体在某期间金额（穿透审计）。
3. 这个月整体支出。
4. 人力成本。
5. 供应商付款数量 + 外部供应商名单与付款金额（按指定期间实际付款统计）。
6. 某实体某月数据是否已出。
7. 某项目某月收入。
8. 某项目某月成本。
9. 某月销项税额。
10. 某月进项税额。
11. 某月总成本（老板问汇总时先尝试合同成本；答不全时再回退现金支出 + 经营成本，必要时解释预提/冲回影响）。
12. 某月应收账款（余额表口径）。
13. 某月应付账款（余额表口径）。
14. 某项目应收/应付（项目净流入口径）。
15. 某主体身份识别（客户/供应商/员工/混合/未知）。
16. 某期间最大流入对手方/大额流水查询。
17. 某科目期末余额精确查询（如“货币资金余额是多少”）。
18. 账龄与健康度分析（应收/应付账龄桶与健康评分）。
19. 某客户合同在某年/某月结算多少、其中某月到账多少。
20. 某供应商合同在某年/某月成本多少、实际付款多少。

## 7. 意图识别与功能模块分流

对接层读取本 appendix 后，必须按下面顺序理解问题，不要跳步：

1. 先判断是不是明确主体问题：
   - 客户 / 供应商 / 员工 / 项目 / 分公司 / 具体公司名
2. 再判断是不是总量型核心指标：
   - `收入` / `营收` / `销售额` / `成本` / `利润`
3. 再判断是不是区间型时间范围：
   - `季度` / `Q1~Q4` / `上半年` / `下半年` / `全年` / `累计` / `年内`
4. 如果同时命中“总量型核心指标 + 区间型时间范围”，且没有明确真实主体：
   - 必须走公司级汇总
   - 必须使用 `from~to` 区间聚合
   - 不能直接拿 `to` 月单月结果充当区间结果
5. 如果命中明确真实主体：
   - 优先走主体审计/往来模块
   - 不要把主体问题改写成整公司汇总
6. 如果同时命中 `合同` + `结算/到账/付款/成本/开票`：
   - 优先走合同维度模块
   - `项目` 视为合同同义词；若识别到真实主体，也按合同维度处理
   - 客户合同先回答现金口径到账，再补财务口径合同结算/开票
   - 供应商合同先回答现金口径实际付款，再补财务口径合同成本
   - 混合合同也必须先现金、再财务
   - 合同优先关键词、来源表映射应视为可配置规则，不假设写死在代码中

## 8. 核心指标返回规则

当问题涉及总量型 `收入/成本/利润/销售额` 时，默认按以下优先级返回：

1. 先尝试合同/项目汇总：`fin_contracts + fin_fund_income + fin_cost_settlements`。
2. 合同汇总能回答时，优先输出老板口径的合同营收、合同成本、合同利润，并可补充合同回款/开票。
3. 合同汇总是否能回答，以 `route_decision.probe_results` 的覆盖状态为准，不靠关键词硬猜。
4. 只有合同汇总答不全时，才回退到“先现金，再经营/财务”。
5. 若发生这类回退，必须显式保留并透出 `data.contract_fallback_reason`，不能把回退后的结果伪装成合同台账原生答案。
6. 如果回答的是 `利润`，经营口径默认解释为：`收入 - 成本及费用 + 营业外收入 - 营业外支出`。
7. 如果老板明确问 `净利润`，要单独返回净利润，不得把“利润”字段直接冒充净利润。
8. 如果经营口径来自序时账汇总，必须同步输出含税说明。
9. 银行流水 / 实际到账 / 实际支出 / 净增加 / 回款问题，不强行先走合同汇总。

兼容字段：

1. `现金流入`
2. `现金流出`
3. `净现金流`
4. `cash_flow`
5. `money_view`
6. `account_view`
7. `财务做账口径(看利润)`

说明：

1. 核心指标不是一律“银行卡上看 + 经营口径”；老板问汇总时先看合同/项目汇总，只有答不全才回退到“先现金、再经营确认”。
2. `季度/半年/全年/累计` 这类区间问题，经营口径必须按区间汇总，不允许直接拿最后一个月的 `current_amount` 充当区间结果。
3. 如果问题继续追问 `差异原因`、`为什么不一样`、`回款和利润差异`，再把利润桥、税项时差和预提/冲回影响讲透。
4. 如果问题明确在问某个客户、供应商、员工或项目，优先返回主体审计结果，不强制改成整月现金和经营汇总。

## 9. 合理性交叉验证

输出结果前，必须做下面这些交叉验证；校验失败时要保留校验结果，并明确提示“需复核”：

1. 单月核心指标：
   - 校验 `收入 - 成本及费用 + 营业外收入 - 营业外支出 ≈ 利润`
   - 校验 `现金流入 - 现金流出 = 净现金流`
   - 如果问题明确问 `净利润`，再校验 `利润 - 所得税 ≈ 净利润`
2. 区间核心指标：
   - 优先使用 `SUM(current_amount)` 做区间汇总
   - 如果 `income_statement` 存在 `cumulative_amount`，必须再做一次 `累计差分` 交叉验证
   - 发现 `月度发生额` 与 `累计差分` 明显不一致时，要在返回里保留校验结果，不要静默吞掉
3. 主体收入 / 回款问题：
   - 不能把到账直接等同于销售额
   - 不能在未配对的情况下盲目说“比收回来的多/少”
4. 应收 / 应付问题：
   - 必须优先使用余额/配对逻辑
   - 不能因为同月一借一贷就直接完成冲销判断
5. 多源编排问题：
   - `message`、`source_plan`、`fact_sets` 三者必须一致
   - 不允许宿主忽略后端已给出的主口径，再自行取某个分源字段回答老板

## 10. 上层 Agent 兜底接口

代码内不直接调用上层模型，改为提供全量数据接口：

1. `query` 自动 fallback 时返回 `data.llm_payload`。
2. 可主动调用 `host-data` 直接获取 `llm_payload`。
3. 上层 Agent/接口层职责分离：接口负责完整暴露中间过程、证据等级、SQL 与规则链；上层 Agent 只负责最终自然语言判断与归纳，老板最终回复默认不展开这些过程字段。

`llm_payload` 内容：

1. `question`
2. `company`
3. `period`
4. `financial_tables.balance_sheet`
5. `financial_tables.income_statement`
6. `financial_tables.balance_detail`
7. `financial_tables.journal`
8. `financial_tables.bank_statement`
9. `financial_tables.fin_contracts`
10. `financial_tables.fin_cost_settlements`
11. `financial_tables.fin_fund_income`
12. `source_catalog`
13. `source_documents`
14. `source_note`
15. `query_spec`
16. `route_decision`
17. `route_decision.probe_results`
18. `extraction_errors`
19. `trace.intent`
20. `trace.strategy`

合同相关表在 `llm_payload.financial_tables` 中，宿主默认应按完整字段消费，不要自建白名单裁剪。尤其要保留：

1. `fin_contracts.contract_start_date`
2. `fin_contracts.contract_end_date`
3. `fin_contracts.settlement_cycle`
4. `fin_fund_income.source_report_type`
5. `fin_fund_income.source_sheet_name`
6. `fin_fund_income.contract_start_date`
7. `fin_fund_income.contract_end_date`
8. `fin_fund_income.settlement_cycle`
9. `fin_fund_income.settlement_unit_price`
10. `fin_cost_settlements.source_report_type`
11. `fin_cost_settlements.source_sheet_name`
12. `fin_cost_settlements.paid_amount`
13. `fin_cost_settlements.contract_start_date`
14. `fin_cost_settlements.contract_end_date`
15. `fin_cost_settlements.settlement_cycle`
16. `fin_cost_settlements.settlement_unit_price`

## 11. 宿主封装注意事项

1. 当前 `finance_bridge.py` 注册 5 个桥接工具：
   - `finance-query`
   - `finance-host-data`
   - `finance-upload`
   - `finance-sync`
   - `finance-dimensions`
   - 宿主应以 `bridge_meta.capabilities.exposed_tools` 为准，不要把仓库内其他 CLI 子命令误判成可直接调用的 bridge tool
2. 桥接层不会读取或注入 `SKILL.md` / appendix 正文，只会：
   - 读取 `SKILL.md` 顶部契约版本
   - 校验 appendix 相对路径存在
   - 把这些元信息写回 `bridge_meta`
3. 对任意宿主都要优先保留原始结构化响应，不要提前做摘要裁剪或字段白名单过滤。
4. `financeqa query` 的 CLI 行为必须注意：
   - 成功时，`stdout` 输出完整 JSON，exit code = `0`
   - 业务失败时，`stdout` 仍可能输出完整 JSON，exit code = `1`
   - 参数错误或系统错误时，才可能只有 `stderr`
5. 因此宿主应当：
   - 优先解析 `stdout` JSON
   - 再看 `success` 和 `answer_method`
   - 若拿不到结构化结果，再调用 `finance-host-data` 兜底
6. 如果 `finance-host-data` 返回 `success=false` 且存在 `data.extraction_errors`：
   - 说明宿主数据包提取不完整
   - 宿主不能再把当前 `data.llm_payload` 当完整事实回答老板
   - 应直接暴露抽取失败信息，并提示先修复库表/字段再重试
7. 如果结果带有 `data.contract_fallback_reason`：
   - 说明系统已先尝试合同/项目口径，但合同台账当前不能直接回答
   - 宿主必须明确说明“以下已回退到财务账/流水口径”
   - 不得把后续金额继续包装成合同台账原生答案
8. 若人类明确要求高级维护能力，再通过直接 CLI / shell 工具调用，不要默认把这些能力说明注入老板问答上下文。
9. 若响应里带有 `data.tax_inclusion` / `data.tax_inclusion_note`：
   - 宿主摘要时必须保留这条税口径提示
   - 不得把序时账汇总金额擅自改写成“不含税”“税后利润”或“已剔税”
   - 如果要对老板做一段自然语言总结，至少补一句“该经营口径来自序时账汇总，默认未剔税，通常按含税理解”
10. `bridge_meta.capabilities.tax_disclosure=true` 时，表示 bridge 已显式暴露税口径提示；宿主应优先消费结构化字段，不要回退到正则抽取自然语言。
11. `bridge_meta.capabilities.boss_reply=true` 时，优先消费 `boss_reply`，不要自己从 `message` / `executed_sql` / `calculation_logs` 重拼老板口径。
12. `bridge_meta.capabilities.contract_summary=true` 时，合同类和合同汇总类问题优先消费 `host_summary_contract`。
13. `bridge_meta.capabilities.supplier_payment_summary=true` 时，供应商付款问题优先消费 `host_summary_supplier_payments`，不要把被剔除的员工、内部往来、税费、手续费对象重新算回去。
14. `bridge_meta.capabilities.route_decision=true` 时，宿主必须保留 `data.route_decision` 和 `probe_results`，但老板可见回复里只解释为“已先探测合同/项目表覆盖情况”或“已按银行流水口径回答”。

## 12. Agent 返回规范（必须透出中间过程）

给老板回复时建议“双层输出”：

1. 业务层：结论 + 必要时的现金/差异解释（用老板语言，不说术语）。
2. 技术层：`executed_sql` + `calculation_logs` + `trace`（可折叠，但必须保留在接口结果中；若有证据等级、规则链路等字段，也应一并保留）。

推荐响应示例：

```json
{
  "answer": "先说结果：2月公司先看现金口径，再看经营口径。经营口径下利润按收入减成本及费用并加回营业外收支计算；若当前结果来自序时账汇总，还会同步提示该金额默认未剔税、通常应按含税口径理解。若你要继续核对银行卡实际收付或解释利润和现金差异，我会再把到账、付款和历史回款拆开给你看。",
  "method": "sql",
  "trace": {
    "executed_sql": ["..."],
    "calculation_logs": ["..."]
  },
  "raw": {
    "success": true,
    "answer_method": "sql",
    "data": {"...": "..."}
  }
}
```

## 12.1 老板回复风格（强制）

禁止只返回一个数字，必须按以下结构输出：

1. 一句话结论：先回答老板最关心的结果（金额 + 时间）。
2. 业务解释：老板问汇总时先说明“合同/项目汇总口径”；只有合同汇总答不全时，再切到“银行卡上看/实际收付 + 经营确认”；继续追问差异时再展开利润桥、税项时差和预提/冲回影响。
3. 如果经营口径来自序时账汇总，必须补一句含税说明；不要把序时账金额默认为不含税。
4. 管理动作：给 1-2 条可执行建议（催收、控费、回款跟进、税务检查等）。
5. 过程可追溯：接口里保留 `executed_sql` / `calculation_logs`，但对老板默认折叠展示。

## 12.2 宿主消费 `tax_inclusion` 规则

宿主在解析 `finance-query` 返回时，对税口径字段必须按下面顺序处理：

1. 先读 `data.tax_inclusion`
2. 再读 `data.tax_inclusion_note`
3. 再看 `data.exposed_fields.tax_inclusion` / `data.exposed_fields.tax_inclusion_note`
4. 最后才退回 `message` 或 `boss_reply["原因"]` 里的自然语言提示

当前已定义语义：

1. `journal_entry_gross_amount_default`
   - 表示经营口径来自序时账汇总
   - 默认按凭证入账金额统计
   - 不主动剔税
   - 若税额未单独拆分，通常按含税口径理解

宿主禁止动作：

1. 看到 `account_value` 就直接说“不含税利润”
2. 因为 `metric=利润` 就默认它是税后结果
3. 丢掉 `tax_inclusion_note` 后只保留金额
4. 用自己总结的话覆盖 `tax_inclusion_note` 原意

额外强制要求：

1. 默认写成“老板汇报风格”，不要写成审计报告或会计教材。
2. 多用老板听得懂的话：
   - `银行卡上看`
   - `账上看`
   - `实际到手`
   - `实际花出去`
   - `历史欠款回来了`
3. 少直接丢术语：
   - 少说 `权责发生制`、`现金口径`、`预提`、`递延`
   - 如果必须说，后面马上翻译成人话
4. 不要只解释“是什么”，还要顺手回答“老板接下来该盯什么”。
5. 对不确定内容直接说：
   - `目前库里看不出来`
   - `这笔还需要补结算单/开票记录/合同台账确认`
   - 不要猜，不要编月份，不要编业务性质
6. 如果结果不好看，也要直接说清楚，但语气要稳，不要制造惊慌。
7. 金额尽量让老板一眼看懂：
   - 大数优先说“约多少万”，必要时再补精确元数
   - 同一句里不要堆太多小数

推荐话术模板：

1. `结论：{时间}先看银行卡上，实际到账{A}、实际花出{B}、净增加{C}。`
2. `再补经营口径：确认收入{D}、成本{E}、利润{F}。`
3. `建议：本周优先盯{客户/项目}回款，同时控制{费用项}，避免下月利润波动。`

推荐翻译规则：

1. 不说：`钱口径/账口径`
   统一改成：`银行卡上看/账上看`
2. 不说：`预提导致利润为负`
   统一改成：`有些本该算在前一个月的成本，这个月才补进账上，所以账面看起来偏低`
3. 不说：`销项税额导致差异`
   统一改成：`这里的差额主要是税，不是业务少赚了`
4. 不说：`应收回款冲减`
   统一改成：`这是以前欠着的钱，这个月回来了`

## 13. 常用调用示例

```bash
# 查询
./financeqa query --company "南京优集数据科技有限公司" "2026年2月收入/成本/利润分别是多少"

# 合同维度查询
./financeqa query --company "南京优集数据科技有限公司" "辽宁金程信息科技有限公司2025年合同结算多少？其中10月到账多少？"

# 主动获取上层 Agent 数据包
./financeqa host-data --company "南京优集数据科技有限公司" --from 2026-02 --to 2026-02 "请判断该月利润异常原因"

# 单文件导入
./financeqa import /path/to/report.xls
```

## 14. 财务统计基本原则（不可违反）

本节列出实际犯过的错误与正确做法。遇到财务查询时，先过这四条。

### 12.1 费用 ≠ 银行流水对手方

**反例：** 用户问”各大客户销售额”，AI 从 `bank_statement.counterparty_name` 按流入金额排序，把”南京林悦智能科技有限公司”（实为供应商）列为第一大客户，把”吴零”（实为员工）列为第二大客户。

**正确做法：** 客户/供应商身份以序时账（`journal`）收入/成本科目摘要和发票凭证为准。银行流水只反映资金进出，不定义业务关系。查客户销售额应查 `journal` 中 `6001%`（主营业务收入）科目的贷方分录，从凭证摘要中提取客户名称。

### 12.2 只取费用科目，不叠加负债科目

**反例：** 用户问”人力成本多少”，AI 同时取了 `660219（管理费用-福利费）借方 21,974` 和 `221104（应付职工薪酬-福利费）借方 21,974` 相加得到 44,954，声称福利费占人力成本 41%。实际这笔是同一分录的两面，福利费就是 21,974。

**正确做法：** 查”花了多少钱”只看 6 开头费用/成本科目的借方发生额。2 开头的负债科目（2211 应付职工薪酬、2202 应付账款等）记录的是”欠了/计提了”，不是”多花了”。永远不要把 6xxx 和 2xxx 的同笔业务金额相加。

### 12.3 借贷对称分录只算一面

**反例：** 同一笔报销在序时账中有两行——“借：管理费用-福利费 14,147”和”贷：应付职工薪酬-福利费 14,147”。AI 把借方所有金额不管科目全加一遍，相当于把每笔业务算了两遍。

**正确做法：** 按查询目的选定一个方向：
- 查”花了多少”→ 费用科目（6xxx）借方
- 查”收入多少”→ 收入科目（6001%）贷方
- 查”付了多少”→ 银行流水 `debit_amount`（实际支付）
- 查”欠了多少”→ 负债科目（2xxx）贷方余额

### 12.4 实体身份先确认后使用

**反例：** AI 把”林悦”直接称为客户，把”吴零”直接列为销售对手方，但林悦实为供应商、吴零实为员工。

**正确做法：** `dimension_members` 中只有会计科目代码，没有客户/供应商/员工的身份档案。实体身份必须从序时账和交易记录中实时推断：
1. 凭证摘要模式：`journal.summary` 中”为XX服务”→ 客户；”收到XX发票”/”转账XX”/”预提成本_XX”→ 供应商；”XX报销”/”发放工资”→ 员工
2. 科目性质：对方出现在 2211（应付职工薪酬）→ 员工；出现在 2202（应付账款）/6401（营业成本）→ 供应商；出现在 1122（应收账款）/6001（收入）→ 客户
3. 银行流水辅助：`bank_statement.counterparty_name` 可作为补充线索，但不能作为唯一判断依据——同一家公司可能既是供应商又是客户

### 12.5 预收/跨期收入要单独核对

**反例：** 对比“账上收入 vs 银行到账”时，AI 只按客户名匹配当月银行流水，得出“金程回款多了、京信没回”，但漏了 YIPIT 的 87.9 万——这笔钱 2 月通过美元汇款已经到账，当时记在“预收账款（2203）”，3 月开票才转入收入。因为 bank_statement 里 YIPIT 的对手方是“南京优集数据科技有限公司”（美元换汇），摘要写的是 `USDCNY:6.8686`，没有 YIPIT 的名字，所以被漏掉了。

**正确做法：** 对比“账上收入 vs 银行到账”或分析“收入差异原因”时，除了按客户名匹配银行流水，还要额外查：
1. **预收账款（2203）**：是否有前期到账本月开票转收入的记录。
2. **应收账款（1122）**：是否有本月新确认应收但未回款的记录。
3. **美元/外币**：可能走换汇通道，bank_statement 对手方可能不是客户名，要从 journal 的贷方收入明细逐条扫。

### 12.6 差异归因与字段边界

**反例：** 把供应商付款说成收入差异，把税额差异说成业务差异，或者在没有字段支持时，硬说某笔款是“某个月的结算款”。

**正确做法：**
1. 供应商付款、工资、还款、税费等引起的差异，要先按对应业务类型归因，不能直接归到收入差异。
2. 销项税、进项税、应交税费等差异，优先解释为税务口径或申报/入账时点差异，不能直接解释成业务量变化。
3. 没有月份、结算周期、合同、发票或凭证摘要等字段支撑时，只能说“待核实”或“疑似”，不能编造某笔款的月份归属或结算性质。
4. 对不能证实的归因，必须同时说明缺失的字段是什么，以及下一步该查什么。

## 15. 集成注意事项

1. 不要把”收入”直接等同”银行到账”。
2. 不要把”成本”直接等同”银行支出”。
3. 涉及核心指标，老板问汇总时先查合同/项目汇总，答不全再给现金收付 + 经营确认；涉及明确主体时优先返回主体审计结果，不强行改成整月现金和经营汇总。
4. 缺数据时必须返回 `llm_payload` 或明确缺口，不可编造。
5. 供应商相关回答要返回具体名单（`data.suppliers`），不能只给总数。
6. 问”今年/本月/上个月”时，账期按数据库最新凭证日期自动锚定，不按自然月盲算。
7. 公司名称支持简称/别名智能匹配，对接层不要自行裁剪公司名再传入。
8. 主体身份是按当前问题和证据实时判断的，同一家公司可能既是客户也是供应商。
9. 高频问法关键词支持配置化调整，尤其是人力成本、税、经营状态、整体支出这几类常见问法。
10. **回答老板前，过一遍第12节原则，确认没有犯反例中的错误。**

## 16. 硬性红线（必须遵守）

1. 不能把“银行卡到账”直接当“当月收入确认”。
2. 不能把供应商付款、工资、税费、借款还款直接解释为收入差异。
3. 不能把 `6xxx` 费用科目与 `2xxx` 负债科目同笔金额重复相加。
4. 不能仅靠银行对手方名称认定客户/供应商身份，必须结合序时账证据。
5. 不能在字段不足时编造“结算月份/合同归属/开票归属”，必须明确“待核实”。
6. 不能只返回结果数字，必须保留 `executed_sql`、`calculation_logs`、`trace`、`route_decision`。
7. 不能因 CLI exit code 非 0 直接判失败并丢弃 stdout JSON，必须先解析 stdout。
8. 不能在老板可见回复中输出 `id`、`contract_id`、`account_code`、`source_report_type`、`source_sheet_name`、SQL、trace 字段名等数据库辅助字段；必须翻译成来源 Excel、合同/项目、会计科目含义等财务概念。
9. 不能把 `route_decision` / `probe_results` 原样贴给老板；它们只用于宿主判断口径优先级和回退原因。
10. 不能在桥接层重复注入 `SKILL.md`，避免上下文膨胀；skill 由宿主 skill 机制统一加载。

---

若文档与程序返回结果冲突，以实际接口返回字段为准。
