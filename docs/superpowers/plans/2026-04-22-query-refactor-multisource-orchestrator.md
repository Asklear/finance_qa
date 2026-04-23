# Query Refactor And Multi-Source Orchestrator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把当前以 `internal/query/engine.go` 为中心的问答逻辑拆成清晰模块，并落地一个按“指标/意图”选择数据源的多源编排器，避免继续靠硬编码路由、单文件堆逻辑和补丁式修复推进财务问答。

**Architecture:** 先冻结现有关键行为，再把查询流程重构为 `Intent -> QuerySpec -> Source Plan -> Source Facts -> Reconciler -> Answer Policy` 六层。数据源选择不再使用“全局表优先级”，而改为“按指标定义 source strategy”，例如收入/成本默认先现金口径再经营口径，应收应付默认先官方余额再 open-item 证据，合同问题优先合同台账但允许补充序时账和银行流水。

**Tech Stack:** Go, PostgreSQL, existing `internal/query`, `internal/openitems`, `internal/analysis`, `internal/accounting`, OpenClaw bridge, Go unit/integration tests, real-data regression scripts.

---

## Scope And Non-Goals

- 本计划优先覆盖查询层重构和多源编排器，不在第一阶段重写所有导入链路。
- `internal/parser` 和 `internal/ingest` 的瘦身放到后续阶段，先保证查询层可持续演进。
- 不引入新的数据库视图，不依赖 SQLite 回退，不做面向单个客户/供应商的硬编码补丁。
- 不改变现有 `fin_` 前缀表体系，不新增“为了查询方便”的临时 view。
- 回答策略必须保留当前业务约束：
  - 收入/成本类问题默认先现金口径，再补经营口径。
  - 利润默认走经营口径，现金差异通过调节桥或解释字段补充，不直接并列成双利润。
  - 应收/应付默认先给官方余额，再补回款/付款冲销证据，不得把“疑似匹配”说成“已冲销”。
  - 人力成本问题要能单列“工资、社保、公积金、分公司内部转账”，不能把内部转账吞掉。
  - 成本问题必须覆盖预提/冲回影响，不能只看当月付款。
  - 余额相关问题不得硬编码 `2026-01` 作为期初，必须使用 `opening_period` 或会计期间起始月语义。
  - 对外结构化返回要兼容现有宿主依赖字段，至少在迁移期保留 `money_view`、`account_view`、`dual_perspective`、`host_summary_contract` 的兼容映射。

## Recommended Design

### Query Pipeline

1. `Intent Router`
   - 只负责识别问题类型、期间、实体、指标、是否合同维度、是否需要多源回答。
   - 输出统一 `QuerySpec`，不直接访问数据库。
2. `Query Planner`
   - 根据 `QuerySpec` 和 answer policy 选择 source strategy。
   - 例子：
     - 月度收入：`cash_receipts + accrual_revenue`
     - 月度成本：`cash_payments + accrual_cost`
     - 利润：`accrual_profit + cash_bridge(optional)`
     - 应收应付：`official_balance + openitem_evidence`
     - 合同问题：`contract_ledger + journal + bank_statement`
3. `Source Adapters`
   - 每个 adapter 只做一件事：从某类表拿到标准化 `FactSet`。
   - 不负责文案，不负责最终结论。
4. `Fact Reconciler`
   - 合并多源 facts，记录覆盖率、冲突、可信度、差异说明。
   - 输出 `AnswerFrame`，供 answer policy 选择表达方式。
5. `Answer Policy`
   - 负责“先现金后经营”“利润单口径”“AR/AP 先官方后证据”等业务口径。
   - 不直接依赖底层表，只依赖统一 `AnswerFrame`。
6. `Bridge / Host Summary`
   - OpenClaw / Claude 宿主层只消费结构化结果，不在桥接层二次猜测财务逻辑。

### Migration Strategy

- 采用“按问题家族切流”的迁移方式，不做大爆炸替换。
- `engine.go` 在迁移期保留 compatibility shim：
  - 已迁移的问题家族走 orchestrator
  - 未迁移的问题家族继续走 legacy path
- 每完成一个问题家族，就补齐：
  - unit tests
  - PostgreSQL integration tests
  - real-data smoke questions
- 只有当新路径在真实数据上稳定后，才删除对应 legacy handler。

### QuerySpec Requirements

`QuerySpec` 除了基础的 `metric/entity/period` 外，还必须显式表达下面这些维度，避免再次把语义混进 handler：

- `time_scope`
  - `month`
  - `quarter`
  - `half_year`
  - `year_full`
  - `year_to_date`
  - `custom_range`
- `sub_period`
  - 用于“今年回款多少？其中3月到账多少？”这类主问题 + 子期间拆分
- `comparison_mode`
  - `cash_then_accrual`
  - `accrual_only`
  - `official_then_evidence`
- `readiness_check_required`
  - 用于“3月数据出来了吗”这类数据可用性问题
- `authoritative_source_required`
  - 用于必须优先官方余额/官方报表的问题
- `opening_period_aware`
  - 用于期初/期末/滚动余额类问题，避免再次写死年度起点
- `lexicon_profile`
  - 路由和关键词判断必须复用 `rules_config.go` / `rule_lexicon.go`，不允许在新 router/planner 里重新散落 literal 词表

### Source Strategy Rules

- 不建立“全局最高优先表”。
- 每个指标定义自己的 source strategy：
  - `cash_receipts`: `bank_statement`, contract cash facts
  - `cash_payments`: `bank_statement`
  - `accrual_revenue`: `income_statement`, `journal`, contract ledger
  - `accrual_cost`: `income_statement`, `journal`, `balance_detail`
  - `official_ar_ap`: `balance_detail`, `balance_sheet`
  - `openitem_evidence`: `internal/openitems`
  - `hr_cost_fact`: `journal`, `balance_detail`, `bank_statement`
  - `tax_fact`: `journal`, `balance_detail`
  - `supplier_payment_fact`: `bank_statement` + supplier filter
  - `data_readiness_fact`: `income_statement`, `balance_detail`, `journal`, contract tables
- planner 决定组合，不由 handler 手写 if/else 串联。

### Fact Contract Requirements

每个 `Fact` / `FactSet` 除金额外，还必须携带：

- `authority_level`
  - `official`
  - `supporting`
  - `derived`
- `period_from` / `period_to`
- `opening_period`
  - 当来源是余额表/科目余额相关数据时必填
- `coverage_status`
  - `full`
  - `partial`
  - `missing`
- `staleness`
  - 例如“只到 2026-02”
- `confidence`
  - 尤其给 open-item 冲销证据使用
- `trace_payload`
  - 保留给 bridge 和调试输出

### Refactor Boundaries

- `engine.go` 最终只保留：
  - `Engine` 初始化
  - 顶层 `Query`
  - 调用 router / planner / orchestrator / answer composer
- 抽离后的核心文件建议：
  - `internal/query/query_spec.go`
  - `internal/query/query_router.go`
  - `internal/query/query_planner.go`
  - `internal/query/orchestrator.go`
  - `internal/query/source_registry.go`
  - `internal/query/source_adapter_core_metrics.go`
  - `internal/query/source_adapter_arap.go`
  - `internal/query/source_adapter_contracts.go`
  - `internal/query/source_adapter_supplier.go`
  - `internal/query/fact_types.go`
  - `internal/query/fact_reconciler.go`
  - `internal/query/answer_policy.go`
  - `internal/query/answer_composer.go`

## File Structure

- Modify: `internal/query/engine.go`
  Responsibility: shrink into thin entrypoint and compatibility shim during migration.
- Create: `internal/query/query_spec.go`
  Responsibility: define `QuerySpec`, `MetricKind`, `PerspectivePolicy`, `SourceNeed`, `Coverage`.
- Create: `internal/query/query_router.go`
  Responsibility: map natural-language question into `QuerySpec`.
- Create: `internal/query/query_planner.go`
  Responsibility: map `QuerySpec` into source strategy without touching SQL.
- Create: `internal/query/orchestrator.go`
  Responsibility: execute source plan, collect facts, call reconciler and answer policy.
- Create: `internal/query/source_registry.go`
  Responsibility: register and resolve source adapters by capability.
- Create: `internal/query/fact_types.go`
  Responsibility: define `Fact`, `FactSet`, `Evidence`, `Conflict`, `AnswerFrame`.
- Create: `internal/query/fact_reconciler.go`
  Responsibility: merge facts, calculate coverage/conflicts, attach explanations.
- Create: `internal/query/answer_policy.go`
  Responsibility: encode boss-facing output rules per metric and question family.
- Create: `internal/query/answer_composer.go`
  Responsibility: convert `AnswerFrame` into `Result`.
- Create: `internal/query/source_adapter_core_metrics.go`
  Responsibility: expose收入/成本/利润/税统一 facts.
- Create: `internal/query/source_adapter_arap.go`
  Responsibility: expose官方应收应付余额和 open-item 证据 facts.
- Create: `internal/query/source_adapter_contracts.go`
  Responsibility: expose合同台账 cash/book facts.
- Create: `internal/query/source_adapter_supplier.go`
  Responsibility: expose供应商付款 facts and supplier roster.
- Create: `internal/query/source_adapter_readiness.go`
  Responsibility: expose月度/季度/年度数据可用性 facts.
- Modify: `internal/query/contracts.go`
  Responsibility: migrate contract SQL and role logic into contract adapter / composer helpers.
- Modify: `internal/query/core_metrics_unified.go`
  Responsibility: become core-metric adapter helper instead of直接生成最终问答。
- Modify: `internal/query/reconciliation.go`
  Responsibility: consume orchestrator facts instead of自行拼接底层来源。
- Modify: `internal/query/helpers.go`
  Responsibility: keep pure parsing helpers only.
- Modify: `internal/query/rules_config.go`
  Responsibility: remain the single source of configurable query lexicon and thresholds during the refactor.
- Modify: `internal/query/rule_lexicon.go`
  Responsibility: expose config-backed lexicon accessors to the new router/planner.
- Modify: `internal/query/internal_party.go`
  Responsibility: provide reusable internal-party evidence for HR and internal-transfer answers.
- Modify: `plugin/openclaw-finance/server/finance_bridge.py`
  Responsibility: trust orchestrator output contract and remove duplicate host-side heuristics where possible.
- Modify: `tests/scripts/build_openclaw_package.sh`
  Responsibility: keep bridge + skill + appendix packaging aligned after output contract changes.
- Modify: `tests/scripts/sync_openclaw_bridge_and_skill.sh`
  Responsibility: keep sync verification aligned with the new contract and avoid stale grep checks.
- Modify: `docs/architecture/01-layered-architecture.md`
  Responsibility: document new query/orchestrator boundaries.
- Modify: `docs/architecture/02-query-sequence.md`
  Responsibility: document `QuerySpec -> Planner -> Orchestrator -> Reconciler -> Answer Policy`.
- Modify: `docs/architecture/03-deployment-runtime.md`
  Responsibility: document bridge / skill / appendix loading responsibilities after refactor.
- Test: `tests/unit/query/*.go`
  Responsibility: split tests by router, planner, source adapters, policies, orchestration.
- Test: `tests/integration/*.go`
  Responsibility: validate end-to-end behavior against PostgreSQL and bridge payloads.

## Task 1: Freeze Current High-Risk Behaviors With Tests

**Files:**
- Modify: `tests/unit/query/entity_routing_test.go`
- Modify: `tests/unit/query/contract_dimension_test.go`
- Modify: `tests/unit/query/arap_open_items_test.go`
- Create: `tests/unit/query/orchestrator_regression_test.go`
- Modify: `tests/integration/realdata_question_suites_test.go`

- [ ] **Step 1: Write failing or missing regression tests for the current must-keep behaviors**

Cover at least:
- 收入/成本回答顺序为“先现金口径，再经营口径”
- 利润默认不输出双利润
- 应收/应付默认先官方余额，再补冲销证据
- 合同问题默认先合同数据，再补充其他来源
- 季度/半年/全年问题不会误落到单月 `current_amount`
- `今年累计 + 其中某月` 问题会拆成主期间和子期间
- 人力成本问题会单列分公司内部转账
- 成本计算包含预提/冲回影响
- 余额类问题使用 `opening_period` / 会计期间起始月，不写死 `2026-01`
- “xx月数据出来了吗” 走数据可用性判断而不是金额问答

- [ ] **Step 2: Run focused unit tests and record the baseline**

Run:
`/opt/homebrew/bin/go test ./tests/unit/query -run 'TestContract|TestARAP|TestEntityRouting|TestQuarter|TestHalfYear|TestFullYear' -v`

Expected:
现有已知缺陷先暴露出来，但新增测试文件结构和断言边界明确。

- [ ] **Step 3: Add real-data integration assertions for the top boss questions**

Add or update coverage for:
- 2026年3月收入、成本、利润分别是多少
- 金程今年回款多少？其中3月到账多少？
- 南京林悦智能科技有限公司3月成本多少？
- 2026年3月有多少家供应商发生付款？分别叫什么、各付了多少？
- 2026年第一季度营收
- 2026年3月人力成本多少？工资、社保、公积金分别是多少？
- 2026年3月数据出来了吗？
- 2026年3月应付账款多少（已收发票未付款）？

- [ ] **Step 4: Run integration baselines on PostgreSQL**

Run:
- `/opt/homebrew/bin/go test ./tests/integration -run 'TestRealdataQuestionSuites|TestMonthlySummaryAuthority|TestReconciliationAnalysis' -v`

Expected:
输出当前真实行为，作为后续重构对照基线。

## Task 2: Introduce QuerySpec And Router/Planner Boundaries

**Files:**
- Create: `internal/query/query_spec.go`
- Create: `internal/query/query_router.go`
- Create: `internal/query/query_planner.go`
- Modify: `internal/query/engine.go`
- Test: `tests/unit/query/entity_routing_test.go`
- Test: `tests/unit/query/intent_period_company_test.go`

- [ ] **Step 1: Write failing tests for `QuerySpec` generation**

Add tests proving that the router emits explicit fields for:
- `metric_kind`
- `period_from`
- `period_to`
- `entity`
- `question_family`
- `perspective_policy`
- `needs_contract_dimension`
- `time_scope`
- `sub_period`
- `readiness_check_required`
- `opening_period_aware`
- `lexicon_profile`

- [ ] **Step 2: Run router tests to verify they fail**

Run:
`/opt/homebrew/bin/go test ./tests/unit/query -run 'TestQuerySpec|TestIntentPeriod|TestEntityRouting' -v`

Expected:
FAIL because `QuerySpec` and planner interfaces do not yet exist.

- [ ] **Step 3: Implement `QuerySpec` types and move pure routing logic out of `engine.go`**

Define:
- `QueryFamily`
- `MetricKind`
- `PerspectivePolicy`
- `SourceCapability`
- `QuerySpec`

Move pure interpretation logic from `engine.go` and `helpers.go` into `query_router.go`.
New routing code must consume `rule_lexicon.go` / `rules_config.go`, not copy literal keyword slices into the new files.

- [ ] **Step 4: Implement planner skeleton with source strategy selection**

Planner must support at least:
- `core_metrics`
- `arap`
- `contracts`
- `supplier_payments`
- `reconciliation`
- `readiness`

- [ ] **Step 4.5: Add migration switches by question family**

During migration, `engine.go` should be able to route per question family:
- `legacy`
- `orchestrator`

without changing external CLI / bridge interfaces.

- [ ] **Step 5: Run tests to verify the routing layer passes**

Run:
`/opt/homebrew/bin/go test ./tests/unit/query -run 'TestQuerySpec|TestIntentPeriod|TestEntityRouting' -v`

Expected:
PASS with no SQL access required for router/planner tests.

- [ ] **Step 6: Commit**

```bash
git add internal/query/query_spec.go internal/query/query_router.go internal/query/query_planner.go internal/query/rules_config.go internal/query/rule_lexicon.go internal/query/engine.go tests/unit/query/entity_routing_test.go tests/unit/query/intent_period_company_test.go tests/unit/query/orchestrator_regression_test.go
git commit -m "refactor: add query spec and planner boundaries"
```

## Task 3: Build Standard Fact Types And Source Registry

**Files:**
- Create: `internal/query/fact_types.go`
- Create: `internal/query/source_registry.go`
- Create: `internal/query/orchestrator.go`
- Test: `tests/unit/query/orchestrator_regression_test.go`

- [ ] **Step 1: Write failing tests for source registry and orchestrator contracts**

Add tests covering:
- planner emits a source plan
- registry resolves adapters by capability
- orchestrator executes adapters in plan order
- missing capability returns structured error

- [ ] **Step 2: Run focused orchestrator tests**

Run:
`/opt/homebrew/bin/go test ./tests/unit/query -run 'TestSourceRegistry|TestOrchestrator' -v`

Expected:
FAIL because orchestrator primitives do not exist.

- [ ] **Step 3: Implement standard fact contracts**

Define:
- `Fact`
- `FactSet`
- `Evidence`
- `CoverageSummary`
- `Conflict`
- `AnswerFrame`

Each fact must carry:
- source name
- metric key
- entity
- period range
- amount/value
- authority level
- coverage status
- opening period when relevant
- staleness/readiness markers
- confidence
- trace payload

- [ ] **Step 4: Implement registry and orchestrator skeleton**

Orchestrator responsibilities:
- accept `QuerySpec`
- ask planner for a source plan
- invoke adapters
- aggregate SQL and trace logs
- pass `FactSet`s to reconciler

- [ ] **Step 5: Run tests to verify source orchestration contracts pass**

Run:
`/opt/homebrew/bin/go test ./tests/unit/query -run 'TestSourceRegistry|TestOrchestrator' -v`

Expected:
PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/query/fact_types.go internal/query/source_registry.go internal/query/orchestrator.go tests/unit/query/orchestrator_regression_test.go
git commit -m "refactor: add query fact contracts and source registry"
```

## Task 4: Convert Core Metrics Into Source Adapters

**Files:**
- Create: `internal/query/source_adapter_core_metrics.go`
- Modify: `internal/query/core_metrics_unified.go`
- Modify: `internal/query/reconciliation.go`
- Modify: `internal/query/engine.go`
- Test: `tests/unit/query/profit_cash_bridge_test.go`
- Test: `tests/integration/monthly_summary_authority_test.go`

- [ ] **Step 1: Write failing tests for core metric fact extraction**

Cover:
- revenue returns `cash_receipts` and `accrual_revenue`
- cost returns `cash_payments` and `accrual_cost`
- profit returns `accrual_profit` and optional `cash_bridge`
- quarter/half-year/full-year aggregation uses range totals correctly
- cost facts include accrual / reversal effects when present
- HR facts can separate工资/社保/公积金/分公司转账

- [ ] **Step 2: Run core metric tests**

Run:
`/opt/homebrew/bin/go test ./tests/unit/query -run 'TestProfitCashBridge|TestQuarter|TestHalfYear|TestFullYear' -v`

Expected:
FAIL because core metrics still emit final answers directly from legacy paths.

- [ ] **Step 3: Refactor `core_metrics_unified.go` into reusable adapter helpers**

Move code so this layer returns facts and validations, not boss-facing messages.

- [ ] **Step 4: Update reconciliation to consume fact outputs**

Reconciliation should reference unified facts instead of independently recomputing source selection where possible.

- [ ] **Step 5: Run targeted unit and integration tests**

Run:
- `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestProfitCashBridge|TestQuarter|TestHalfYear|TestFullYear' -v`
- `/opt/homebrew/bin/go test ./tests/integration -run 'TestMonthlySummaryAuthority|TestReconciliationAnalysis' -v`

Expected:
PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/query/source_adapter_core_metrics.go internal/query/core_metrics_unified.go internal/query/reconciliation.go internal/query/engine.go tests/unit/query/profit_cash_bridge_test.go tests/integration/monthly_summary_authority_test.go tests/integration/reconciliation_analysis_test.go
git commit -m "refactor: move core metrics into source adapters"
```

## Task 5: Convert AR/AP, Open Item, And Supplier Payments Into Source Adapters

**Files:**
- Create: `internal/query/source_adapter_arap.go`
- Create: `internal/query/source_adapter_supplier.go`
- Modify: `internal/openitems/pairing.go`
- Modify: `internal/query/engine.go`
- Test: `tests/unit/query/arap_open_items_test.go`
- Test: `tests/unit/query/supplier_payment_filter_test.go`

- [ ] **Step 1: Write failing tests for official-balance plus evidence composition**

Cover:
- AR/AP answers expose official balance separately from settlement evidence
- unmatched evidence never becomes confirmed recovery
- supplier payment question obeys selected period and external-supplier filter
- supplier payment answer includes structured supplier list and totals

- [ ] **Step 2: Run AR/AP and supplier tests**

Run:
`/opt/homebrew/bin/go test ./tests/unit/query -run 'TestARAP|TestSupplierPayment' -v`

Expected:
FAIL because legacy code mixes direct answer wording and data extraction.

- [ ] **Step 3: Implement AR/AP and supplier adapters**

Requirements:
- `official_ar_ap` adapter returns official month-end facts
- `openitem_evidence` adapter returns confidence-scored settlement evidence
- `supplier_payment` adapter returns external supplier roster, filtered by month and non-supplier exclusions
- open-item and official balance answers both carry `opening_period` semantics when relevant

- [ ] **Step 4: Run unit and integration tests**

Run:
- `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestARAP|TestSupplierPayment' -v`
- `/opt/homebrew/bin/go test ./tests/integration -run 'TestRealdataQuestionSuites' -v`

Expected:
PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query/source_adapter_arap.go internal/query/source_adapter_supplier.go internal/openitems/pairing.go internal/query/engine.go tests/unit/query/arap_open_items_test.go tests/unit/query/supplier_payment_filter_test.go tests/integration/realdata_question_suites_test.go
git commit -m "refactor: add arap and supplier source adapters"
```

## Task 6: Convert Contract Dimension Queries Into Source Adapters

**Files:**
- Create: `internal/query/source_adapter_contracts.go`
- Modify: `internal/query/contracts.go`
- Modify: `internal/query/engine.go`
- Test: `tests/unit/query/contract_dimension_test.go`
- Test: `tests/integration/finance_bridge_contract_test.go`

- [ ] **Step 1: Write failing tests for contract source planning**

Cover:
- contract questions produce a plan with `contract_ledger` first
- customer contract answers expose cash and book views separately
- supplier contract answers expose付款 and合同成本 separately
- mixed contract counterparties preserve role ambiguity in structured payload

- [ ] **Step 2: Run contract-focused tests**

Run:
`/opt/homebrew/bin/go test ./tests/unit/query -run 'TestContract' -v`

Expected:
FAIL because contracts still bypass common orchestration.

- [ ] **Step 3: Implement contract adapter and shrink `contracts.go`**

Move:
- contract matching
- role detection
- contract ledger fact extraction

Keep `contracts.go` focused on contract-specific parsing helpers only if still needed.

- [ ] **Step 4: Run unit and integration tests**

Run:
- `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestContract' -v`
- `/opt/homebrew/bin/go test ./tests/integration -run 'TestFinanceBridgeContract' -v`

Expected:
PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query/source_adapter_contracts.go internal/query/contracts.go internal/query/engine.go tests/unit/query/contract_dimension_test.go tests/integration/finance_bridge_contract_test.go
git commit -m "refactor: move contract queries into source adapters"
```

## Task 6.5: Add Readiness And Coverage Adapters

**Files:**
- Create: `internal/query/source_adapter_readiness.go`
- Modify: `internal/query/query_planner.go`
- Modify: `internal/query/answer_policy.go`
- Test: `tests/unit/query/orchestrator_regression_test.go`

- [ ] **Step 1: Write failing tests for readiness questions**

Cover:
- “3月数据出来了吗” returns readiness/coverage instead of fabricated金额
- source gaps produce conservative wording
- readiness checks can report which source is missing or stale

- [ ] **Step 2: Run readiness tests**

Run:
`/opt/homebrew/bin/go test ./tests/unit/query -run 'TestReadiness|TestCoverage' -v`

Expected:
FAIL because readiness adapter does not exist.

- [ ] **Step 3: Implement readiness adapter and planner integration**

Requirements:
- planner recognizes readiness queries
- adapter inspects core tables and returns availability facts
- answer policy emits boss-readable readiness conclusion

- [ ] **Step 4: Run focused tests**

Run:
`/opt/homebrew/bin/go test ./tests/unit/query -run 'TestReadiness|TestCoverage' -v`

Expected:
PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query/source_adapter_readiness.go internal/query/query_planner.go internal/query/answer_policy.go tests/unit/query/orchestrator_regression_test.go
git commit -m "feat: add readiness source adapter"
```

## Task 7: Add Reconciler, Answer Policy, And Answer Composer

**Files:**
- Create: `internal/query/fact_reconciler.go`
- Create: `internal/query/answer_policy.go`
- Create: `internal/query/answer_composer.go`
- Modify: `internal/query/orchestrator.go`
- Modify: `internal/query/engine.go`
- Test: `tests/unit/query/orchestrator_regression_test.go`
- Test: `tests/integration/fallback_ux_test.go`

- [ ] **Step 1: Write failing tests for answer-policy behavior**

Cover:
- 收入回答先现金后经营
- 成本回答先现金后经营
- 利润默认经营口径，现金差异只作解释字段
- 应收应付回答先官方余额，再补冲销证据
- 缺失覆盖率时给出“未找到足够证据”的保守表述

- [ ] **Step 2: Run answer-policy tests**

Run:
`/opt/homebrew/bin/go test ./tests/unit/query -run 'TestOrchestrator|TestFallbackUX' -v`

Expected:
FAIL because answer policy layer does not exist yet.

- [ ] **Step 3: Implement reconciler and policy layer**

Requirements:
- `FactReconciler` 负责标准化差异说明
- `AnswerPolicy` 负责最终业务口径
- `AnswerComposer` 负责 message/data/sql/logs 组织
- 迁移期继续输出兼容字段，避免 bridge / 宿主马上失配

- [ ] **Step 4: Run unit and integration tests**

Run:
- `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestOrchestrator|TestFallbackUX' -v`
- `/opt/homebrew/bin/go test ./tests/integration -run 'TestFallbackUX|TestEngineIntegration' -v`

Expected:
PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query/fact_reconciler.go internal/query/answer_policy.go internal/query/answer_composer.go internal/query/orchestrator.go internal/query/engine.go tests/unit/query/orchestrator_regression_test.go tests/integration/fallback_ux_test.go tests/integration/engine_integration_test.go
git commit -m "refactor: add reconciler and answer policy layers"
```

## Task 8: Reduce `engine.go` To A Thin Entry Point

**Files:**
- Modify: `internal/query/engine.go`
- Modify: `internal/query/helpers.go`
- Test: `tests/integration/engine_integration_test.go`

- [ ] **Step 1: Measure and enforce the final responsibilities**

Target end state:
- `engine.go` under ~800 LOC
- no business-SQL-heavy source logic left in `engine.go`
- no direct contract/supplier/ARAP query assembly in `engine.go`
- no hardcoded year-opening assumptions in `engine.go`
- no duplicated literal keyword dictionaries in new router/planner code

- [ ] **Step 2: Move remaining pure helpers or dead logic out**

Candidates:
- period helpers into `helpers.go` or `query_router.go`
- fallback routing pieces into `query_router.go`
- message composition into `answer_composer.go`

- [ ] **Step 3: Run full query suite**

Run:
- `/opt/homebrew/bin/go test ./tests/unit/query -v`
- `/opt/homebrew/bin/go test ./tests/integration -run 'TestEngineIntegration|TestMain|TestSchemaContract' -v`

Expected:
PASS with unchanged external API shape.

- [ ] **Step 4: Commit**

```bash
git add internal/query/engine.go internal/query/helpers.go tests/integration/engine_integration_test.go
git commit -m "refactor: slim engine entrypoint"
```

## Task 9: Update Bridge And Skill Contracts To Match The New Architecture

**Files:**
- Modify: `plugin/openclaw-finance/server/finance_bridge.py`
- Modify: `SKILL.md`
- Modify: `docs/SKILL_APPENDIX_FULL.md`
- Modify: `tests/scripts/build_openclaw_package.sh`
- Modify: `tests/scripts/sync_openclaw_bridge_and_skill.sh`
- Modify: `docs/architecture/01-layered-architecture.md`
- Modify: `docs/architecture/02-query-sequence.md`
- Modify: `docs/architecture/03-deployment-runtime.md`
- Test: `tests/integration/schema_contract_test.go`
- Test: `tests/integration/skill_appendix_distribution_test.go`

- [ ] **Step 1: Write failing tests for bridge/schema expectations**

Cover:
- bridge still exposes expected structured fields
- host summary does not override orchestrator answer policy
- skill text reflects current source/policy behavior

- [ ] **Step 2: Run bridge/skill contract tests**

Run:
`/opt/homebrew/bin/go test ./tests/integration -run 'TestSchemaContract|TestSkillAppendixDistribution' -v`

Expected:
FAIL if docs and bridge drift from the new output contract.

- [ ] **Step 3: Update bridge and skill docs**

Requirements:
- `SKILL.md` 只描述对宿主真正重要的意图识别、数据源策略、合理性交叉验证、输出口径
- appendix 只保留业务规则和问答补充，不写开发/验收步骤
- bridge 优先使用结构化字段，不再二次发明财务口径
- packaging / sync scripts and architecture docs reflect the same contract and load path assumptions

- [ ] **Step 4: Run contract tests**

Run:
`/opt/homebrew/bin/go test ./tests/integration -run 'TestSchemaContract|TestSkillAppendixDistribution|TestFinanceBridgeContract' -v`

Expected:
PASS.

- [ ] **Step 5: Commit**

```bash
git add plugin/openclaw-finance/server/finance_bridge.py SKILL.md docs/SKILL_APPENDIX_FULL.md tests/integration/schema_contract_test.go tests/integration/skill_appendix_distribution_test.go tests/integration/finance_bridge_contract_test.go
git commit -m "docs: align bridge and skill contracts with orchestrator"
```

## Task 10: Run PostgreSQL Real-Data Regression And Deployment Smoke Tests

**Files:**
- Modify only if regressions require legitimate fixes
- Test: `tests/integration/*.go`
- Test: `tests/scripts/*.sh`

- [ ] **Step 1: Run full unit suite**

Run:
`/opt/homebrew/bin/go test ./tests/unit/... -v`

Expected:
PASS.

- [ ] **Step 2: Run PostgreSQL integration suite**

Run:
`/opt/homebrew/bin/go test ./tests/integration/... -v`

Expected:
PASS with PostgreSQL as the default tested backend.

- [ ] **Step 3: Run real-data smoke scripts**

Run:
- `./tests/scripts/run_user15_realdata_check.sh`
- `./tests/scripts/run_user19_realdata_check.sh`
- `./tests/scripts/run_top20_realdata_check.sh`

Expected:
所有报告生成成功；季度、合同、供应商付款、AR/AP、核心指标问题结果与真实数据一致。

- [ ] **Step 4: Run bridge smoke tests against the deployed environment**

Run examples:
- `./financeqa query --company "南京优集数据科技有限公司" "2026年3月收入、成本、利润分别是多少？"`
- `./financeqa query --company "南京优集数据科技有限公司" "金程今年回款多少？其中3月到账多少？"`
- `./financeqa query --company "南京优集数据科技有限公司" "2026年第一季度营收"`
- `./financeqa query --company "南京优集数据科技有限公司" "2026年3月有多少家供应商发生付款？分别叫什么、各付了多少？"`

Expected:
- 收入/成本：先现金后经营
- 利润：经营口径为主
- 季度/半年/全年：不误落单月
- 回款冲销：不把未确认匹配说成已回
- 供应商付款：按月份和外部供应商口径返回
- 期初/期末/滚动余额：按 `opening_period` 和会计期间语义回答
- readiness 问题：返回数据可用性，不编造金额

- [ ] **Step 5: Final commit**

```bash
git status --short
git add .
git commit -m "refactor: introduce multi-source query orchestration"
```

## Phase 2 Follow-Up Plan

下一个阶段再做，但在本计划里提前锁边界：

- `internal/ingest/importer.go`
  - split into import coordinator, company resolver, idempotency policy, writer
- `internal/parser/parse.go`
  - split by report type and period parsing concerns
- `plugin/openclaw-finance/server/finance_bridge.py`
  - split into contract loading, response shaping, tool dispatch if file continues to grow

## Acceptance Criteria

- `internal/query/engine.go` 不再是核心业务堆栈中心，而是薄入口。
- 所有高频财务问题通过统一 orchestrator 走 source plan，而不是在 handler 里各自拼 SQL。
- source priority 改为 metric-specific strategy，不存在“任何问题都先查同一张表”的硬编码。
- 收入/成本/利润、AR/AP、合同、供应商付款都能通过统一事实模型输出。
- QuerySpec 能表达 `time_scope`、`sub_period`、`readiness_check_required`、`opening_period_aware`。
- 新 router / planner 继续以 `rules_config.go` + `rule_lexicon.go` 作为唯一词表来源，不回退到散落硬编码。
- 迁移采用 family-by-family cutover，新旧路径并存期间外部接口保持稳定。
- 新增 adapter / planner / reconciler 不引入 SQLite-only SQL，PostgreSQL 为主测试后端。
- OpenClaw / Claude 宿主层不再依赖桥接层二次猜测财务逻辑。
- PostgreSQL 集成测试和真实数据 smoke test 全部通过。
