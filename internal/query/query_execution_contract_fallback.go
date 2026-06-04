package query

import (
	"fmt"
	"strings"
)

func (e *Engine) tryExplicitContractFallback(ctx queryExecutionContext, contractFailure Result) (Result, bool) {
	if !(ctx.spec.NeedsContractDimension || ctx.spec.QueryFamily == QueryFamilyContractDimension) {
		return Result{}, false
	}
	if !ctx.hasRealEntity || !containsAny(ctx.q, counterpartyMetricKeywords(ctx.cfg)) {
		return Result{}, false
	}

	fallback := e.queryCounterpartyAmountFallback(ctx.q, ctx.entity, ctx.from, ctx.to)
	if containsAny(ctx.q, []string{"应收", "应付", "应收账款", "应付账款"}) {
		fallback = e.queryARAP(ctx.q, ctx.entity, ctx.from, ctx.to)
	}
	if !fallback.Success {
		return Result{}, false
	}

	reason := strings.TrimSpace(contractFailure.Message)
	if reason == "" {
		reason = "项目台账当前不能直接回答该问题"
	}
	if fallback.Data == nil {
		fallback.Data = map[string]any{}
	}
	applyContractFallbackSourceAttribution(fallback.Data)
	fallback.Data["contract_fallback_reason"] = reason
	fallback.Data["contract_fallback_target"] = "counterparty_financial_or_cash"
	fallback.Message = fmt.Sprintf("项目台账当前不能直接回答（%s），本次已回退到财务账/流水口径。\n%s", reason, fallback.Message)
	fallback.CalculationLogs = append([]string{
		fmt.Sprintf("[合同显式回退] reason=%s target=counterparty_financial_or_cash", reason),
	}, fallback.CalculationLogs...)
	return fallback, true
}

func applyContractFallbackSourceAttribution(data map[string]any) {
	source := strings.TrimSpace(anyToString(data["source"]))
	var primary []string
	var supporting []string
	switch {
	case strings.Contains(source, "journal"):
		primary = []string{"fin_journal"}
		supporting = []string{"fin_balance_detail"}
	case strings.Contains(source, "balance_sheet"):
		primary = []string{"fin_balance_sheet"}
		supporting = []string{"fin_balance_detail"}
	case strings.Contains(source, "bank"):
		primary = []string{"fin_bank_statement"}
	}
	if len(primary) == 0 {
		return
	}
	data["source_primary_tables"] = primary
	data["source_supporting_tables"] = supporting
	data["source_tables"] = dedupeSourceTables(append(append([]string{}, primary...), supporting...)...)
	data["executed_source"] = "financial_or_cash_fallback"
}
