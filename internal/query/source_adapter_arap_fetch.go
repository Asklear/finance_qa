package query

import (
	"context"
	"strings"
)

type ARAPSourceAdapter struct {
	runtime ARAPSourceRuntime
}

func NewARAPSourceAdapter(runtime ARAPSourceRuntime) *ARAPSourceAdapter {
	return &ARAPSourceAdapter{runtime: runtime}
}

func (a *ARAPSourceAdapter) Name() string {
	return "arap"
}

func (a *ARAPSourceAdapter) Capabilities() []SourceCapability {
	return []SourceCapability{
		SourceCapabilityOfficialARAP,
		SourceCapabilityOpenItemEvidence,
	}
}

type arapScope struct {
	typ         string
	accountName string
	codePrefix  string
	metricLabel string
}

func (a *ARAPSourceAdapter) Fetch(_ context.Context, spec QuerySpec) (FactSet, error) {
	scopes := detectARAPScopes(spec)
	facts := make([]Fact, 0, len(scopes)*6)
	for _, scope := range scopes {
		result := a.runtime.queryAccountPayableReceivable(spec.PeriodTo, scope.accountName, scope.codePrefix, scope.typ, spec.Entity)
		if result.Success {
			official := result.Data["source"] == "balance_sheet"
			facts = append(facts, buildARAPFactsFromResult(spec, scope, result, official)...)
			if openData, ok := result.Data["open_item_analysis"].(map[string]any); ok {
				facts = append(facts, buildARAPOpenItemFacts(spec, scope, openData)...)
			}
			continue
		}
	}
	return FactSet{Source: a.Name(), Facts: facts}, nil
}

func detectARAPScopes(spec QuerySpec) []arapScope {
	q := spec.NormalizedQuestion
	hasReceivable := strings.Contains(q, "应收")
	hasPayable := strings.Contains(q, "应付")
	scopes := make([]arapScope, 0, 2)
	if hasReceivable || (!hasReceivable && !hasPayable) {
		scopes = append(scopes, arapScope{
			typ:         "receivable",
			accountName: "应收账款",
			codePrefix:  "1122",
			metricLabel: "arap",
		})
	}
	if hasPayable {
		scopes = append(scopes, arapScope{
			typ:         "payable",
			accountName: "应付账款",
			codePrefix:  "2202",
			metricLabel: "arap",
		})
	}
	return scopes
}
