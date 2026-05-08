package query

import (
	"fmt"
	"strings"
)

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
	cfg := e.currentRuleConfig()
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
		return false, "unknown_organization_without_evidence"
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
