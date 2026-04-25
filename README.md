# Finance QA - Go 版本核心架构

`finance_qa` 是一个旨在替代传统手工处理，打通底层序时账（Journal）与宏观财务报表映射链条的智能化查询引擎系统。
本项目从原先的 Node.js/TypeScript 重构为了 **Go** 版本，显著提升了并发解析稳定性及数据处理速度。

## 一、核心特色能力

### 1. 老板口径优先的数据血缘
系统既保留底层“序时帐 / 银行流水 / 科目余额 / 利润表 / 资产负债表”的可追溯证据，也优先使用财务专家按老板意图整理后的合同维度台账：
- **老板口径主源**：收入、销售额、营收、成本、利润、客户、供应商、项目、合同类问题，优先探测 `fin_contracts + fin_fund_income + fin_cost_settlements`。
- **底层财务证据**：当合同/专家表无法覆盖问题时，默认先说明合同口径缺口；只有用户显式要求账上或银行流水口径时，才查序时帐、科目余额、利润表、资产负债表或银行流水。
- **全格式兼容**：内建 `Python/xlrd` 回落机制，支持老式用友/金蝶产生的 OLE2 腐败文件，并实现了**全自动多页签 (Multi-sheet) 遍历解析**。
- **智能期间识别**：支持复合期间报表（如 `2026.01-2026.02`）的无损提取与分录重组。
- **财务演算引擎**：`calculator.go` 自动利用财务准则聚沙成塔，可复核科目余额表与利润逻辑。

### 2. 老板口径优先 + 双视角兜底
系统不再把所有核心指标默认拆成“现金/财务账”两套口径。当前顺序是：
* **合同/专家表口径(老板默认口径)**：优先回答收入、销售额、营收、成本、利润、客户、供应商、项目和合同问题，并返回来源 Excel 说明。
* **业务现金流口径(明确问钱/到账/付款/银行卡时优先)**：直查银行流水，回答实际到账、实际支出、回款、付款、净增加等问题。
* **财务做账口径(经营/财务补充或兜底)**：严守权责发生制，用序时帐、利润表、科目余额等解释经营确认、应收应付、税额和利润差异。

## 核心能力

- **三位一体身份核验 (Trinity Identity Detector)**：系统不再盲目识别动词，而是通过**银行现金流向（In/Out）**、**会计科目归属（AR/AP）**及**税务特征（进项/销项）**三位一体交叉核验，自动锁定实体身为“客户”、“供应商”或“项目”。
- **Intent Router V2（可解释路由）**：查询入口统一走 V2 路由，返回 `intent_trace`（命中规则、得分、最终意图、置信度），支持冲突裁决和最小置信度回退。
- **老板问题改写与轻量探测**：先把自然语言问题改写为期间、指标、实体、粒度、口径等意图槽位，再利用表/字段注释和少量真实数据探测，判断合同/专家表是否能直接回答。
- **数据库辅助识别 (DB-Assisted Recognition)**：集成动态回溯算法，解决口语化提问（如“飞未云科多少钱”）中由于缺少后缀、动词导致的解析难题。
- **审计穿透挖掘 (Summary Penetration)**：自动扫描序时账摘要字段，提取银行流水中缺失的往来单位信息。
- **自动日期锚定 (Dynamic Anchoring)**：智能识别数据库最新业务月份，确保模糊时间查询（如“今年”、“本月”）准确命中。
- **资产负债审计**：实时计算任意日期的科目余额与资产负债表勾稽关系。
- **零配置执行**：支持**项目根目录自动探测**（基于 `go.mod` 自动寻址），确保系统始终准确命中真相源。

## 二、运行与测试指南

### 1. 环境依赖
* **开发环境**：`Go >= 1.20`
* **底层库依赖（可选）**：`Python3 + xlrd` 仅在解析极老旧 XLS 文件时作为回退路径；常规场景可不安装。
* **数据库**：默认使用 PostgreSQL（通过 `.env` 或环境变量配置）；仅在显式传入 SQLite 路径时才走本地 SQLite 兼容模式。

### 1.1 账密管理（推荐 `.env`）

项目已支持自动加载 `.env`（不会覆盖已有环境变量）：

1. 复制 `.env.example` 为 `.env`
2. 填写 `PGHOST/PGPORT/PGUSER/PGPASSWORD/PGDATABASE`
3. 可选设置 `FINANCEQA_PG_SCHEMA`（默认 `tenant_uhub`）

注意：
1. `.env` 已加入 `.gitignore`，不会提交到仓库。
2. 线上建议把 `.env` 放在 `/root/finance_qa/.env`，并设置权限 `600`。

### 2. 构建与运行
```bash
# 1. 编译系统
go build ./cmd/financeqa/...

# 2. 从本地文件夹全量导入初始化数据库
./financeqa sync "/path/to/exported/excel/files"

# 3. 命令行自然语言查询大体验
./financeqa query --company "你的公司名称" "这个月花了多少钱"
./financeqa query "今年客户收入总和汇总"
```

### 3. 运行测试套件
新重构版本的测试已深度囊括各项业务边界：
```bash
# 执行审计回归报告，一键核验 17 道核心生产审计题 (南京优集实测集)
/opt/homebrew/bin/go run tests/scripts/prod_audit_regression.go

# 执行后端核心模块单测
go test ./internal/accounting/ -v
```

### 4. 规则配置化（统一入口：`config/rules.json`）

系统已支持把关键规则从硬编码抽离出来，便于线上快速调参。

1. 默认规则文件：`config/rules.json`
2. 启用文件覆盖：设置 `FINANCEQA_RULES_PATH`
3. 也可用环境变量直接覆盖

```bash
# 方式 1：加载规则文件
FINANCEQA_RULES_PATH=./config/rules.json ./financeqa query --company "南京优集数据科技有限公司" "2026年2月收入/成本/利润分别是多少"

# 方式 2：直接覆盖 stopwords
FINANCEQA_METRIC_STOPWORDS="收入,成本,利润,经营状况" ./financeqa query --company "南京优集数据科技有限公司" "飞未2月收入多少"
```

当前默认 schema 为 `schema_version = 2`，按词典域组织：

1. `router.stopwords.generic_metric`：泛指标词，避免被误识别成实体。
2. `router.intents.<intent>.keywords`：意图关键词。
3. `router.intents.<intent>.priority` / `min_confidence` / `conflicts` / `high_priority_phrases`：意图优先级与冲突裁决。
4. `router.metric_keywords`：收入 / 成本 / 利润等核心指标关键词。
5. `router.hr_breakdown_keywords`：工资 / 社保 / 公积金等拆分问法关键词。
6. `router.counterparty_classification_question_keywords`：如“成本还是收入”“客户还是供应商”等分类问法。
7. `router.profit_single_view_block_keywords`：出现这些词时，利润问题不走单一账面口径。
8. `router.fallback_monthly_expense_keywords`：整体支出 / 支出汇总类问法关键词。
9. `counterparty.roles`：客户 / 供应商 / 员工角色词典。
10. `counterparty.tax`：销项 / 进项税词典。
11. `counterparty.thresholds`：角色识别阈值。
12. `internal_party.org_suffixes`：内部主体组织后缀词典。
13. `internal_party.account_context_keywords`：内部往来/代发等上下文词典。

兼容说明：

1. 旧的平铺字段仍可继续读取一个过渡周期。
2. 环境变量覆盖顺序不变，仍然会在文件之后生效。
3. 查询层新的关键词规则请统一写入 `config/rules.json`。

支持的环境变量覆盖项：

1. `FINANCEQA_RULES_PATH`
2. `FINANCEQA_METRIC_STOPWORDS`（逗号分隔）
3. `FINANCEQA_ROLE_MIXED_MIN_RATIO`
4. `FINANCEQA_ROLE_MIXED_MIN_POSITIVE_SCORE`
5. `FINANCEQA_ROLE_MIXED_MIN_POSITIVE_ROLES`
6. `FINANCEQA_ROLE_MIN_PRIMARY_SCORE`
7. `FINANCEQA_ROLE_MIN_CONFIDENCE`
8. `FINANCEQA_INTENT_ARAP_KEYWORDS`
9. `FINANCEQA_INTENT_HR_COST_KEYWORDS`
10. `FINANCEQA_INTENT_TAX_KEYWORDS`
11. `FINANCEQA_INTENT_HEALTH_KEYWORDS`
12. `FINANCEQA_INTENT_FALLBACK_KEYWORDS`
13. `FINANCEQA_INTENT_ANALYSIS_KEYWORDS`
14. `FINANCEQA_INTENT_HOST_PAYLOAD_KEYWORDS`
15. `FINANCEQA_INTENT_MONTHLY_SUMMARY_KEYWORDS`
16. `FINANCEQA_FALLBACK_MONTHLY_EXPENSE_KEYWORDS`
17. `FINANCEQA_HIGH_PRIORITY_PHRASES`（JSON 对象）
18. `FINANCEQA_INTENT_PRIORITY`（JSON 对象）
19. `FINANCEQA_INTENT_CONFLICTS`（JSON 对象）
20. `FINANCEQA_INTENT_MIN_CONFIDENCE`（JSON 对象）

仍保留在代码中的内容：

1. 项目 + 指标的组合判断。
2. 开放项配对、置信度和财务证据逻辑。
3. 税额、收入、成本、现金桥等财务语义判断。
4. 内部转账、往来冲销等凭证级推断逻辑。

保留原则：

1. 高频、经常需要线上微调的词放到 `rules.json`。
2. 配置文件只放词表和阈值，组合逻辑与财务语义继续放代码里。
3. 遗留的 `internal/config/keywords_manager.go` 仅保留给旧 `query_keywords.json` 场景，查询主链路不再新增规则到那里。

## 三、代码集成与调用指南 (API / SDK)

除了使用 CLI 之外，本模块被设计为极具解耦性的 Go SDK。你可以非常简单地将其接入到任何现有的 HTTP 服务（如 Gin/Fiber/HTTPMUX）或者更大的 LLM RAG Agent 层中：

```go
package main

import (
	"fmt"
	"encoding/json"
	"financeqa/internal/query"
)

func main() {
	// 1. 实例化查询引擎 (需传入 PostgreSQL DSN 或显式 SQLite 路径，以及默认公司名)
	// 如果公司名传空，引擎会尝试自动从自然语言中提取
	engine, err := query.NewEngine("host=127.0.0.1 port=5432 user=finance password=secret dbname=finance search_path=tenant_uhub,public", "模拟财务")
	if err != nil {
		panic(err)
	}

	// 2. 将自然语言问题直接喂入 query 分析树
	// 解析器会自动执行：实体/期间/指标识别 -> 老板口径改写 -> 数据源轻量探测 -> PostgreSQL 查询或兜底数据包
	res := engine.Query("今年合作伙伴A客户销售额是多少")

	// 3. 处理结果 (res 包含 Success / Message / Data / SQL)
	if res.Success {
		// Data 为灵活的 map[string]any，包含了历史履历(history)和多口径对比
		output, _ := json.MarshalIndent(res.Data, "", "  ")
		fmt.Printf("查询成功: %s\n%s\n", res.Message, string(output))
	} else {
		// 捕捉 Fallback 未接住的意图断层
		fmt.Printf("查询失败: %s\n", res.Message)
	}
}
```

> **系统接入优势**：`engine.Query()` 是线程（Goroutine）安全的，因为底层使用的 `sql.DB` 自带连接池机制，你可以安全地在 Web 服务器的 handler 里面并发调用它。

## 四、 系统架构与模块设计

本项目采用分层解耦的 Go 后端架构，确保了从原始凭证解析到自然语言查询的全链路稳定性。

### 1. 架构图（分三张图）

为了提升可读性，原先“一张大图”已拆为三张独立图：

1. [分层架构图（Layered Architecture）](docs/architecture/01-layered-architecture.md)
2. [查询请求时序图（Query Sequence）](docs/architecture/02-query-sequence.md)
3. [部署与运行图（Deployment & Runtime）](docs/architecture/03-deployment-runtime.md)

阅读建议：
1. 先看分层图，理解系统边界；
2. 再看时序图，理解一次查询如何流转；
3. 最后看部署图，理解线上/本地如何运行与接入。

### 2. 逻辑分层
*   **接入层 (Parser & Ingest)**：处理各版本用友、金蝶及银行导出的 Excel 原始数据。具备自动脱敏、元数据提取（日期/公司识别）及数据清洗能力。
*   **持久层 (DB & Dimensions)**：PostgreSQL 优先，默认使用 `tenant_uhub` 等业务 schema；仅在显式传入 SQLite 路径时走本地兼容模式。采用多维模型（Dimensions）管理财务周期，支持快速切换公司与会计月份。
*   **计算层 (Accounting)**：核心业务大脑。实现了从“序时账”自动平衡“科目余额表”及“利润表”的算法，并支持合同/专家表老板口径、现金流口径、权责发生制口径之间的可解释切换；默认不把非合同口径自动冒充合同口径。
*   **查询层 (Query)**：混合式自然语言引擎。集成业务规则库、正则表达式匹配、Intent Router V2、老板问题改写、数据源能力目录和轻量探测；当规则计算无法稳定回答时，返回 `llm_payload` 给上层 Agent 做最终判别（本仓库不直接调用宿主 LLM）。

### 2. 目录结构
```text
finance_qa/
├── cmd/
│   └── financeqa/          # CLI 工具主入口 (main.go)
├── internal/               # 核心业务逻辑 (不向外部包公开)
│   ├── accounting/         # 财务结算引擎核心 (科目平衡、利润计算、双口径对比)
│   ├── analysis/           # 财务指标分析 (账龄分析、健康度评估)
│   ├── config/             # 配置管理与关键字管理
│   ├── db/                 # 数据库 Schema 管理与初始化
│   ├── dimensions/         # 财务维度建模与仓储模式
│   ├── ingest/             # 数据流水线与同步处理器
│   ├── parser/             # Excel 解析器与元数据自动提取
│   ├── query/              # 自然语言查询引擎 (含词法归一化、Intent Router V2、llm_payload 输出)
│   ├── support/            # 全局路径与工具支持
│   └── types/              # 通用数据结构定义
├── tests/                  # 跨包测试、集成回归、测试数据与测试脚本
│   ├── unit/               # 黑盒/契约型单元测试（按领域归类，不要求与源码同目录）
│   ├── integration/        # 集成/契约/回归测试（跨模块、桥接层、真实题库）
│   ├── testdata/           # 样本库 (已脱敏的典型财务报表样本)
│   ├── scripts/            # 测试与部署辅助脚本
│   └── README.md           # 测试目录放置规范
├── docs/                   # 项目说明文档
└── README.md
```

## 五、 环境与测试

### 1. 环境依赖
*   **Go**: `>= 1.20`
*   **Python3（可选）**: 仅在极老旧 XLS 容错回退场景下需要 `xlrd`。
*   **环境变量**: 默认无需 `OPENAI_API_KEY`；本仓库在兜底时输出 `llm_payload` 供上层 Agent 使用，不直接调用宿主 LLM。

### 2. 运行测试
本项目采用全自动化的集成测试套件，可一键验证重构后的业务逻辑对齐情况：
```bash
# 运行全部 Go 测试（包含 internal 包内测试 + tests 目录测试）
go test ./... -count=1

# 运行包内单元测试（适合改动 internal 逻辑后快速回归）
go test ./internal/... -count=1

# 运行 tests 目录下的黑盒/契约单元测试
go test ./tests/unit/... -count=1

# 运行集成测试 (全量覆盖业务场景)
go test ./tests/integration/... -count=1

# 运行回归检查工具 (自动输出生产提问审计对照表)
/opt/homebrew/bin/go run tests/scripts/prod_audit_regression.go

# 运行 20 道老板高频问题真实数据检查（JSON 题库驱动）
./tests/scripts/run_top20_realdata_check.sh

# 运行用户确认的 19 条问题真实数据检查
./tests/scripts/run_user19_realdata_check.sh
```

真实数据检查脚本的 Markdown 报告默认输出到 `scratch/reports/`，该目录属于本地运行产物，不纳入正式文档入口。

### 3. 测试文件应该放哪里
仓库当前采用混合布局，不要求所有 `*_test.go` 都放进顶层 `tests/`：

- 放在源码旁边：适合测试单个 package 的内部逻辑、未导出辅助函数、SQL 兼容边界。
- 放在 `tests/unit/`：适合黑盒单元测试、规则契约测试、跨多个 package 的轻量级行为验证。
- 放在 `tests/integration/`：适合集成测试、bridge/host 契约测试、真实数据题库回归、线上 smoke。
- 放在 `tests/testdata/`：仅存放测试输入、基准输出和报告样本，不放执行逻辑。
- 放在 `tests/scripts/`：仅存放测试驱动脚本、部署校验脚本和辅助工具。

快速判断标准：

- 如果测试必须和某个 package 紧耦合，优先就地放。
- 如果测试代表“外部调用者视角”，优先放 `tests/unit` 或 `tests/integration`。
- 如果测试会连 PostgreSQL、桥接层、CLI、线上环境，统一放 `tests/integration`。
- 新增测试前，先看 [tests/README.md](/Users/gaorongvc/work/other/finance_qa/tests/README.md) 的放置规范。

## 六、查询结果契约（对接层必读）

`query` 返回统一 JSON，至少包含以下字段：

1. `success`
2. `message`
3. `answer_method`（`sql` 或 `llm_payload`）
4. `data`
5. `executed_sql`
6. `calculation_logs`

`data` 中建议重点消费：

1. `intent_trace.router_version`
2. `intent_trace.matched`
3. `intent_trace.scores`
4. `intent_trace.final_intent`
5. `intent_trace.confidence`
6. `trace.executed_sql`
7. `trace.calculation_logs`

老板核心指标（收入/营收/销售额/成本/利润/客户/供应商/项目/合同）默认按“合同/专家表优先”输出：

1. 先探测 `fin_contracts + fin_fund_income + fin_cost_settlements` 是否能回答老板问题；能回答时，以这些表作为主要口径。
2. 合同/专家表不能覆盖时，默认返回合同口径缺口；只有显式问“银行流水/实际到账/实际支出”或“账上/科目余额/序时账/资产负债表”时，再切到对应口径，不把非合同结果伪装成合同口径。
3. 问题明确包含 `银行`、`银行卡`、`到账`、`实际收款`、`实际支出`、`回款`、`付款` 等现金语义时，优先走银行流水。
4. 当问题继续追问 `差异原因`、`为什么不一样`、`回款和利润差异` 时，再展开 `cash_flow`、`difference_bridge` 与 `profit_cash_bridge`。
5. 老板可见回答必须翻译成财务概念与 Excel 来源，不展示数据库 ID、科目代码、SQL、`route_decision`、`probe_results` 等辅助字段。

桥接层（`plugin/openclaw-finance/server/finance_bridge.py`）会额外补充：

1. `data.exposed_fields.dual_perspective`
2. `data.exposed_fields.hr_breakdown`
3. `data.exposed_fields.arithmetic_checks`
4. `data.exposed_fields.intent_trace`
5. `data.query_spec`
6. `data.route_decision`
7. `data.route_decision.probe_results`
8. `data.source_note`
9. `data.source_documents`
10. `data.primary_source_tables`
11. `data.supporting_source_documents`
12. `bridge_meta.skill_contract_version`
13. `bridge_meta.protocol_version`（当前 `v2`）
14. `bridge_meta.capabilities`
15. `bridge_meta.capabilities.exposed_tools`
16. `bridge_meta.skill_appendix_relative_path`
17. `bridge_meta.skill_appendix_path`
18. `bridge_meta.skill_appendix_exists`

说明：桥接层不再读取/注入 `SKILL.md` 或 appendix 的正文规则，skill 仍由宿主（OpenClaw/Claude Code）skills 机制加载；但桥接层会读取 `SKILL.md` 顶部的契约版本标记，校验 appendix 相对路径是否存在，并把这些元数据写回响应。
当前推荐使用“核心版 SKILL + 附录”：

1. 核心注入：仓库根目录 `SKILL.md`（短上下文高准确）
2. 详细规则：`docs/SKILL_APPENDIX_FULL.md`（按需查阅）
3. 发布到 Claude Code / OpenClaw 时，需保留 `SKILL.md -> docs/SKILL_APPENDIX_FULL.md` 这条相对路径
4. 线上 OpenClaw 当前路径：`/root/.openclaw/skills/finance/SKILL.md` 与 `/root/.openclaw/skills/finance/docs/SKILL_APPENDIX_FULL.md`
5. 线上 Claude Code 当前路径：`/root/.claude/skills/finance/SKILL.md` 与 `/root/.claude/skills/finance/docs/SKILL_APPENDIX_FULL.md`
6. 旧路径 `/root/.openclaw/workspace/skills/finance-orchestrator` 已废弃，不再作为发布或验证目标

## 七、Agent 对接能力矩阵

为保证 OpenClaw / Claude Code 全面调用代码库功能，建议按下表接入：

1. 桥接工具（开箱即用）：
   - `finance-query` → `financeqa query`
   - `finance-host-data` → `financeqa host-data`
   - `finance-upload` → `financeqa import`（单文件）
   - `finance-sync` → `financeqa sync`（目录批量导入）
   - `finance-dimensions` → `financeqa dimensions ...`（维度导入导出与规则管理）
2. 直接 CLI/SDK（显式维护时使用）：
   - `financeqa config show`
   - `financeqa keywords intents`
   - Go SDK：`query.NewEngine / Engine.Query / Engine.HostLLMPayload`
3. OpenClaw/Claude 调 MCP bridge 时，`finance-query` 的推荐调用格式是：

```json
{"action":"call","name":"finance-query","arguments":{"query":"2026年Q1利润多少？"}}
```

## 八、回答红线（不能犯）

1. 不能把“银行到账”直接当“当月收入确认”。
2. 不能把供应商付款/工资/税费误归因成收入差异。
3. 不能在证据不足时编造结算月份、合同归属或开票归属。
4. 面向老板不能只回一个数字，必须保留口径、期间、主要来源和必要解释；但不要展示老板看不懂的数据库辅助字段。
5. 不能只看 CLI 退出码；业务失败时仍要先解析 stdout 的结构化 JSON（其中可能含 `llm_payload`）。
6. 接口层可以保留 `executed_sql`、`calculation_logs`、`trace`、`route_decision` 等调试字段，但宿主给老板总结时必须转成业务语言。
