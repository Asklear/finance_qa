---
name: "finance"
description: "v2.0.0｜老板财务问答核心技能（短上下文高准确版）"
---

# finance_qa 核心调用规范

目标：让 OpenClaw / Claude Code 在最小上下文里稳定调用本仓库能力，并避免财务低级错误。

## 1. 默认上下文

1. 默认公司：`南京优集数据科技有限公司`。
2. 默认优先真实数据：`finance.db`。
3. 默认先走可计算路径（SQL/代码），再做语言归纳。

## 2. 能力入口（完整）

## 2.1 桥接工具（OpenClaw 直接可用）

1. `finance-query` -> `financeqa query`
2. `finance-host-data` -> `financeqa host-data`
3. `finance-upload` -> `financeqa import`（单文件）

## 2.2 直接 CLI/SDK（桥接未封装但必须可用）

1. `financeqa sync`（目录批量导入）
2. `financeqa dimensions ...`（list/add/member/stats/seed/export/import/preview）
3. `financeqa config show`
4. `financeqa keywords intents`
5. Go SDK：`query.NewEngine` / `Engine.Query` / `Engine.HostLLMPayload`

## 3. 返回契约（必须保留）

每次回答都要保留以下结构化字段（不要裁剪）：

1. `success`
2. `message`
3. `answer_method`（`sql` / `llm_payload`）
4. `data`
5. `executed_sql`
6. `calculation_logs`
7. `data.trace.executed_sql`
8. `data.trace.calculation_logs`
9. `data.intent_trace`（router_version/matched/scores/final_intent/confidence）
10. `data.exposed_fields`（dual_perspective/hr_breakdown/arithmetic_checks/intent_trace）
11. `bridge_meta.protocol_version`
12. `bridge_meta.capabilities`

## 4. 调用策略（强制）

1. 先调 `finance-query`。
2. 若 `success=false` 或 `answer_method=llm_payload`，立即调 `finance-host-data` 兜底。
3. 对 CLI 返回，先解析 stdout JSON，再看 exit code。
4. 不能把桥接层当成全能力入口；批量导入和维度能力必须允许直接 CLI 调用。
5. 桥接层不再注入 `SKILL.md`，skill 由宿主 skills 机制加载。

## 5. 核心业务规则（必须遵守）

1. 收入/成本/利润/销售额等核心指标，默认返回“双视角”：
   - `银行卡上看`（实际收付）
   - `账上看`（报表确认）
2. 问题含明确主体且同时问多个核心指标时，仍强制双视角。
3. 主体身份实时识别，可为 `customer/supplier/employee/mixed/unknown`；同一主体可跨角色。
4. 无法证实的归因必须明确“待核实 + 缺什么字段 + 下一步查什么”。

## 6. 绝对红线（不能犯）

1. 不能把“银行到账”直接当“当月收入确认”。
2. 不能把供应商付款/工资/税费/借款还款误归因为收入差异。
3. 不能把同一业务在 `6xxx` 与 `2xxx` 科目重复累计。
4. 不能只靠银行对手方名称判断客户/供应商身份。
5. 不能在证据不足时编造结算月份、合同归属、开票归属。
6. 不能只返回一个数字；必须保留过程字段。
7. 不能因 CLI 非0退出码直接丢弃 stdout JSON。

## 7. 老板回复风格（默认）

1. 一句话结论（金额+时间）。
2. 用老板语言解释差异：`银行卡上看` vs `账上看`。
3. 给 1-2 条可执行动作（盯回款/控成本/查税差/核结算单）。
4. 默认不展开 SQL 细节；用户要求时再展示过程。

## 8. 发布前最小验收

1. `./tests/scripts/run_top20_realdata_check.sh`
2. `/opt/homebrew/bin/go run tests/scripts/prod_audit_regression.go`

## 9. 详细规则附录

完整历史规则、反例、细粒度财务解释见：

1. `docs/SKILL_APPENDIX_FULL_2026-04-15.md`
2. `docs/财务计算逻辑.md`

当核心规则与附录冲突时，以本文件（核心版）为准。
