package query

type QuerySpecPolicy struct {
	PerspectivePolicy           PerspectivePolicy
	NeedsContractDimension      bool
	PreferContractAggregate     bool
	ReadinessCheckRequired      bool
	AuthoritativeSourceRequired bool
	OpeningPeriodAware          bool
}

func deriveQuerySpecPolicy(spec QuerySpec, cfg RuleConfig) QuerySpecPolicy {
	policy := QuerySpecPolicy{
		PerspectivePolicy:           detectPerspectivePolicy(spec.NormalizedQuestion, spec.Intent, spec.NeedsContractDimension, cfg),
		NeedsContractDimension:      spec.NeedsContractDimension,
		PreferContractAggregate:     shouldPreferContractAggregate(spec.NormalizedQuestion, spec.Intent, spec.QueryFamily, spec.MetricKind, cfg),
		ReadinessCheckRequired:      containsAny(spec.NormalizedQuestion, []string{"数据出来"}),
		AuthoritativeSourceRequired: isAuthoritativeSourceQuestion(spec.NormalizedQuestion),
		OpeningPeriodAware:          isOpeningPeriodQuestion(spec.NormalizedQuestion),
	}

	switch spec.QueryFamily {
	case QueryFamilyARAP:
		policy.AuthoritativeSourceRequired = true
		policy.OpeningPeriodAware = true
		policy.PerspectivePolicy = PerspectiveOfficialThenEvidence
		policy.PreferContractAggregate = false
	case QueryFamilyReadiness:
		policy.ReadinessCheckRequired = true
		policy.PreferContractAggregate = false
	case QueryFamilyContractDimension:
		policy.NeedsContractDimension = true
		policy.PerspectivePolicy = PerspectiveCashThenAccrual
		policy.PreferContractAggregate = false
	}

	return policy
}

func applyQuerySpecPolicy(spec QuerySpec, policy QuerySpecPolicy) QuerySpec {
	spec.PerspectivePolicy = policy.PerspectivePolicy
	spec.NeedsContractDimension = policy.NeedsContractDimension
	spec.PreferContractAggregate = policy.PreferContractAggregate
	spec.ReadinessCheckRequired = policy.ReadinessCheckRequired
	spec.AuthoritativeSourceRequired = policy.AuthoritativeSourceRequired
	spec.OpeningPeriodAware = policy.OpeningPeriodAware
	return spec
}
