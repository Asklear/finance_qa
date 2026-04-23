# Keyword Config Convergence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把查询层关键词与阈值配置收敛到 `config/rules.json`，并在不回归现有财务逻辑的前提下完成旧 schema 兼容与调用点迁移。

**Architecture:** 先在 `rules_config.go` 引入兼容旧平铺 schema 和新嵌套 schema 的加载层，再增加统一 lexicon accessor，最后逐步替换 router/engine/classifier/internal_party 内部的硬编码 literal 关键词。财务判断和证据机制继续保留在代码中，只迁 literal 词表与阈值。

**Tech Stack:** Go, JSON config loading, existing query engine, Go unit/integration tests, real-data regression scripts.

---

### Task 1: Add Nested Rules Schema Loader With Backward Compatibility

**Files:**
- Modify: `internal/query/rules_config.go`
- Test: `tests/unit/query/rules_config_test.go`

- [ ] **Step 1: Write the failing test**

Add tests covering:
- nested `schema_version=2` file load
- legacy flat file load still works
- nested config takes effect for at least one router keyword and one role threshold

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestRulesConfigLoadsNestedSchema|TestRulesConfigStillLoadsLegacyFlatSchema' -v`
Expected: FAIL because nested schema parsing is not implemented yet.

- [ ] **Step 3: Write minimal implementation**

In `internal/query/rules_config.go`:
- add nested raw config structs for `router`, `counterparty`, `internal_party`
- map nested schema into runtime `RuleConfig`
- preserve legacy flat parsing behavior
- keep env override order unchanged

- [ ] **Step 4: Run test to verify it passes**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestRulesConfigLoadsNestedSchema|TestRulesConfigStillLoadsLegacyFlatSchema' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query/rules_config.go tests/unit/query/rules_config_test.go
git commit -m "refactor: add nested rules schema compatibility"
```

### Task 2: Add Unified Lexicon Accessors

**Files:**
- Create: `internal/query/rule_lexicon.go`
- Modify: `internal/query/rules_config.go`
- Test: `tests/unit/query/rules_config_test.go`

- [ ] **Step 1: Write the failing test**

Add tests for accessor helpers such as:
- intent keyword lookup
- metric keyword lookup
- HR breakdown keyword lookup
- counterparty role keyword lookup
- internal party suffix lookup

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestRuleLexiconAccessors' -v`
Expected: FAIL because accessors do not exist.

- [ ] **Step 3: Write minimal implementation**

Create `internal/query/rule_lexicon.go` and expose helpers that wrap `RuleConfig` access without leaking file schema details into callers.

- [ ] **Step 4: Run test to verify it passes**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestRuleLexiconAccessors' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query/rule_lexicon.go internal/query/rules_config.go tests/unit/query/rules_config_test.go
git commit -m "refactor: add query lexicon accessors"
```

### Task 3: Migrate Router Literal Keywords To Config Accessors

**Files:**
- Modify: `internal/query/intent_router_v2.go`
- Modify: `internal/query/helpers.go`
- Modify: `internal/query/rules_config.go`
- Test: `tests/unit/query/intent_period_company_test.go`
- Test: `tests/unit/query/rules_config_test.go`

- [ ] **Step 1: Write the failing test**

Add tests that prove the following can be changed through config:
- large transaction keywords
- identity keywords
- precise balance keywords

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestIntentRouterConfigurableLargeTransactionKeywords|TestIntentRouterConfigurableIdentityKeywords|TestIntentRouterConfigurablePreciseKeywords' -v`
Expected: FAIL because these keyword groups are still hardcoded.

- [ ] **Step 3: Write minimal implementation**

Replace hardcoded literal arrays in `intent_router_v2.go` and `helpers.go` with config-backed accessor calls.

- [ ] **Step 4: Run test to verify it passes**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestIntentRouterConfigurableLargeTransactionKeywords|TestIntentRouterConfigurableIdentityKeywords|TestIntentRouterConfigurablePreciseKeywords' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query/intent_router_v2.go internal/query/helpers.go internal/query/rules_config.go tests/unit/query/intent_period_company_test.go tests/unit/query/rules_config_test.go
git commit -m "refactor: move router literal keywords into rules config"
```

### Task 4: Migrate Engine-Level Literal Question Keywords To Config

**Files:**
- Modify: `internal/query/engine.go`
- Modify: `internal/query/rules_config.go`
- Test: `tests/unit/query/entity_routing_test.go`
- Test: `tests/integration/reconciliation_analysis_test.go`

- [ ] **Step 1: Write the failing test**

Add tests verifying config can override:
- metric detection keywords
- HR breakdown keywords
- counterparty classification question keywords
- profit single-view block keywords

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestEngineUsesConfigurableMetricKeywords|TestEngineUsesConfigurableHRBreakdownKeywords|TestEngineUsesConfigurableCounterpartyClassificationKeywords|TestEngineUsesConfigurableProfitSingleViewBlockKeywords' -v`
Expected: FAIL because these keyword groups are still hardcoded in engine helpers.

- [ ] **Step 3: Write minimal implementation**

Update `engine.go` helper functions to use config-backed lexicon accessors while keeping behavior identical under default config.

- [ ] **Step 4: Run test to verify it passes**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestEngineUsesConfigurableMetricKeywords|TestEngineUsesConfigurableHRBreakdownKeywords|TestEngineUsesConfigurableCounterpartyClassificationKeywords|TestEngineUsesConfigurableProfitSingleViewBlockKeywords' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query/engine.go internal/query/rules_config.go tests/unit/query/entity_routing_test.go tests/integration/reconciliation_analysis_test.go
git commit -m "refactor: move engine literal question keywords into rules config"
```

### Task 5: Migrate Counterparty Role And Tax Dictionaries To Config

**Files:**
- Modify: `internal/query/counterparty_classifier.go`
- Modify: `internal/query/rules_config.go`
- Test: `tests/unit/query/counterparty_classifier_test.go`

- [ ] **Step 1: Write the failing test**

Add tests proving custom config can override:
- customer role keywords
- supplier role keywords
- employee role keywords
- tax output/input keywords

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestCounterpartyClassifierUsesConfigurableRoleLexicon|TestCounterpartyClassifierUsesConfigurableTaxLexicon' -v`
Expected: FAIL because these dictionaries are still package-level hardcoded slices.

- [ ] **Step 3: Write minimal implementation**

Move role/tax dictionaries behind config accessors and keep threshold logic unchanged.

- [ ] **Step 4: Run test to verify it passes**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestCounterpartyClassifierUsesConfigurableRoleLexicon|TestCounterpartyClassifierUsesConfigurableTaxLexicon' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query/counterparty_classifier.go internal/query/rules_config.go tests/unit/query/counterparty_classifier_test.go
git commit -m "refactor: move counterparty role dictionaries into rules config"
```

### Task 6: Migrate Internal Party Lexicon To Config

**Files:**
- Modify: `internal/query/internal_party.go`
- Modify: `internal/query/rules_config.go`
- Test: `tests/unit/query/entity_routing_test.go`

- [ ] **Step 1: Write the failing test**

Add tests proving custom config can override:
- internal organization suffixes
- internal account context keywords

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestInternalPartyUsesConfigurableOrgSuffixes|TestInternalPartyUsesConfigurableAccountContextKeywords' -v`
Expected: FAIL because `internal_party.go` still hardcodes these terms.

- [ ] **Step 3: Write minimal implementation**

Replace internal organization literal dictionaries with config-backed accessors, while preserving voucher-level inference logic.

- [ ] **Step 4: Run test to verify it passes**

Run: `/opt/homebrew/bin/go test ./tests/unit/query -run 'TestInternalPartyUsesConfigurableOrgSuffixes|TestInternalPartyUsesConfigurableAccountContextKeywords' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query/internal_party.go internal/query/rules_config.go tests/unit/query/entity_routing_test.go
git commit -m "refactor: move internal party lexicon into rules config"
```

### Task 7: Freeze Legacy Keywords Manager And Document Single Config Source

**Files:**
- Modify: `internal/config/keywords_manager.go`
- Modify: `README.md`
- Modify: `SKILL.md`
- Test: `tests/unit/config/keywords_manager_test.go`

- [ ] **Step 1: Write the failing test**

Add or update tests/documentation expectations so the query stack officially points to `rules.json` as the primary keyword source and `keywords_manager` is treated as legacy.

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./tests/unit/config -v`
Expected: Either docs assertions fail or no explicit legacy note exists yet.

- [ ] **Step 3: Write minimal implementation**

- Add clear legacy/deprecated comments in `keywords_manager.go`
- Update README and operator guidance to point new query rules to `config/rules.json`

- [ ] **Step 4: Run test to verify it passes**

Run: `/opt/homebrew/bin/go test ./tests/unit/config -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/keywords_manager.go README.md SKILL.md tests/unit/config/keywords_manager_test.go
git commit -m "docs: declare rules.json as primary query config source"
```

### Task 8: Run Full Regression

**Files:**
- Modify: none unless regression failures require targeted fixes
- Test: `tests/unit/query`
- Test: `tests/integration`
- Test: `tests/scripts`

- [ ] **Step 1: Run focused unit tests**

Run:
```bash
/opt/homebrew/bin/go test ./tests/unit/query -v
```
Expected: PASS.

- [ ] **Step 2: Run integration tests**

Run:
```bash
/opt/homebrew/bin/go test ./tests/integration -v
```
Expected: PASS or explicit SKIP for missing external fixtures.

- [ ] **Step 3: Run real-data regression suites**

Run:
```bash
./tests/scripts/run_user15_realdata_check.sh
./tests/scripts/run_user19_realdata_check.sh
./tests/scripts/run_top20_realdata_check.sh
```
Expected: all suites pass and regenerate reports.

- [ ] **Step 4: Run repository-wide verification**

Run:
```bash
/opt/homebrew/bin/go test ./...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "refactor: converge query keyword configuration"
```
