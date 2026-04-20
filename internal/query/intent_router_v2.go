package query

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// IntentTrace captures explainable routing details for Intent Router V2.
type IntentTrace struct {
	RouterVersion string             `json:"router_version"`
	Matched       []string           `json:"matched"`
	Scores        map[string]float64 `json:"scores"`
	FinalIntent   string             `json:"final_intent"`
	Confidence    float64            `json:"confidence"`
}

// ClassifyIntentV2 returns the routed intent and the trace contract for diagnostics.
func ClassifyIntentV2(question string) (Intent, IntentTrace) {
	return classifyIntentV2(question, getRuleConfig())
}

func classifyIntentV2(question string, cfg RuleConfig) (Intent, IntentTrace) {
	q := strings.ReplaceAll(question, " ", "")
	scores := map[string]float64{}
	matched := make([]string, 0, 8)

	addScore := func(intent Intent, score float64, rule string) {
		scores[string(intent)] += score
		matched = append(matched, rule)
	}

	for intentKey, phrases := range cfg.HighPriorityPhrases {
		if containsAny(q, phrases) {
			scores[intentKey] += 5.0
			matched = append(matched, "high_priority:"+intentKey)
		}
	}

	if containsAny(q, cfg.IntentKeywords(IntentLargeTransactionQuery)) {
		addScore(IntentLargeTransactionQuery, 3.2, "large_transaction")
	}
	if containsAny(q, cfg.IntentKeywords(IntentIdentityQuery)) {
		addScore(IntentIdentityQuery, 3.0, "identity")
	}
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) {
		addScore(IntentFallback, 2.8, "hr_cost_fallback")
	}
	if containsAny(q, cfg.intentKeywordGroup(string(IntentARAPQuery))) {
		addScore(IntentARAPQuery, 2.8, "arap")
	}
	if containsAny(q, cfg.intentKeywordGroup(string(IntentTaxQuery))) {
		addScore(IntentTaxQuery, 2.8, "tax")
	}
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHealth)) {
		addScore(IntentFallback, 2.4, "health_fallback")
	}
	if containsAny(q, cfg.intentKeywordGroup(string(IntentAnalysis))) {
		addScore(IntentAnalysis, 2.2, "analysis")
	}
	if containsAny(q, cfg.intentKeywordGroup(string(IntentFallback))) {
		addScore(IntentFallback, 2.1, "fallback_keyword")
	}
	if containsAny(q, cfg.intentKeywordGroup(string(IntentHostPayload))) {
		addScore(IntentHostPayload, 2.0, "host_payload")
	}
	if strings.Contains(q, "项目") && containsAny(q, []string{"收入", "成本", "支出", "应收", "应付", "数据出来"}) {
		addScore(IntentFallback, 2.0, "project_fallback")
	}
	if containsAny(q, cfg.intentKeywordGroup(string(IntentMonthlySummary))) {
		addScore(IntentMonthlySummary, 2.0, "monthly_summary")
	}
	if containsAny(q, cfg.IntentKeywords(IntentPrecise)) {
		addScore(IntentPrecise, 1.8, "precise")
	}

	if len(scores) == 0 {
		scores[string(IntentGeneral)] = 1.0
		matched = append(matched, "default_general")
	}

	finalIntent, confidence := resolveIntentWithConfidence(scores, cfg.IntentPriority)
	if conflicts, ok := cfg.IntentConflicts[string(finalIntent)]; ok && len(conflicts) > 0 {
		for _, loser := range conflicts {
			if _, exists := scores[loser]; exists {
				delete(scores, loser)
				matched = append(matched, "conflict_drop:"+loser)
			}
		}
		finalIntent, confidence = resolveIntentWithConfidence(scores, cfg.IntentPriority)
	}
	if min, ok := cfg.IntentMinConfidence[string(finalIntent)]; ok && confidence < min {
		matched = append(matched, "confidence_fallback:"+strconv.FormatFloat(confidence, 'f', 3, 64))
		finalIntent = IntentFallback
	}

	trace := IntentTrace{
		RouterVersion: "v2",
		Matched:       dedupeStrings(matched),
		Scores:        scores,
		FinalIntent:   string(finalIntent),
		Confidence:    confidence,
	}
	return finalIntent, trace
}

func resolveIntentWithConfidence(scores map[string]float64, priorities map[string]int) (Intent, float64) {
	type kv struct {
		intent string
		score  float64
	}
	items := make([]kv, 0, len(scores))
	total := 0.0
	for intent, score := range scores {
		items = append(items, kv{intent: intent, score: score})
		total += score
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return intentPriority(items[i].intent, priorities) > intentPriority(items[j].intent, priorities)
		}
		return items[i].score > items[j].score
	})

	if len(items) == 0 {
		return IntentGeneral, 1.0
	}
	best := items[0]
	if total <= 0 {
		return Intent(best.intent), 0
	}
	confidence := math.Round((best.score/total)*1000) / 1000
	return Intent(best.intent), confidence
}

func intentPriority(intent string, priorities map[string]int) int {
	if priorities != nil {
		if p, ok := priorities[intent]; ok {
			return p
		}
	}
	return 0
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
