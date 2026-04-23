package query

import "strings"

func inferContractAskedTopic(question string) string {
	q := strings.TrimSpace(question)
	switch {
	case containsAny(q, []string{"内容", "合同内容", "是什么"}):
		return "content"
	case containsAny(q, []string{"利润", "毛利", "净利"}):
		return "profit"
	case containsAny(q, []string{"营收", "收入", "销售额"}):
		return "revenue"
	case containsAny(q, []string{"成本", "支出"}):
		return "cost"
	case containsAny(q, []string{"回款", "到账", "收款"}):
		return "receipts"
	case containsAny(q, []string{"付款", "支付"}):
		return "payments"
	default:
		return "generic"
	}
}

func contractSourceTablesForRole(role string) []string {
	return getRuleConfig().ContractSourceTables(role)
}

func (e *Engine) hasContractDimensionEntity(entity string) bool {
	like := "%" + strings.TrimSpace(entity) + "%"
	if like == "%%" {
		return false
	}
	var exists int
	e.db.QueryRow(`SELECT 1 FROM fin_contracts WHERE customer_name LIKE ? OR contract_content LIKE ? LIMIT 1`, like, like).Scan(&exists)
	return exists == 1
}

func (e *Engine) shouldPrioritizeContractQuery(question, entity string, hasRealEntity bool) bool {
	if shouldUseContractDimension(question) {
		return true
	}
	if !isContractPriorityQuestion(question) {
		return false
	}
	if hasRealEntity && e.hasContractDimensionEntity(entity) {
		return true
	}
	matched := e.matchContractSubjectByName(question)
	return matched != ""
}
