---
name: "finance"
description: "Use when OpenClaw or Claude needs finance_qa to answer老板财务问题、读取结构化财务结果，或在不能直接精算时切到宿主LLM兜底。"
---

# finance_qa 宿主问答契约

目标：让 OpenClaw / Claude / 宿主 Agent 用最少上下文稳定调用本仓库能力，直接回答老板问题；研发测试、部署、验收和全量运维命令不放在主 skill 里，避免污染宿主问答上下文。

## 0. 契约版本

1. `skill_contract_version`: `2026-04-21.2`
2. `bridge_protocol_version`: `v2`
3. 按需附录：`docs/SKILL_APPENDIX_FULL.md`

## 1. 运行前提

1. 默认公司：`南京优集数据科技有限公司`
2. 默认数据库优先级：
   - `FINANCEQA_DB`
   - `FINANCEQA_PG_DSN`
   - `PGHOST/PGPORT/PGUSER/PGPASSWORD/PGDATABASE/FINANCEQA_PG_SCHEMA`
3. 未配置数据库时，CLI/桥接层会明确报错，不再回退本地 `finance.db`
4. 桥接默认二进制：`/root/finance_qa/financeqa`
5. 桥接层会自动加载：
   - 当前目录 `.env`
   - `/root/finance_qa/.env`
6. OpenClaw 桥接返回的是 `content[0].text`，宿主必须先把这段文本再解析成 JSON

## 2. 宿主优先使用的工具

1. `finance-query`
   - 老板财务问答主入口
   - 参数：
```json
{"query":"2026年3月收入、成本、利润分别是多少？"}
```

2. `finance-host-data`
   - 当 `finance-query` 不能稳定直答时，输出宿主 LLM 兜底数据包
   - 参数：
```json
{"query":"为什么3月利润和现金差这么大","from":"2026-03","to":"2026-03"}
```

3. `finance-upload`
   - 单文件导入财务报表时使用
   - 参数：
```json
{"filePath":"/abs/path/report.xlsx"}
```

说明：

1. 主 skill 面向宿主问答，不再展开测试、部署、回归、维度维护、批量同步等研发/运维指令。
2. 如果人类操作者明确要求维护能力，再看 `README.md`、CLI `--help` 或附录，不要默认把这些内容注入老板问答上下文。

## 3. 宿主结果消费顺序

1. 先解析桥接返回的 `content[0].text` JSON。
2. 若存在 `boss_reply`，优先直接使用：
   - `结论`
   - `原因`
   - `建议`
3. 若存在 `host_summary_contract`，宿主摘要必须受它约束，不能脱离结构化字段自行重算。
4. 若没有 `boss_reply`，再退回 `message`。
5. 无论对老板是否展示，都要保留：
   - `success`
   - `answer_method`
   - `data`
   - `executed_sql`
   - `calculation_logs`
   - `data.trace`
   - `data.intent_trace`
   - `bridge_meta`
6. 若 `success=false` 或 `answer_method=llm_payload`：
   - 立即调用 `finance-host-data`
   - 让宿主 LLM 基于 `data.llm_payload` 继续判断

## 4. 宿主不能自己改写的结构化约束

1. `boss_reply` 是后端已整理好的老板口径，不要再从 `executed_sql`、`calculation_logs`、`evidence` 里二次拼数。
2. `host_summary_contract` 出现时，必须按其字段回答，不允许自行改写成别的时间口径。
3. 对“累计回款 + 子期间到账”类问题：
   - `total_amount` 是累计值
   - `sub_period_amount` 是子期间值
   - 不能把 `sub_period_receipts` 当累计值
   - 不能把“其中 3 月到账”改写成“全部在 3 月到账”
4. CLI 非 0 退出码不等于没有结果：
   - 必须优先解析 `stdout` JSON
   - 再看 exit code

## 5. 老板问答规则

1. 核心经营指标：
   - 收入 / 成本 / 利润 / 销售额默认先给现金收付，再补经营确认
   - 回答顺序保持“先现金、再经营”
2. 差异原因：
   - 只有在用户追问“为什么不一样/差额是什么造成”时，再展开利润调现金桥、回款时点和成本确认时差
3. 明确主体问题：
   - 如果问题明确点名客户 / 供应商 / 员工 / 项目 / 分公司，优先回答主体审计结果
   - 不要强行改成整月汇总
4. 人力成本：
   - 要看 `hr_breakdown`
   - 至少覆盖工资、社保、公积金
   - 分公司内部转账要单列解释，不能静默并入别的科目
5. 应收 / 应付：
   - 以开放项配对和余额逻辑为准
   - 不能只按当月回款/付款机械相减
6. 区间问题：
   - 季度 / 半年 / 全年 / 累计必须按区间聚合
   - 不能把最后一个月的 `current_amount` 当整个区间答案
   - 要做 `current_amount` 和 `cumulative_amount` 的合理性交叉验证
7. 证据不足：
   - 直接说“目前库里还不能硬判”
   - 说明缺什么字段
   - 告诉老板下一步该查什么

## 6. 绝对红线

1. 不能把银行到账直接当当月收入确认
2. 不能把供应商付款、工资、税费、借款还款直接归因为收入差异
3. 不能只靠银行对手方名称判断客户 / 供应商身份
4. 不能在证据不足时编造结算月份、合同归属、开票归属
5. 不能只给一个数字，不保留结构化过程字段
6. 不能因为 CLI 非 0 退出码直接丢弃 `stdout` JSON
7. 不能无视 `host_summary_contract`，自行从日志或证据里重算金额
8. 不能把 `sub_period_receipts` 改写成累计回款
9. 不能把季度 / 半年 / 全年问题偷换成最后一个月结果

## 7. 最小调用示例

### 7.1 直接问答

```bash
./financeqa query --company "南京优集数据科技有限公司" "2026年3月收入、成本、利润分别是多少？"
```

### 7.2 获取宿主兜底数据包

```bash
./financeqa host-data --company "南京优集数据科技有限公司" --from 2026-03 --to 2026-03 "为什么3月利润和现金差这么大"
```

### 7.3 单文件导入

```bash
./financeqa import --company "南京优集数据科技有限公司" /abs/path/report.xlsx
```

## 8. 附录说明

1. 更细的财务规则、问法覆盖和统计原则，按需参考 `docs/SKILL_APPENDIX_FULL.md`
2. 若主 skill 与附录冲突，以主 skill 为准
3. 发布到 Claude / OpenClaw 时，必须保留相对路径 `docs/SKILL_APPENDIX_FULL.md`
