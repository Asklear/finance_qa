# Open Item Evidence Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace patch-style finance QA logic with a reusable evidence-driven matching engine that works across counterparties, periods, and voucher patterns without hardcoded customer or amount rules.

**Architecture:** We will split AR/AP reasoning into three layers: normalized ledger events, confidence-scored settlement matching, and answer policies that only emit certainty levels justified by evidence. HR internal-transfer handling will reuse the same evidence-first approach by introducing an internal-party resolver instead of relying on narrow summary heuristics.

**Tech Stack:** Go, SQLite, existing `internal/openitems`, `internal/query`, unit tests, integration tests, real-data regression scripts.

---

## File Structure

- Modify: `internal/openitems/pairing.go`
  Responsibility: replace direct FIFO-only settlement interpretation with event normalization, candidate matching, confidence grading, and richer summary output.
- Create: `internal/openitems/evidence.go`
  Responsibility: define reusable event, match, confidence, and explanation types used by pairing and query layers.
- Create: `internal/query/internal_party.go`
  Responsibility: detect internal branches/entities using generic name and account evidence instead of customer-specific heuristics.
- Modify: `internal/query/engine.go`
  Responsibility: consume richer open-item summaries, enforce official-balance-vs-open-item answer policy, downgrade wording when evidence is not confirmed, and generalize HR internal-transfer reporting.
- Modify: `internal/query/reconciliation.go`
  Responsibility: use confidence-aware wording for cash-vs-accrual explanations so unmatched settlements are never overstated.
- Modify: `tests/unit/query/arap_open_items_test.go`
  Responsibility: add failing tests for confidence-aware settlement output, unmatched wording, and non-overclaiming on customer receipts.
- Modify: `tests/unit/query/entity_routing_test.go`
  Responsibility: add failing tests for HR internal transfer detection using generic internal-party evidence and for profit/cost wording guarantees.
- Modify: `tests/integration/realdata_question_suites_test.go`
  Responsibility: keep suite coverage intact after answer-policy changes.
- Modify: `tests/testdata/user19_questions_2026-04-20.json`
  Responsibility: add or refine questions that exercise the new non-overclaiming semantics where needed.
- Modify: `tests/scripts/run_realdata_question_suite.go`
  Responsibility: preserve full-answer reporting for new semantics verification.

### Task 1: Build Confidence-Aware Open Item Matching

**Files:**
- Create: `internal/openitems/evidence.go`
- Modify: `internal/openitems/pairing.go`
- Test: `tests/unit/query/arap_open_items_test.go`

- [ ] **Step 1: Write failing tests for confidence-aware settlement buckets**

Add tests covering:
- cross-month confirmed settlement still works;
- same-counterparty but ambiguous receipt does not become `confirmed` automatically;
- unmatched receipts do not allow “已回/未回” certainty wording.

- [ ] **Step 2: Run target tests to verify they fail for the right reason**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestARAPUsesCrossMonthFIFOSettlement|TestARAPNormalizesSummaryDerivedCounterpartyBeforeSettlement|TestARAPUnmatchedSettlementDoesNotClaimRecovered' -v`
Expected: FAIL because summary/result structure and message policy do not yet expose confidence states.

- [ ] **Step 3: Implement event normalization and confidence scoring**

Introduce reusable event/match structs and upgrade pairing to output:
- settlement matches with `confirmed/probable/unmatched`;
- per-counterparty explanation fields;
- separate confirmed historical settlement vs unmatched decrease amounts.

- [ ] **Step 4: Run target tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestARAPUsesCrossMonthFIFOSettlement|TestARAPNormalizesSummaryDerivedCounterpartyBeforeSettlement|TestARAPUnmatchedSettlementDoesNotClaimRecovered' -v`
Expected: PASS.

### Task 2: Enforce Answer Policies From Evidence Levels

**Files:**
- Modify: `internal/query/engine.go`
- Modify: `internal/query/reconciliation.go`
- Test: `tests/unit/query/arap_open_items_test.go`

- [ ] **Step 1: Write failing tests for answer wording and official/open-item separation**

Add tests covering:
- entity AR/AP questions can say “未找到足够证据证明已冲销” instead of “全部未回” when evidence is unmatched;
- general AR/AP questions still prefer official balance totals;
- reconciliation explanations avoid direct subtraction claims when only probable or unmatched pairing exists.

- [ ] **Step 2: Run target tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestGeneralARAPUsesBalanceSheetAsOfficialTotal|TestEntityARAPUsesConfidenceAwareWording|TestReconciliationDoesNotOverclaimSettlement' -v`
Expected: FAIL because current wording overstates certainty.

- [ ] **Step 3: Implement confidence-aware answer policy**

Update query layer so:
- official totals remain primary for generic AR/AP;
- open-item analysis carries confidence metadata;
- answer text uses certainty only for confirmed matches.

- [ ] **Step 4: Run target tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestGeneralARAPUsesBalanceSheetAsOfficialTotal|TestEntityARAPUsesConfidenceAwareWording|TestReconciliationDoesNotOverclaimSettlement' -v`
Expected: PASS.

### Task 3: Generalize Internal-Party Detection For HR Cash Breakdown

**Files:**
- Create: `internal/query/internal_party.go`
- Modify: `internal/query/engine.go`
- Test: `tests/unit/query/entity_routing_test.go`

- [ ] **Step 1: Write failing tests for generic internal-party branch transfer detection**

Add tests covering:
- branch transfers identified via internal-party resolver, not exact branch strings;
- real-style `1221 -> 1002` internal transfers can still be surfaced separately in HR cash output when evidence suggests payroll-related internal movement;
- non-payroll internal transfers do not get mislabeled as wages.

- [ ] **Step 2: Run target tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestHRBreakdownListsBranchTransferSeparatelyWhenVoucherHasPayrollLiability|TestHRBreakdownDetectsPayrollRelatedInternalTransferWithoutHardcodedBranchName|TestHRBreakdownDoesNotTreatGenericInternalTransferAsPayroll' -v`
Expected: FAIL because current logic is too narrow.

- [ ] **Step 3: Implement internal-party resolver and integrate it**

Add reusable internal-party detection using generic lexical/account evidence and use it in HR cash classification.

- [ ] **Step 4: Run target tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestHRBreakdownListsBranchTransferSeparatelyWhenVoucherHasPayrollLiability|TestHRBreakdownDetectsPayrollRelatedInternalTransferWithoutHardcodedBranchName|TestHRBreakdownDoesNotTreatGenericInternalTransferAsPayroll' -v`
Expected: PASS.

### Task 4: Preserve Real-Data Regressions And Reports

**Files:**
- Modify: `tests/integration/realdata_question_suites_test.go`
- Modify: `tests/testdata/user19_questions_2026-04-20.json`
- Modify: `tests/scripts/run_realdata_question_suite.go`
- Test: real-data scripts and integration suite

- [ ] **Step 1: Update regression expectations only where wording semantics intentionally changed**

Keep question suites stable unless a question must be sharpened to verify the new confidence-aware behavior.

- [ ] **Step 2: Run integration suite**

Run: `/opt/homebrew/bin/go test ./tests/integration -run TestRealdataQuestionSuites`
Expected: PASS.

- [ ] **Step 3: Run real-data reports**

Run:
- `./tests/scripts/run_user15_realdata_check.sh`
- `./tests/scripts/run_user19_realdata_check.sh`
- `./tests/scripts/run_top20_realdata_check.sh`
Expected: all PASS and reports regenerate successfully.

- [ ] **Step 4: Run focused unit suite as final verification**

Run: `/opt/homebrew/bin/go test ./tests/unit/query ./tests/unit/analysis`
Expected: PASS.
