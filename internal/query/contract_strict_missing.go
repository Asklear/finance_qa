package query

import (
	"fmt"
	"strings"
)

func shouldStopAtStrictContractSource(ctx queryExecutionContext) bool {
	if shouldAllowContractMissingFinancialFallback(ctx) {
		return false
	}
	return shouldUseStrictContractSourceForSpec(ctx.spec)
}

func shouldAllowContractMissingFinancialFallback(ctx queryExecutionContext) bool {
	if !ctx.hasRealEntity {
		return false
	}
	if shouldUseContractDetailQuestion(ctx.q) || inferContractAskedTopic(ctx.q) == "content" {
		return false
	}
	return containsAny(ctx.q, counterpartyMetricKeywords(ctx.cfg))
}

func shouldUseStrictContractSourceForSpec(spec QuerySpec) bool {
	if spec.BossRewrite.Perspective == BossPerspectiveExplicitCash ||
		spec.BossRewrite.Perspective == BossPerspectiveFinancialAccount ||
		spec.BossRewrite.Perspective == BossPerspectiveOfficialThenEvidence ||
		spec.SourceConstraint == BossSourceBankStatement {
		return false
	}
	return spec.NeedsContractDimension ||
		spec.QueryFamily == QueryFamilyContractDimension ||
		spec.PreferContractAggregate ||
		spec.SourceConstraint == BossSourceContractAggregate ||
		spec.RouteDecision.SelectedSource == BossSourceContractAggregate
}

func buildStrictContractMissingResult(ctx queryExecutionContext, failure Result) Result {
	reason := strings.TrimSpace(failure.Message)
	result := buildStrictContractMissingResultForSpec(ctx.spec, reason, nil, failure.ExecutedSQL, failure.CalculationLogs)
	if ctx.engine != nil {
		result = ctx.engine.enrichStrictContractMissingWithContinuityCandidates(ctx.spec, result)
	}
	return result
}

func buildStrictContractMissingResultForSpec(spec QuerySpec, reason string, sourceTables []string, executedSQL, calculationLogs []string) Result {
	rawReason := strings.TrimSpace(reason)
	reason = normalizeStrictContractMissingReason(rawReason)
	if reason == "" {
		reason = "合同/项目台账在请求期间没有足够记录"
	}
	tables := contractStrictSourceTablesForSpec(spec, sourceTables)
	period := displayPeriod(spec.PeriodFrom, spec.PeriodTo)
	if strings.TrimSpace(period) == "" {
		period = "请求期间"
	}
	entityPrefix := ""
	if entity := strings.TrimSpace(spec.Entity); entity != "" {
		entityPrefix = fmt.Sprintf("[%s] ", entity)
	}
	message := fmt.Sprintf("%s%s 合同口径当前不能直接回答：%s。系统已停止自动回退到财务账/银行流水，避免把非老板口径当成合同口径；如需查看非合同口径，请明确说“账上/科目余额/资产负债表/序时账/银行流水/实际到账/实际支出”。", entityPrefix, period, reason)
	if len(executedSQL) == 0 {
		executedSQL = []string{"contract_strict_missing: contract source probe/lookup did not provide full coverage; financial/cash fallback intentionally blocked"}
	}
	if len(calculationLogs) == 0 {
		calculationLogs = []string{fmt.Sprintf("[合同严格口径] blocked_non_contract_fallback=true reason=%s", reason)}
	} else {
		calculationLogs = append([]string{fmt.Sprintf("[合同严格口径] blocked_non_contract_fallback=true reason=%s", reason)}, calculationLogs...)
	}
	return Result{
		Success: true,
		Message: message,
		Data: map[string]any{
			"contract_answer_status":   "missing",
			"contract_source_required": true,
			"contract_fallback_reason": reason,
			"contract_raw_error":       rawReason,
			"source_priority":          "contract_strict",
			"source_tables":            tables,
			"source_primary_tables":    tables,
			"requested_metrics":        detectRequestedMetrics(spec.OriginalQuestion),
			"period":                   period,
			"period_from":              spec.PeriodFrom,
			"period_to":                spec.PeriodTo,
			"entity":                   strings.TrimSpace(spec.Entity),
		},
		ExecutedSQL:     executedSQL,
		CalculationLogs: calculationLogs,
	}
}

func contractStrictSourceTablesForSpec(spec QuerySpec, sourceTables []string) []string {
	if len(sourceTables) > 0 {
		return dedupeSourceTables(sourceTables...)
	}
	if len(spec.RouteDecision.PrimaryTables) > 0 {
		return dedupeSourceTables(spec.RouteDecision.PrimaryTables...)
	}
	for _, probe := range spec.RouteDecision.ProbeResults {
		if len(probe.PrimaryTables) > 0 {
			return dedupeSourceTables(probe.PrimaryTables...)
		}
	}
	if inferContractAskedTopic(spec.OriginalQuestion) == "content" {
		return contractSourceTablesForRole("contract_content")
	}
	if tables := contractAggregateSourceTablesForMetrics(detectRequestedMetrics(spec.OriginalQuestion)); len(tables) > 0 {
		return tables
	}
	return getRuleConfig().ContractSourceTables(contractAggregateRole)
}

func normalizeStrictContractMissingReason(reason string) string {
	reason = strings.TrimSpace(strings.ReplaceAll(reason, "，已回退到现金+经营/财务口径", ""))
	switch strings.TrimSpace(reason) {
	case "contract entity not found":
		return "没有识别到合同/项目主体"
	case "contract not found":
		return "合同信息表没有匹配到该主体/项目"
	case "contract role not found":
		return "合同收入/成本结算表在请求期间没有可用于判断收入侧或成本侧的匹配记录"
	default:
		return strings.TrimSpace(reason)
	}
}
