package query

import (
	"fmt"
	"strings"
)

func (e *Engine) queryFallback(q, from, to, err string) Result {
	if r := e.ruleFallback(q, from, to); r.Success {
		return r
	}

	accounts := e.availableAccounts(to)
	samples := e.counterpartySamples()
	entity := e.extractNamedEntity(q)
	logs := []string{fmt.Sprintf("[识别] fallback实体识别结果: %s", entity)}
	payload := e.buildHostLLMPayload(from, to, q)
	return Result{
		Success:      false,
		Message:      "指令语义模糊",
		AnswerMethod: "llm_payload",
		Data: map[string]any{
			"fallback_attempted":  true,
			"hint":                "请给出更具体的问题，例如“某年某月应收账款多少”或“某客户某月回款多少”",
			"available_accounts":  accounts,
			"counterparty_sample": samples,
			"llm_payload":         payload,
		},
		CalculationLogs: logs,
	}
}

func (e *Engine) ruleFallback(q, from, to string) Result {
	cfg := getRuleConfig()
	entity := e.extractNamedEntity(q)
	hasRealEntity := e.isRealBusinessEntity(q, entity)
	if isIntervalCoreMetricQuestion(q, entity, hasRealEntity, from, to) || shouldPreferCoreMetricSummary(q, entity, hasRealEntity, from, to) {
		return e.queryDualPerspectiveForCoreMetric(q, from, to)
	}
	if strings.Contains(q, "供应商") && strings.Contains(q, "多少") {
		return e.querySupplierPayments(from, to)
	}
	if containsAny(q, cfg.intentKeywordGroup(routerGroupHRCost)) {
		return e.queryHRBreakdown(from, to)
	}
	if containsAny(q, cfg.FallbackMonthlyExpenseKeywords) {
		return e.queryMonthlyExpenseFromBank(from, to)
	}
	if entity != "" && strings.Contains(q, "数据出来") {
		return e.queryEntityDataReady(entity, from, to)
	}
	if entity != "" && strings.Contains(q, "项目") && containsAny(q, []string{"收入", "成本", "支出"}) {
		return e.queryProjectIncomeCost(entity, from, to, q)
	}
	if entity != "" {
		return e.queryCounterpartyAmountFallback(q, entity, from, to)
	}
	return Result{Success: false}
}

func (e *Engine) queryMonthlyExpenseFromBank(from, to string) Result {
	var total float64
	sqlTxt := `SELECT COALESCE(SUM(debit_amount), 0) FROM bank_statement WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%') AND transaction_date BETWEEN ? AND ?`
	e.db.QueryRow(sqlTxt, e.Company, e.Company, from+"-01", monthEndDay(to)).Scan(&total)
	return Result{
		Success: true,
		Message: fmt.Sprintf("%s 整体支出 %.2f 元（按银行卡实际支出统计）", displayPeriod(from, to), total),
		Data: map[string]any{
			"period": displayPeriod(from, to),
			"total":  total,
		},
		ExecutedSQL:     []string{fmt.Sprintf("queryMonthlyExpenseFromBank: %s [args: %s, %s, %s]", sqlTxt, e.Company, from+"-01", monthEndDay(to))},
		CalculationLogs: []string{fmt.Sprintf("[整体支出] period=%s~%s bank_out=%.2f", from, to, total)},
	}
}

func (e *Engine) querySupplierPayments(from, to string) Result {
	summary, err := e.collectSupplierPaymentSummary(from, to)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	msg := fmt.Sprintf("%s 发生付款的外部供应商共 %d 家，合计 %.2f 元。", summary.Period, len(summary.Suppliers), summary.Total)
	if len(summary.Suppliers) == 0 {
		msg = fmt.Sprintf("%s 暂未识别到外部供应商付款。", summary.Period)
	}

	spec := QuerySpec{
		QueryFamily: QueryFamilySupplierPayments,
		MetricKind:  MetricKindUnknown,
		PeriodFrom:  from,
		PeriodTo:    to,
	}
	factSet := buildSupplierPaymentFactSet(spec, summary)

	return Result{
		Success: true,
		Message: msg,
		Data: map[string]any{
			"period":                  summary.Period,
			"count":                   len(summary.Suppliers),
			"total":                   summary.Total,
			"suppliers":               summary.Suppliers,
			"excluded_counterparties": summary.Excluded,
			"fact_sets":               []FactSet{factSet},
		},
		ExecutedSQL:     summary.ExecutedSQL,
		CalculationLogs: summary.Logs,
	}
}

func (e *Engine) shouldIncludeSupplierPaymentCounterparty(name string, classification CounterpartyClassification) (bool, string) {
	cfg := getRuleConfig()
	switch {
	case looksLikeSupplierPaymentExcludedName(name, cfg):
		return false, "non_counterparty_flow"
	case internalPartyMatchesCompany(e.Company, name) || looksLikeInternalOrgUnit(name, cfg):
		return false, "internal_party"
	case classification.Role == CounterpartyEmployee:
		return false, "employee_related"
	case classification.Role == CounterpartyCustomer:
		return false, "customer_only"
	case classification.Role == CounterpartySupplier:
		return true, "classified_supplier"
	case classification.Role == CounterpartyMixed:
		return true, "classified_mixed"
	case looksLikeExternalOrganizationCounterparty(name):
		return true, "organization_name_fallback"
	default:
		return false, "unknown_non_organization"
	}
}

func looksLikeSupplierPaymentExcludedName(name string, cfg RuleConfig) bool {
	normalized := normalizeEntityText(name)
	if normalized == "" {
		return false
	}
	for _, kw := range cfg.SupplierPaymentExcludeNames() {
		nk := normalizeEntityText(kw)
		if nk != "" && strings.Contains(normalized, nk) {
			return true
		}
	}
	return false
}

func looksLikeExternalOrganizationCounterparty(name string) bool {
	normalized := normalizeEntityText(name)
	if normalized == "" {
		return false
	}
	hints := []string{
		"公司", "有限责任公司", "有限公司", "事务所", "管理中心", "中心",
		"研究院", "合伙企业", "基金会", "协会", "学院", "大学",
	}
	for _, hint := range hints {
		if strings.Contains(normalized, normalizeEntityText(hint)) {
			return true
		}
	}
	return false
}

func (e *Engine) queryProjectIncomeCost(entity, from, to, question string) Result {
	snap := e.buildCounterpartySnapshot(entity, from, to)
	sqlTxt := `SELECT counterparty_name, credit_amount, debit_amount FROM bank_statement WHERE ... AND counterparty_name LIKE ?`
	if strings.Contains(question, "收入") {
		amount := round2(snap.RevenueNet)
		if amount == 0 {
			amount = round2(snap.BankIn)
		}
		return annotateJournalTaxDisclosure(Result{
			Success: true,
			Message: fmt.Sprintf("%s %s 项目收入 %.2f 元", to, entity, amount),
			Data:    map[string]any{"entity": entity, "period": to, "income": amount, "bank_in": round2(snap.BankIn), "revenue_net": round2(snap.RevenueNet)},
			ExecutedSQL: []string{
				fmt.Sprintf("queryProjectIncomeCost: %s [args: %s, %s, %s]", sqlTxt, e.Company, "%"+entity+"%", from+"-01"),
			},
			CalculationLogs: []string{
				fmt.Sprintf("[项目收支] bank_in=%.2f revenue_net=%.2f", snap.BankIn, snap.RevenueNet),
			},
		}, snap.RevenueNet > 0)
	}
	amount := round2(snap.BookCost + snap.BookExpense)
	if amount == 0 {
		amount = round2(snap.BankOut)
	}
	return annotateJournalTaxDisclosure(Result{
		Success: true,
		Message: fmt.Sprintf("%s %s 项目成本 %.2f 元", to, entity, amount),
		Data:    map[string]any{"entity": entity, "period": to, "cost": amount, "bank_out": round2(snap.BankOut), "book_cost": round2(snap.BookCost), "book_expense": round2(snap.BookExpense)},
		ExecutedSQL: []string{
			fmt.Sprintf("queryProjectIncomeCost: %s [args: %s, %s, %s]", sqlTxt, e.Company, "%"+entity+"%", from+"-01"),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[项目收支] bank_out=%.2f book_cost=%.2f book_expense=%.2f", snap.BankOut, snap.BookCost, snap.BookExpense),
		},
	}, snap.BookCost+snap.BookExpense > 0)
}
