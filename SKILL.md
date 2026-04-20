---
name: "finance"
description: "Use when OpenClaw or Claude needs to call finance_qa for老板财务问答、宿主LLM兜底数据、报表导入、维度管理或财务配置查询。"
---

# finance_qa 调用契约

目标：让 OpenClaw / Claude / 宿主 Agent 用最少上下文稳定调用本仓库能力，直接回答老板财务问题，并在失败时自动切到可审计的兜底数据模式。

## 0. 契约版本

1. `skill_contract_version`: `2026-04-20.1`
2. `bridge_protocol_version`: `v2`

## 1. 运行前提

1. 默认公司：`南京优集数据科技有限公司`
2. 默认数据库：
   - 优先 `FINANCEQA_DB`
   - 其次 `FINANCEQA_PG_DSN`
   - 其次 `PGHOST/PGPORT/PGUSER/PGPASSWORD/PGDATABASE/FINANCEQA_PG_SCHEMA`
3. 未配置数据库时，CLI/桥接层现在会明确报错，不再回退根目录 `finance.db`
4. 桥接默认二进制：`/root/finance_qa/financeqa`
5. 桥接层会自动加载：
   - 当前目录 `.env`
   - `/root/finance_qa/.env`

## 2. 能力地图

### 2.1 OpenClaw 桥接工具

1. `finance-query`
   - 用途：老板财务问答主入口
   - 参数：
```json
{"query":"2026年3月收入是多少"}
```

2. `finance-host-data`
   - 用途：给宿主 LLM 提供原始财务数据包
   - 参数：
```json
{"query":"为什么3月利润和现金差这么大","from":"2026-03","to":"2026-03"}
```

3. `finance-upload`
   - 用途：导入单个 Excel / 报表文件
   - 参数：
```json
{"filePath":"/abs/path/report.xlsx"}
```

### 2.2 直接 CLI

1. `financeqa query [--db <dsn-or-path>] [--company <name>] <question>`
2. `financeqa host-data [--db <dsn-or-path>] [--company <name>] [--from YYYY-MM] [--to YYYY-MM] [question]`
3. `financeqa import [--db <dsn-or-path>] [--incremental] [--company <name>] <file>`
4. `financeqa sync [--db <dsn-or-path>] [--incremental] [--company <name>] <directory>`
5. `financeqa config show [--config <path>]`
6. `financeqa keywords intents [--keywords <path>]`
7. `financeqa dimensions list [--db <dsn-or-path>]`
8. `financeqa dimensions add-dimension --db <dsn-or-path> --code <code> --name <name> --type <type> [--hierarchical]`
9. `financeqa dimensions add-member --db <dsn-or-path> --dimension <code> --code <code> --name <name>`
10. `financeqa dimensions mapping-stats [--db <dsn-or-path>] [--company <name>]`
11. `financeqa dimensions seed-standard [--db <dsn-or-path>] --company <name>`
12. `financeqa dimensions export-package --db <dsn-or-path> --output <file> [--format json]`
13. `financeqa dimensions import-dimensions --db <dsn-or-path> --file <file> [--validate-only] [--skip-existing] [--update-existing]`
14. `financeqa dimensions import-members --db <dsn-or-path> --dimension <code> --file <file> [--validate-only] [--skip-existing] [--update-existing]`
15. `financeqa dimensions import-rules --db <dsn-or-path> --file <file> [--company <name>] [--validate-only] [--skip-existing] [--update-existing]`
16. `financeqa dimensions preview-import --db <dsn-or-path> --type <dimensions|members> --file <file> [--dimension <code>]`

### 2.3 Go SDK

1. `query.NewEngine(dbPath, company)`
2. `(*Engine).Query(question)`
3. `(*Engine).HostLLMPayload(from, to, question)`
4. `(*Engine).Close()`

## 3. 模块接口

### 3.1 问答模块

主入口：

1. 桥接：`finance-query`
2. CLI：`financeqa query`
3. SDK：`Engine.Query`

行为：

1. 成功时：
   - `stdout` 输出完整 JSON
   - exit code = `0`
2. 失败时：
   - `stdout` 仍输出完整 JSON
   - `stderr` 输出错误消息
   - exit code = `1`
3. 这条规则非常重要：
   - 先解析 `stdout` JSON
   - 再看 exit code
   - 不能因为 exit code 非 0 就丢弃 `stdout`

### 3.2 宿主 LLM 数据包模块

主入口：

1. 桥接：`finance-host-data`
2. CLI：`financeqa host-data`
3. SDK：`Engine.HostLLMPayload`

行为：

1. `from/to` 为空时，会自动锚定数据库最新账期
2. 返回 `answer_method = llm_payload`
3. 主要看 `data.llm_payload`

### 3.3 导入模块

1. 单文件导入：
   - 桥接：`finance-upload`
   - CLI：`financeqa import`
   - 返回 `ImportSummary`

2. 目录批量导入：
   - CLI：`financeqa sync`
   - 返回 `SyncSummary`

导入参数：

1. `--incremental`
   - 保留已有数据
   - 走去重 / 增量策略
2. `--company`
   - 覆盖导入文件里的公司名

### 3.4 维度模块

1. 管理主体：`dimensions.Manager`
2. 交换主体：`dimensions.DataExchange`
3. 典型用途：
   - 科目到维度映射
   - 标准规则初始化
   - 维度/成员/规则的导入导出

返回类型：

1. `list` -> `PaginatedResult`
2. `export-package` -> `ExportDataPackage`
3. `import-*` -> `DetailedImportReport`
4. `preview-import` -> `ImportPreview`

### 3.5 配置与规则模块

1. `financeqa config show`
   - 输出 YAML
2. `financeqa keywords intents`
   - 输出意图名称列表，逐行文本
3. 查询规则文件：
   - 默认 `config/rules.json`
   - 可由 `FINANCEQA_RULES_PATH` 覆盖

## 4. 返回契约

### 4.1 问答/host-data 标准字段

每次回答都必须尽量保留这些字段，不要裁掉：

1. `success`
2. `message`
3. `answer_method`
   - `sql`
   - `llm_payload`
4. `data`
5. `executed_sql`
6. `calculation_logs`

### 4.2 `data` 内必须关注的字段

1. `data.answer_method`
2. `data.trace.executed_sql`
3. `data.trace.calculation_logs`
4. `data.process.executed_sql`
5. `data.process.calculation_logs`
6. `data.executed_sql`
7. `data.calculation_logs`
8. `data.intent_trace`
   - `router_version`
   - `matched`
   - `scores`
   - `final_intent`
   - `confidence`
9. `data.exposed_fields`
   - `dual_perspective`
   - `hr_breakdown`
   - `arithmetic_checks`
   - `intent_trace`

### 4.3 桥接层附加字段

桥接层会再补这些：

1. `boss_reply`
   - `结论`
   - `原因`
   - `建议`
2. `bridge_meta`
   - `skill_contract_version`
   - `protocol_version`
   - `generated_at`
   - `query`
   - `company`
   - `db`
   - `capabilities`

注意：

1. OpenClaw 桥接返回的是 `text` 内容
2. `content[0].text` 本身是一段 JSON 字符串
3. 宿主必须再做一次 JSON 解析

## 5. 调用决策

### 5.1 OpenClaw / Claude 默认流程

1. 先调 `finance-query`
2. 若 `success=true` 且 `answer_method=sql`
   - 直接用结果回答老板
3. 若 `success=false` 或 `answer_method=llm_payload`
   - 立即切 `finance-host-data`
   - 用 `data.llm_payload` 交给宿主 LLM 继续归纳

### 5.2 桥接层自动 fallback

`finance-query` 在桥接层里已经内置降级：

1. 如果底层 `financeqa query` exit code 非 0
2. 桥接会自动再调用一次 `financeqa host-data`
3. 然后返回：
   - `answer_method = llm_payload`
   - `data.fallback_attempted = true`
   - `data.llm_payload = ...`

但宿主仍应按上面的标准流程检查结果，不要假设每次都是直接可答。

## 6. 老板问题的回答规则

1. 默认先走可计算路径，再做语言归纳
2. 收入 / 成本 / 利润 / 销售额等核心经营指标，默认按财务确认口径回答
   - 优先返回 `account_value` / `metrics`
   - 不默认展开 `银行卡上看` / `账上看` 两套结论
3. 只有问题明确提到 `银行卡` / `到账` / `回款` / `付款` / `现金流` / `差异原因` 时，才展开现金视角或差异解释
4. 人力成本问题，要关注：
   - `hr_breakdown`
   - `工资`
   - `社保`
   - `公积金`
   - 分公司内部转账应作为单列信息解释
5. 应收应付问题，要以开放项配对逻辑为准，不能只按当月回款/付款机械相减
6. 证据不足时必须明确：
   - 还不能硬判
   - 缺什么字段
   - 下一步该查什么

## 7. 绝对红线

1. 不能把银行到账直接当当月收入确认
2. 不能把供应商付款 / 工资 / 税费 / 借款还款误归因为收入差异
3. 不能只靠银行对手方名称判断客户 / 供应商身份
4. 不能在证据不足时编造结算月份、合同归属、开票归属
5. 不能只返回一个数字，不带过程字段
6. 不能因为 CLI 非 0 退出码直接丢弃 `stdout` JSON
7. 不能把桥接工具当成全部能力入口，批量导入和维度管理需要允许直连 CLI

## 8. 最小示例

### 8.1 直接问答

```bash
./financeqa query --company "南京优集数据科技有限公司" "2026年3月收入是多少"
```

### 8.2 获取宿主 LLM 数据包

```bash
./financeqa host-data --company "南京优集数据科技有限公司" --from 2026-03 --to 2026-03 "为什么3月利润和现金差这么大"
```

### 8.3 单文件导入

```bash
./financeqa import --company "南京优集数据科技有限公司" /abs/path/report.xlsx
```

### 8.4 批量导入

```bash
./financeqa sync --incremental /abs/path/reports/
```

### 8.5 维度导出

```bash
./financeqa dimensions export-package --output /tmp/dimensions.json
```

## 9. 验收

默认回归：

1. `/opt/homebrew/bin/go test ./...`

真实数据库回归：

1. `./tests/scripts/run_top20_realdata_check.sh`
2. `./tests/scripts/run_user19_realdata_check.sh`
3. `/opt/homebrew/bin/go run -tags scriptmain tests/scripts/prod_audit_regression.go`
4. 如需运行 live DB 集成测试，显式设置：
   - `FINANCEQA_RUN_LIVE_DB_TESTS=1`

## 10. 附录

1. 完整历史规则与财务说明：`docs/SKILL_APPENDIX_FULL_2026-04-15.md`
2. 若本文件与附录冲突，以本文件为准
3. 发布到 Claude / OpenClaw 时，必须保留相对路径 `docs/SKILL_APPENDIX_FULL_2026-04-15.md`
