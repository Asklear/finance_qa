# Q1 Finance Feedback Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align 2026 Q1 finance QA answers with the finance expert's verified口径, especially for profit formula, cash-bridge rollforward, tax basis, quarter aggregation, and AP open-item settlement.

**Architecture:** Keep the existing multi-source orchestrator, but tighten the authoritative accounting path in three layers: `core_metric` answers must expose complete利润字段和税口径；`cash_bridge` must be reconstructed from official balance deltas plus explicit non-operating adjustments; `arap` answers must separate official closing balance from open-item evidence and allow cross-month settlement matching. The PDF/Excel feedback is converted into a regression suite so future changes can be checked against the same 10 issues.

**Tech Stack:** Go, PostgreSQL, financeqa CLI, OpenClaw bridge, Go integration/unit tests.

---

## File Structure

- `internal/query/reconciliation.go`
  Responsible for monthly book summary, income-statement fallback, and profit-related core field assembly.
- `internal/query/core_metric_queries.go`
  Responsible for monthly/dual-perspective core metric answers and question-level wording.
- `internal/query/core_metrics_unified.go`
  Responsible for cross-period aggregation, consistency guard, and quarter/YTD authority logic.
- `internal/query/query_router.go`
  Responsible for metric-kind classification and perspective policy.
- `internal/query/contracts.go`
  Responsible for contract/customer/supplier dimension answers and tax-basis alignment.
- `internal/query/arap_queries.go`
  Responsible for official AR/AP answers plus open-item evidence packaging.
- `internal/openitems/pairing.go`
  Responsible for cross-month settlement matching logic and rollforward buckets.
- `internal/analysis/cash_bridge.go`
  Responsible for the accounting cash bridge from profit to bank net cash.
- `plugin/openclaw-finance/server/finance_bridge.py`
  Responsible for host-facing structured disclosure and boss summary.
- `tests/integration/monthly_summary_authority_test.go`
  Focused core-metric authority and journal-fallback tests.
- `tests/integration/reconciliation_analysis_test.go`
  Focused profit/cash bridge and Q1/Q3 real-accounting consistency tests.
- `tests/unit/analysis/cash_bridge_test.go`
  Focused bridge math tests.
- `tests/unit/query/contract_dimension_test.go`
  Focused tax-basis and contract answer tests.
- `tests/unit/query/source_adapter_arap_internal_test.go`
  Focused AR/AP source-adapter behavior.
- `tests/integration/realdata_question_suites_test.go`
  End-to-end regression entry point for expert-reviewed questions.
- `tests/testdata/`
  Regression fixtures for the Q1 PDF + Excel expert feedback.

---

### Task 1: Lock The Expert Feedback Into Regression Fixtures

**Files:**
- Create: `tests/testdata/q1_finance_feedback_cases.json`
- Create: `tests/integration/q1_finance_feedback_regression_test.go`
- Modify: `tests/integration/realdata_question_suites_test.go`

- [ ] **Step 1: Write the failing regression cases**
  Encode the 10 expert findings as structured cases:
  - question
  - expected key fields
  - expected accounting rule
  - forbidden wording

- [ ] **Step 2: Run focused test to verify current failures**
  Run: `/opt/homebrew/bin/go test ./tests/integration -run Q1FinanceFeedback -v`
  Expected: FAIL on the currently mismatched items.

- [ ] **Step 3: Wire the regression helper into the realdata suite**
  Ensure the same fixture can be reused in smoke tests and future production audits.

- [ ] **Step 4: Re-run focused test until the fixture is stable**
  Run: `/opt/homebrew/bin/go test ./tests/integration -run 'Q1FinanceFeedback|RealdataQuestion' -v`

**Acceptance:**
- Every Excel finding maps to one explicit regression case.
- No future fix can silently regress one of these Q1 answers.

---

### Task 2: Fix Profit Formula And Required Profit Fields

**Files:**
- Modify: `internal/query/reconciliation.go`
- Modify: `internal/query/core_metric_queries.go`
- Modify: `internal/query/core_metric_snapshot.go`
- Modify: `internal/query/source_adapter_core_metrics.go`
- Modify: `tests/integration/monthly_summary_authority_test.go`
- Modify: `tests/integration/reconciliation_analysis_test.go`

- [ ] **Step 1: Write failing tests for non-operating items**
  Cover:
  - 2 月利润需包含营业外收入 `0.19`
  - 3 月利润需包含营业外收入 `0.12`
  - `利润` and `净利润` must stay separate

- [ ] **Step 2: Run focused tests to verify failures**
  Run: `/opt/homebrew/bin/go test ./tests/integration -run 'MonthlySummary|Reconciliation' -v`

- [ ] **Step 3: Update the authoritative monthly book summary path**
  Ensure `monthlyBookSummary` and downstream snapshots always expose:
  - `营业外收入`
  - `营业外支出`
  - `利润`
  - `净利润`
  - `所得税`

- [ ] **Step 4: Make wording and snapshots use the same formula**
  Profit must always follow:
  `收入 - 成本及费用 + 营业外收入 - 营业外支出`

- [ ] **Step 5: Re-run focused tests**
  Run: `/opt/homebrew/bin/go test ./tests/integration -run 'MonthlySummary|Reconciliation' -v`

**Acceptance:**
- 2 月、3 月利润解释不再漏营业外收入。
- 问“利润”和问“净利润”返回的字段与口径完全分开。

---

### Task 3: Rebuild The Profit-To-Cash Bridge Using Accounting Rollforward

**Files:**
- Modify: `internal/analysis/cash_bridge.go`
- Modify: `internal/query/reconciliation.go`
- Modify: `internal/query/core_metric_queries.go`
- Modify: `tests/unit/analysis/cash_bridge_test.go`
- Modify: `tests/integration/reconciliation_analysis_test.go`

- [ ] **Step 1: Write failing bridge tests for expert-provided 3 月拆解**
  Cover these bridge buckets:
  - 应收净减少
  - 预付净减少
  - 其他应收净增加
  - 应付账款净减少
  - 预收净减少
  - 应付职工薪酬净增加
  - 税项留抵
  - 固定资产购置本金
  - 折旧

- [ ] **Step 2: Run focused bridge tests**
  Run: `/opt/homebrew/bin/go test ./tests/unit/analysis ./tests/integration -run 'CashBridge|Reconciliation' -v`

- [ ] **Step 3: Refactor the bridge to separate operating / tax / capex / non-operating adjustments**
  The bridge should be reproducible from official deltas rather than broad heuristic “difference reasons”.

- [ ] **Step 4: Update answer rendering**
  Answers should state the bridge buckets explicitly and use the same numbers as the structured payload.

- [ ] **Step 5: Re-run focused tests**
  Run: `/opt/homebrew/bin/go test ./tests/unit/analysis ./tests/integration -run 'CashBridge|Reconciliation' -v`

**Acceptance:**
- 3 月利润调现金桥 can be recomputed from returned fields.
- “差异原因” stops mixing operating deltas with capex/tax/noise.

---

### Task 4: Unify Tax-Inclusive vs Tax-Exclusive Customer Metrics

**Files:**
- Modify: `internal/query/contracts.go`
- Modify: `internal/query/counterparty_queries.go`
- Modify: `internal/query/result_helpers.go`
- Modify: `plugin/openclaw-finance/server/finance_bridge.py`
- Modify: `tests/unit/query/contract_dimension_test.go`
- Modify: `tests/integration/finance_bridge_contract_test.go`
- Modify: `tests/integration/q1_finance_feedback_regression_test.go`

- [ ] **Step 1: Write failing tests for Flywheel / 金程 inconsistencies**
  Cover:
  - 飞未销售额 vs 回款 comparison must not imply “未回款” when one side is税前 and the other is含税
  - 飞未与金程 answers must use consistent tax-basis labels
  - 银行累计流入 must include all normalized aliases

- [ ] **Step 2: Run focused tests**
  Run: `/opt/homebrew/bin/go test ./tests/unit/query ./tests/integration -run 'Contract|Bridge|Q1FinanceFeedback' -v`

- [ ] **Step 3: Add explicit tax-basis fields to customer/counterparty answers**
  At minimum expose:
  - `tax_basis`
  - `comparison_allowed`
  - `comparison_note`

- [ ] **Step 4: Normalize counterparty aliases before bank aggregation**
  Ensure 飞未云科 cumulative bank receipts align with the expert total `3,600,800.00`.

- [ ] **Step 5: Re-run focused tests**
  Run: `/opt/homebrew/bin/go test ./tests/unit/query ./tests/integration -run 'Contract|Bridge|Q1FinanceFeedback' -v`

**Acceptance:**
- 系统不再把“含税回款”和“不含税销售额”直接相减解释成未回款。
- 飞未、金程、合同维度回答具备统一税口径说明。

---

### Task 5: Fix Quarter / YTD Aggregation And Self-Consistency

**Files:**
- Modify: `internal/query/core_metrics_unified.go`
- Modify: `internal/query/core_metric_snapshot.go`
- Modify: `internal/query/query_router.go`
- Modify: `tests/integration/monthly_summary_authority_test.go`
- Modify: `tests/integration/q1_finance_feedback_regression_test.go`

- [ ] **Step 1: Write failing tests for Q1 totals**
  Cover expert-authoritative values:
  - 营业成本及费用 `10,522,743.20`
  - 账面净利润 `312,220.72`
  Also verify that monthly rows, quarter total, and final summary agree.

- [ ] **Step 2: Run focused tests**
  Run: `/opt/homebrew/bin/go test ./tests/integration -run 'MonthlySummary|Q1FinanceFeedback' -v`

- [ ] **Step 3: Refactor range aggregation**
  Use:
  - `SUM(current_amount)` as the primary range aggregation
  - `cumulative_amount delta` as a consistency check
  - explicit failure note when the two disagree

- [ ] **Step 4: Prevent self-contradictory rendering**
  The quarter answer must not show one total in the summary and another total in the monthly breakdown.

- [ ] **Step 5: Re-run focused tests**
  Run: `/opt/homebrew/bin/go test ./tests/integration -run 'MonthlySummary|Q1FinanceFeedback' -v`

**Acceptance:**
- Q1 answer becomes internally consistent.
- Range totals are auditable and explainable.

---

### Task 6: Rebuild AP Official Balance + Open-Item Settlement Matching

**Files:**
- Modify: `internal/query/arap_queries.go`
- Modify: `internal/query/source_adapter_arap.go`
- Modify: `internal/openitems/pairing.go`
- Modify: `internal/openitems/evidence.go`
- Modify: `tests/unit/query/source_adapter_arap_internal_test.go`
- Modify: `tests/integration/reconciliation_analysis_test.go`
- Modify: `tests/integration/q1_finance_feedback_regression_test.go`

- [ ] **Step 1: Write failing tests for 林悦 / 中闻 / 南京众信 AP cases**
  Cover:
  - 林悦 3 月末 official AP should be `2,530,000.00`
  - 中闻 `50,000` 已支付不能继续挂开放项
  - 南京众信 `32,902.07` 已支付不能继续挂开放项

- [ ] **Step 2: Run focused AP tests**
  Run: `/opt/homebrew/bin/go test ./tests/unit/query ./tests/integration -run 'ARAP|OpenItem|Q1FinanceFeedback' -v`

- [ ] **Step 3: Make official balance the first-class answer**
  Return:
  - 期初
  - 本期新增
  - 本期冲减 / 付款
  - 期末
  before listing open items.

- [ ] **Step 4: Tighten cross-month settlement matching**
  Pairing must support:
  - historical settlement
  - same-month settlement
  - confirmed vs probable buckets
  - zeroing out already-paid counterparties

- [ ] **Step 5: Re-run focused AP tests**
  Run: `/opt/homebrew/bin/go test ./tests/unit/query ./tests/integration -run 'ARAP|OpenItem|Q1FinanceFeedback' -v`

**Acceptance:**
- AP answers stop confusing official balance with residual open-item snapshot.
- 林悦/中闻/南京众信 examples match expert-reviewed numbers.

---

### Task 7: Sync Host Output Rules And Final Smoke Tests

**Files:**
- Modify: `SKILL.md`
- Modify: `docs/SKILL_APPENDIX_FULL.md`
- Modify: `tests/integration/finance_bridge_contract_test.go`
- Modify: `tests/integration/q1_finance_feedback_regression_test.go`

- [ ] **Step 1: Write failing bridge expectations**
  Verify host summary preserves:
  - 利润 vs 净利润 distinction
  - tax-basis warnings
  - official AP vs open-item evidence distinction

- [ ] **Step 2: Run bridge-focused tests**
  Run: `/opt/homebrew/bin/go test ./tests/integration -run 'FinanceBridge|Q1FinanceFeedback' -v`

- [ ] **Step 3: Update host-facing skill docs**
  Make the host rules explicit for:
  - tax basis
  - comparison bans
  - official AR/AP first, evidence second

- [ ] **Step 4: Run full verification**
  Run:
  - `/opt/homebrew/bin/go test ./...`
  - `python3 -m py_compile plugin/openclaw-finance/server/finance_bridge.py`

- [ ] **Step 5: Re-run Q1 smoke suite with real data**
  Run the Q1 regression suite and compare against the expert feedback workbook.

**Acceptance:**
- Host summaries cannot silently strip accounting caveats.
- Q1 finance smoke tests become part of the standard release gate.

