package query

import "testing"

func TestDeriveQuerySpecPolicyForARAP(t *testing.T) {
	spec := QuerySpec{
		NormalizedQuestion:     "2026年3月应付账款多少（已收发票未付款）？",
		Intent:                 IntentARAPQuery,
		QueryFamily:            QueryFamilyARAP,
		MetricKind:             MetricKindUnknown,
		NeedsContractDimension: false,
	}

	policy := deriveQuerySpecPolicy(spec, getRuleConfig())
	if !policy.AuthoritativeSourceRequired {
		t.Fatalf("AuthoritativeSourceRequired = false, want true")
	}
	if !policy.OpeningPeriodAware {
		t.Fatalf("OpeningPeriodAware = false, want true")
	}
	if policy.PerspectivePolicy != PerspectiveOfficialThenEvidence {
		t.Fatalf("PerspectivePolicy = %s, want %s", policy.PerspectivePolicy, PerspectiveOfficialThenEvidence)
	}
	if policy.PreferContractAggregate {
		t.Fatalf("PreferContractAggregate = true, want false")
	}
}

func TestDeriveQuerySpecPolicyForReadiness(t *testing.T) {
	spec := QuerySpec{
		NormalizedQuestion:     "飞未3月数据出来了吗？",
		Intent:                 IntentGeneral,
		QueryFamily:            QueryFamilyReadiness,
		MetricKind:             MetricKindUnknown,
		NeedsContractDimension: false,
	}

	policy := deriveQuerySpecPolicy(spec, getRuleConfig())
	if !policy.ReadinessCheckRequired {
		t.Fatalf("ReadinessCheckRequired = false, want true")
	}
	if policy.PreferContractAggregate {
		t.Fatalf("PreferContractAggregate = true, want false")
	}
}

func TestDeriveQuerySpecPolicyForContractDimension(t *testing.T) {
	spec := QuerySpec{
		NormalizedQuestion:     "飞未合同2026年回款多少？其中3月到账多少？",
		Intent:                 IntentFallback,
		QueryFamily:            QueryFamilyContractDimension,
		MetricKind:             MetricKindReceipts,
		NeedsContractDimension: false,
	}

	policy := deriveQuerySpecPolicy(spec, getRuleConfig())
	if !policy.NeedsContractDimension {
		t.Fatalf("NeedsContractDimension = false, want true")
	}
	if policy.PerspectivePolicy != PerspectiveCashThenAccrual {
		t.Fatalf("PerspectivePolicy = %s, want %s", policy.PerspectivePolicy, PerspectiveCashThenAccrual)
	}
	if policy.PreferContractAggregate {
		t.Fatalf("PreferContractAggregate = true, want false")
	}
}

func TestDeriveQuerySpecPolicyForAggregateCoreMetric(t *testing.T) {
	spec := QuerySpec{
		NormalizedQuestion:     "2026年第一季度收入、成本、利润分别是多少？",
		Intent:                 IntentMonthlySummary,
		QueryFamily:            QueryFamilyCoreMetric,
		MetricKind:             MetricKindRevenue,
		NeedsContractDimension: false,
	}

	policy := deriveQuerySpecPolicy(spec, getRuleConfig())
	if !policy.PreferContractAggregate {
		t.Fatalf("PreferContractAggregate = false, want true")
	}
	if policy.NeedsContractDimension {
		t.Fatalf("NeedsContractDimension = true, want false")
	}
	if policy.PerspectivePolicy != PerspectiveCashThenAccrual {
		t.Fatalf("PerspectivePolicy = %s, want %s", policy.PerspectivePolicy, PerspectiveCashThenAccrual)
	}
}

func TestApplyQuerySpecPolicyCopiesDerivedFlags(t *testing.T) {
	spec := QuerySpec{
		QueryFamily: QueryFamilyContractDimension,
	}
	policy := QuerySpecPolicy{
		PerspectivePolicy:           PerspectiveCashThenAccrual,
		NeedsContractDimension:      true,
		PreferContractAggregate:     false,
		ReadinessCheckRequired:      false,
		AuthoritativeSourceRequired: false,
		OpeningPeriodAware:          false,
	}

	got := applyQuerySpecPolicy(spec, policy)
	if got.PerspectivePolicy != PerspectiveCashThenAccrual {
		t.Fatalf("PerspectivePolicy = %s, want %s", got.PerspectivePolicy, PerspectiveCashThenAccrual)
	}
	if !got.NeedsContractDimension {
		t.Fatalf("NeedsContractDimension = false, want true")
	}
}
