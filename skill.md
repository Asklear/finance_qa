---
name: "finance_qa_engine"
description: "面向老板问答的财务查询插件（双口径 + 可追溯过程 + 宿主LLM兜底）"
version: "1.2.0"
---

# finance_qa OpenClaw 接入手册（全功能暴露）

本文档目标：把本代码库所有已实现功能与接口完整暴露，便于 OpenClaw 直接调用。

## 1. 插件定位

`finance_qa` 是老板财务助理引擎，能力分为四层：

1. 数据层：初始化库、导入报表、目录同步。
2. 规则层：自然语言意图识别、实体识别、账期识别。
3. 计算层：双视角核算（银行卡实际进出账 + 财务报表确认）、税额、应收应付、项目收支等。
4. 兜底层：输出 `llm_payload` 全量财报上下文给宿主 LLM 做最终判别。

## 2. 运行模式与过程暴露

### 2.1 模式开关（代码实际行为）

1. 正式模式：`APP_ENV=production`。
2. 测试模式：默认即测试模式（未设置 `APP_ENV=production`）。
3. 本地兜底：代码中还会读取 `skill.md` 是否包含 `当前运行模式：【正式版本】` 文案作为备用判断。

### 2.2 过程字段暴露约定（必须对外返回）

所有查询响应都必须暴露以下字段，不可只返回结果值：

1. `success`
2. `message`
3. `answer_method`
4. `data`
5. `executed_sql`
6. `calculation_logs`
7. `data.trace.executed_sql`
8. `data.trace.calculation_logs`

说明：`withTraceData()` 会在缺失时自动补默认 trace，确保宿主可稳定读取中间过程。

## 3. 对外接口总览（全量）

## 3.1 CLI 命令接口（`cmd/financeqa/main.go`）

1. `help | -h | --help`
2. `init-db`
3. `config show`
4. `keywords intents`
5. `query`
6. `import`
7. `sync`
8. `host-data`
9. `dimensions`（含完整子命令，见 3.2）

补充：当首个参数不是上述命令时，CLI 会按 `query` 处理。

## 3.2 dimensions 子命令全量接口

1. `dimensions list [--db <path>]`
2. `dimensions add-dimension --db <path> --code <code> --name <name> [--type <type>] [--hierarchical]`
3. `dimensions add-member --db <path> --dimension <code> --code <code> --name <name>`
4. `dimensions mapping-stats [--db <path>] [--company <name>]`
5. `dimensions seed-standard [--db <path>] --company <name>`
6. `dimensions export-package --db <path> --output <file> [--format json]`
7. `dimensions import-dimensions --db <path> --file <file> [--validate-only] [--skip-existing] [--update-existing] [--format json]`
8. `dimensions import-members --db <path> --dimension <code> --file <file> [--validate-only] [--skip-existing] [--update-existing] [--format json]`
9. `dimensions import-rules --db <path> --file <file> [--company <name>] [--validate-only] [--skip-existing] [--update-existing] [--format json]`
10. `dimensions preview-import --db <path> --type <dimensions|members> --file <file> [--dimension <code>] [--format json]`

## 3.3 Go SDK 接口（可供宿主服务封装）

1. `query.NewEngine(dbPath, company)`
2. `(*Engine).Query(question)`
3. `(*Engine).HostLLMPayload(from, to, question)`
4. `(*Engine).Close()`

## 4. 查询响应契约（OpenClaw 必须按此解析）

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
2. `llm_payload`: 插件无法直接准确回答，转交宿主 LLM 基于全量数据推理。

## 4.3 失败兜底结构（`success=false` 常见字段）

1. `data.fallback_attempted`
2. `data.hint`
3. `data.available_accounts`
4. `data.counterparty_sample`
5. `data.llm_payload`

## 5. 意图识别与处理器映射（代码实装）

`ClassifyIntent(question)` 输出意图后分流：

1. `host_payload` -> `queryHostLLMPayload`
2. `identity` -> `detectEntityRole`
3. `arap` -> `queryARAP`
4. `large_transaction` -> `queryLargeBankTransactions`
5. `tax` -> `queryTax`
6. `monthly_summary` -> `queryMonthlySummary`
7. `analysis` -> `queryAnalysis`
8. `fallback` -> `queryFallback`
9. 其他 -> `queryPrecise`

此外：
1. 若命中核心指标（收入/成本/利润/销售额）且不属于排除场景，会强制走 `queryDualPerspectiveForCoreMetric`。
2. 若精确查询失败且存在实体，会自动降级到 fallback 路径。

## 6. 已支持问题能力清单（老板问法）

以下问题均有代码路径支撑，且会返回中间过程：

1. 月度收入/成本/利润（双视角：实际进出账 + 报表确认）。
2. 某客户/供应商/主体在某期间金额（穿透审计）。
3. 这个月整体支出。
4. 人力成本。
5. 供应商数量 + 供应商名单与净流出。
6. 某实体某月数据是否已出。
7. 某项目某月收入。
8. 某项目某月成本。
9. 某月销项税额。
10. 某月进项税额。
11. 某月总成本（走月度总结或双视角成本）。
12. 某月应收账款（余额表口径）。
13. 某月应付账款（余额表口径）。
14. 某项目应收/应付（项目净流入口径）。
15. 某主体身份识别（客户/供应商/员工/未知）。
16. 某期间最大流入对手方/大额流水查询。
17. 某科目期末余额精确查询（如“货币资金余额是多少”）。
18. 账龄与健康度分析（应收/应付账龄桶与健康评分）。

## 7. 双视角强制策略（核心指标）

当问题涉及 `收入/成本/利润/销售额`，默认返回两套口径：

1. 老板可理解表达：`卡上实际进出账`（对应内部 `money_view` / `money_value`）。
2. 老板可理解表达：`财务报表确认`（对应内部 `account_view` / `account_value`）。

并同步提供兼容字段：

1. `现金流入`
2. `现金流出`
3. `净现金流`
4. `财务做账口径(看利润)`

## 8. 宿主 LLM 兜底接口（不可直接调宿主模型时）

代码内不直接调用宿主 LLM，改为提供全量数据接口：

1. `query` 自动 fallback 时返回 `data.llm_payload`。
2. 可主动调用 `host-data` 直接获取 `llm_payload`。

`llm_payload` 内容：

1. `question`
2. `company`
3. `period`
4. `financial_tables.balance_sheet`
5. `financial_tables.income_statement`
6. `financial_tables.balance_detail`
7. `financial_tables.journal`
8. `financial_tables.bank_statement`
9. `trace.intent`
10. `trace.strategy`

## 9. OpenClaw 推荐工具封装（对接层）

建议在 OpenClaw 暴露以下工具名（桥接到 CLI）：

1. `finance-query`
2. `finance-host-data`
3. `finance-import`
4. `finance-sync`
5. `finance-dimensions`

最小调用策略：

1. 先调 `finance-query`。
2. 若 `success=true`，直接回复并附中间过程字段。
3. 若 `success=false` 或 `answer_method=llm_payload`，读取 `data.llm_payload` 交宿主推理。

## 9.1 关键实现差异（必须注意）

`financeqa query` 的 CLI 行为是：

1. 成功：stdout 输出完整 JSON，exit code=0。
2. 失败：仅 stderr 输出 `message`，exit code=1（不会输出 JSON 结构）。

这意味着如果桥接层只依赖 CLI stdout，会丢失 `llm_payload/trace`。

推荐做法：

1. 优先在桥接层直接用 Go SDK（`Engine.Query`）拿结构化 `Result`。
2. 若必须走 CLI，建议桥接层对失败场景二次调用 `host-data`，至少保证有全量 `llm_payload`。
3. 桥接层对外接口要“统一输出JSON”，不要把 CLI 的非0退出直接透传给老板。

## 10. OpenClaw 返回规范（必须透出中间过程）

给老板回复时建议“双层输出”：

1. 业务层：结论 + 双视角解释（用老板语言，不说术语）。
2. 技术层：`executed_sql` + `calculation_logs` + `trace`（可折叠，但必须保留在接口结果中）。

推荐响应示例：

```json
{
  "answer": "2月公司卡上实际到账约180万，实际付出约120万，手里净增加约60万；财务报表确认收入约165万、确认成本约130万、账面利润约35万。两边有差异，主要是部分成本在下月才入账。",
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

## 10.1 老板回复风格（强制）

禁止只返回一个数字，必须按以下结构输出：

1. 一句话结论：先回答老板最关心的结果（金额 + 时间）。
2. 业务解释：用“卡上实际进出账”和“财务报表确认”解释差异。
3. 管理动作：给 1-2 条可执行建议（催收、控费、回款跟进、税务检查等）。
4. 过程可追溯：接口里保留 `executed_sql` / `calculation_logs`，但对老板默认折叠展示。

推荐话术模板：

1. `结论：{时间}公司实际到手{A}，实际花出{B}，净增加{C}。`
2. `报表上确认收入{D}、成本{E}、利润{F}，和实际到手有差异，主要因为{原因}。`
3. `建议：本周优先盯{客户/项目}回款，同时控制{费用项}，避免下月利润波动。`

## 11. 常用调用示例

```bash
# 查询
./financeqa query --db finance.db --company "南京优集数据科技有限公司" "2026年2月收入/成本/利润分别是多少"

# 主动获取宿主LLM数据包
./financeqa host-data --db finance.db --company "南京优集数据科技有限公司" --from 2026-02 --to 2026-02 "请判断该月利润异常原因"

# 单文件导入
./financeqa import --db finance.db /path/to/report.xls

# 目录同步
./financeqa sync --db finance.db /path/to/reports

# 查看维度
./financeqa dimensions list --db finance.db
```

## 12. 集成注意事项

1. 不要把“收入”直接等同“银行到账”。
2. 不要把“成本”直接等同“银行支出”。
3. 涉及核心指标，强制双视角返回。
4. 缺数据时必须返回 `llm_payload` 或明确缺口，不可编造。
5. 供应商相关回答要返回具体名单（`data.suppliers`），不能只给总数。
6. 问“今年/本月/上个月”时，账期按数据库最新凭证日期自动锚定，不按自然月盲算。
7. 公司名称支持简称/别名智能匹配，桥接层不要自行裁剪公司名再传入。

---

若代码与文档冲突，以 `cmd/financeqa/main.go` 与 `internal/query/engine.go` 实际实现为准。
