package query_test

import (
	"encoding/json"
	"testing"
	"time"

	"financeqa/internal/query"
)

func TestExtractPeriodWithNow(t *testing.T) {
	now := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		question string
		wantFrom string
		wantTo   string
	}{
		{name: "explicit year month", question: "2026年2月收入是多少", wantFrom: "2026-02", wantTo: "2026-02"},
		{name: "range", question: "2026年1月到2026年2月支出", wantFrom: "2026-01", wantTo: "2026-02"},
		{name: "month only current year", question: "2月收入", wantFrom: "2026-02", wantTo: "2026-02"},
		{name: "month only previous year", question: "12月收入", wantFrom: "2025-12", wantTo: "2025-12"},
		{name: "explicit year cumulative", question: "飞未云科2026年累计销售额多少", wantFrom: "2026-01", wantTo: "2026-04"},
		{name: "explicit year quarter chinese", question: "2026年第一季度营收", wantFrom: "2026-01", wantTo: "2026-03"},
		{name: "explicit year quarter q-format", question: "2026年Q1收入", wantFrom: "2026-01", wantTo: "2026-03"},
		{name: "relative quarter without year", question: "第一季度收入", wantFrom: "2026-01", wantTo: "2026-03"},
		{name: "explicit year first half", question: "2026年上半年营收", wantFrom: "2026-01", wantTo: "2026-06"},
		{name: "explicit year second half", question: "2026年下半年营收", wantFrom: "2026-07", wantTo: "2026-12"},
		{name: "relative first half", question: "上半年营收", wantFrom: "2026-01", wantTo: "2026-06"},
		{name: "relative second half", question: "下半年营收", wantFrom: "2025-07", wantTo: "2025-12"},
		{name: "explicit year full year", question: "2026年全年营收", wantFrom: "2026-01", wantTo: "2026-12"},
		{name: "relative this year full year", question: "今年全年营收", wantFrom: "2026-01", wantTo: "2026-12"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, to := query.ExtractPeriodWithNow(tc.question, now)
			if from != tc.wantFrom || to != tc.wantTo {
				t.Fatalf("ExtractPeriodWithNow(%q) = (%s,%s), want (%s,%s)", tc.question, from, to, tc.wantFrom, tc.wantTo)
			}
		})
	}
}

func TestResolveCompany(t *testing.T) {
	available := []string{"模拟财务科技有限公司", "模拟财务", "苏州示例"}
	got := query.ResolveCompany("模拟财务", available)
	if got != "模拟财务科技有限公司" {
		t.Fatalf("ResolveCompany() = %q, want %q", got, "模拟财务科技有限公司")
	}
}

func TestClassifyIntent(t *testing.T) {
	cases := []struct {
		question string
		want     query.Intent
	}{
		{question: "2月收入是多少", want: query.IntentMonthlySummary},
		{question: "2026年第一季度经营概览", want: query.IntentMonthlySummary},
		{question: "2月增值税是多少", want: query.IntentTaxQuery},
		{question: "2月应收账款情况", want: query.IntentARAPQuery},
		{question: "2月账龄分析", want: query.IntentAnalysis},
		{question: "供应商多少", want: query.IntentFallback},
		{question: "公司财务健康度怎么", want: query.IntentFallback},
		{question: "货币资金余额是多少", want: query.IntentPrecise},
	}

	for _, tc := range cases {
		if got := query.ClassifyIntent(tc.question); got != tc.want {
			t.Fatalf("ClassifyIntent(%q) = %q, want %q", tc.question, got, tc.want)
		}
	}
}

func TestBuildQuerySpecTreatsOverviewAsCoreMetricSummary(t *testing.T) {
	now := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)

	spec := query.BuildQuerySpec("2026年第一季度经营概览", now)
	if spec.Intent != query.IntentMonthlySummary {
		t.Fatalf("Intent = %q, want %q", spec.Intent, query.IntentMonthlySummary)
	}
	if spec.QueryFamily != query.QueryFamilyCoreMetric {
		t.Fatalf("QueryFamily = %q, want %q", spec.QueryFamily, query.QueryFamilyCoreMetric)
	}
	if spec.PeriodFrom != "2026-01" || spec.PeriodTo != "2026-03" {
		t.Fatalf("period = %s~%s, want 2026-01~2026-03", spec.PeriodFrom, spec.PeriodTo)
	}
}

func TestClassifyIntentV2TraceContract(t *testing.T) {
	intent, tr := query.ClassifyIntentV2("汇智在2026年2月这笔是供应商付款还是预收款？")
	if intent == "" {
		t.Fatalf("expected non-empty intent")
	}

	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal trace: %v", err)
	}

	for _, k := range []string{"router_version", "matched", "scores", "final_intent", "confidence"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing trace key=%s", k)
		}
	}
	if tr.FinalIntent != string(intent) {
		t.Fatalf("trace final_intent=%q does not match returned intent=%q", tr.FinalIntent, intent)
	}
	if tr.Confidence < 0 || tr.Confidence > 1 {
		t.Fatalf("unexpected confidence=%f", tr.Confidence)
	}
	if len(tr.Scores) == 0 {
		t.Fatalf("expected non-empty scores")
	}
}

func TestIntentRouterConfigurableLargeTransactionKeywords(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "router": {
    "intents": {
      "large_transaction": {
        "keywords": ["峰值来款"]
      }
    }
  }
}`)
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	intent, _ := query.ClassifyIntentV2("3月峰值来款是谁")
	if intent != query.IntentLargeTransactionQuery {
		t.Fatalf("ClassifyIntentV2 with configurable large transaction keywords = %s, want %s", intent, query.IntentLargeTransactionQuery)
	}
}

func TestIntentRouterConfigurableIdentityKeywords(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "router": {
    "intents": {
      "identity": {
        "keywords": ["什么来头"]
      }
    }
  }
}`)
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	intent, _ := query.ClassifyIntentV2("汇智什么来头")
	if intent != query.IntentIdentityQuery {
		t.Fatalf("ClassifyIntentV2 with configurable identity keywords = %s, want %s", intent, query.IntentIdentityQuery)
	}
}

func TestIntentRouterConfigurablePreciseKeywords(t *testing.T) {
	rulesPath := writeRulesConfigFile(t, `{
  "schema_version": 2,
  "router": {
    "intents": {
      "precise": {
        "keywords": ["结余存量"]
      }
    }
  }
}`)
	t.Setenv("FINANCEQA_RULES_PATH", rulesPath)

	intent, _ := query.ClassifyIntentV2("货币资金结余存量")
	if intent != query.IntentPrecise {
		t.Fatalf("ClassifyIntentV2 with configurable precise keywords = %s, want %s", intent, query.IntentPrecise)
	}
}

func TestBuildQuerySpecCapturesSubPeriodAndCashFirstPolicy(t *testing.T) {
	now := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)

	spec := query.BuildQuerySpec("金程今年回款多少？其中3月到账多少？", now)

	if spec.QueryFamily != query.QueryFamilyCounterparty {
		t.Fatalf("QueryFamily = %s, want %s", spec.QueryFamily, query.QueryFamilyCounterparty)
	}
	if spec.PeriodFrom != "2026-01" || spec.PeriodTo != "2026-04" {
		t.Fatalf("period = %s~%s, want 2026-01~2026-04", spec.PeriodFrom, spec.PeriodTo)
	}
	if spec.SubPeriod != "2026-03" {
		t.Fatalf("SubPeriod = %s, want 2026-03", spec.SubPeriod)
	}
	if spec.TimeScope != query.TimeScopeYearToDate {
		t.Fatalf("TimeScope = %s, want %s", spec.TimeScope, query.TimeScopeYearToDate)
	}
	if spec.PerspectivePolicy != query.PerspectiveCashThenAccrual {
		t.Fatalf("PerspectivePolicy = %s, want %s", spec.PerspectivePolicy, query.PerspectiveCashThenAccrual)
	}
	if spec.Entity == "" {
		t.Fatalf("expected entity to be extracted")
	}
	if spec.LexiconProfile == "" {
		t.Fatalf("expected non-empty lexicon profile")
	}
}

func TestBuildQuerySpecCapturesReadinessAndOpeningSemantics(t *testing.T) {
	now := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)

	readiness := query.BuildQuerySpec("飞未3月数据出来了吗？", now)
	if readiness.QueryFamily != query.QueryFamilyReadiness {
		t.Fatalf("readiness QueryFamily = %s, want %s", readiness.QueryFamily, query.QueryFamilyReadiness)
	}
	if !readiness.ReadinessCheckRequired {
		t.Fatalf("expected ReadinessCheckRequired=true")
	}
	if readiness.PeriodFrom != "2026-03" || readiness.PeriodTo != "2026-03" {
		t.Fatalf("readiness period = %s~%s, want 2026-03~2026-03", readiness.PeriodFrom, readiness.PeriodTo)
	}

	arap := query.BuildQuerySpec("2026年3月账上应付账款多少（已收发票未付款）？", now)
	if arap.QueryFamily != query.QueryFamilyARAP {
		t.Fatalf("ARAP QueryFamily = %s, want %s", arap.QueryFamily, query.QueryFamilyARAP)
	}
	if !arap.OpeningPeriodAware {
		t.Fatalf("expected OpeningPeriodAware=true")
	}
	if !arap.AuthoritativeSourceRequired {
		t.Fatalf("expected AuthoritativeSourceRequired=true")
	}
	if arap.PerspectivePolicy != query.PerspectiveOfficialThenEvidence {
		t.Fatalf("PerspectivePolicy = %s, want %s", arap.PerspectivePolicy, query.PerspectiveOfficialThenEvidence)
	}
}

func TestBuildQuerySpecCapturesContractDimension(t *testing.T) {
	now := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)

	spec := query.BuildQuerySpec("飞未合同2026年回款多少？其中3月到账多少？", now)

	if spec.QueryFamily != query.QueryFamilyContractDimension {
		t.Fatalf("QueryFamily = %s, want %s", spec.QueryFamily, query.QueryFamilyContractDimension)
	}
	if !spec.NeedsContractDimension {
		t.Fatalf("expected NeedsContractDimension=true")
	}
	if spec.SubPeriod != "2026-03" {
		t.Fatalf("SubPeriod = %s, want 2026-03", spec.SubPeriod)
	}
	if spec.PerspectivePolicy != query.PerspectiveCashThenAccrual {
		t.Fatalf("PerspectivePolicy = %s, want %s", spec.PerspectivePolicy, query.PerspectiveCashThenAccrual)
	}
}

func TestPlanQuerySpecUsesMetricSpecificSourceStrategies(t *testing.T) {
	now := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)

	revenuePlan := query.PlanQuerySpec(query.BuildQuerySpec("2026年3月收入是多少？", now))
	if revenuePlan.QueryFamily != query.QueryFamilyCoreMetric {
		t.Fatalf("revenue QueryFamily = %s, want %s", revenuePlan.QueryFamily, query.QueryFamilyCoreMetric)
	}
	if !revenuePlan.Requires(query.SourceCapabilityContractLedger) || !revenuePlan.Requires(query.SourceCapabilityCashReceipts) || !revenuePlan.Requires(query.SourceCapabilityAccrualRevenue) {
		t.Fatalf("revenue plan capabilities = %+v, want contract ledger + cash receipts + accrual revenue", revenuePlan.Capabilities)
	}

	arapPlan := query.PlanQuerySpec(query.BuildQuerySpec("2026年3月账上应收账款多少？", now))
	if !arapPlan.Requires(query.SourceCapabilityOfficialARAP) || !arapPlan.Requires(query.SourceCapabilityOpenItemEvidence) {
		t.Fatalf("AR/AP plan capabilities = %+v, want official balance + open-item evidence", arapPlan.Capabilities)
	}

	readinessPlan := query.PlanQuerySpec(query.BuildQuerySpec("飞未3月数据出来了吗？", now))
	if !readinessPlan.Requires(query.SourceCapabilityDataReadiness) {
		t.Fatalf("readiness plan capabilities = %+v, want data readiness", readinessPlan.Capabilities)
	}

	contractPlan := query.PlanQuerySpec(query.BuildQuerySpec("飞未合同2026年回款多少？", now))
	if !contractPlan.Requires(query.SourceCapabilityContractLedger) || !contractPlan.Requires(query.SourceCapabilityBankCashReceipts) {
		t.Fatalf("contract plan capabilities = %+v, want contract ledger + bank cash receipts", contractPlan.Capabilities)
	}
}

func TestBuildQuerySpecMarksBossAggregateContractPriority(t *testing.T) {
	now := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)

	spec := query.BuildQuerySpec("2026年第一季度收入、成本、利润分别是多少？", now)
	if spec.QueryFamily != query.QueryFamilyCoreMetric {
		t.Fatalf("QueryFamily = %s, want %s", spec.QueryFamily, query.QueryFamilyCoreMetric)
	}
	if !spec.PreferContractAggregate {
		t.Fatalf("PreferContractAggregate = false, want true")
	}
	if spec.NeedsContractDimension {
		t.Fatalf("NeedsContractDimension = true, want false")
	}

	cashSpec := query.BuildQuerySpec("2026年3月银行卡上实际到账、实际支出、净增加分别是多少？", now)
	if cashSpec.PreferContractAggregate {
		t.Fatalf("cash flow question should not prefer contract aggregate")
	}

	receiptsSpec := query.BuildQuerySpec("金程今年回款多少？其中3月到账多少？", now)
	if receiptsSpec.PreferContractAggregate {
		t.Fatalf("receipts question should not prefer contract aggregate")
	}
}
