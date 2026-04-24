package query

import "strings"

func sourceTablesForTaxQuery() []string {
	return []string{"fin_journal"}
}

func sourceTablesForLargeBankTransaction() []string {
	return []string{"fin_bank_statement"}
}

func sourceTablesForPreciseBalance() []string {
	return []string{"fin_balance_sheet", "fin_journal"}
}

func sourceTablesForReadiness(summary readinessSummary) []string {
	tables := make([]string, 0, 5)
	if summary.JournalRows > 0 {
		tables = append(tables, "fin_journal")
	}
	if summary.BankRows > 0 {
		tables = append(tables, "fin_bank_statement")
	}
	if summary.ContractRows > 0 {
		tables = append(tables, "fin_contracts")
	}
	if summary.ContractFundRows > 0 {
		tables = append(tables, "fin_fund_income")
	}
	if summary.ContractCostRows > 0 {
		tables = append(tables, "fin_cost_settlements")
	}
	if len(tables) == 0 {
		tables = append(tables, "fin_journal", "fin_bank_statement", "fin_contracts", "fin_fund_income", "fin_cost_settlements")
	}
	return dedupeSourceTables(tables...)
}

func sourceTablesForReconciliation(bookSource string) []string {
	tables := []string{"fin_bank_statement", "fin_journal"}
	if !strings.Contains(strings.TrimSpace(bookSource), "journal") {
		tables = append([]string{"fin_income_statement"}, tables...)
	}
	return dedupeSourceTables(tables...)
}

func sourceTablesForCoreMetric(accrualSource string, includeCash bool) []string {
	tables := make([]string, 0, 3)
	if strings.Contains(strings.TrimSpace(accrualSource), "journal") {
		tables = append(tables, "fin_journal")
	} else {
		tables = append(tables, "fin_income_statement")
	}
	if includeCash {
		tables = append(tables, "fin_bank_statement")
	}
	return dedupeSourceTables(tables...)
}

func sourceTablesForCounterparty(question string) []string {
	q := strings.TrimSpace(question)
	if isCounterpartyClassificationQuestion(q) {
		return []string{"fin_journal", "fin_bank_statement"}
	}
	if containsAny(q, []string{"回款", "到账", "收款"}) &&
		!containsAny(q, []string{"收入", "营收", "销售额", "成本", "费用", "利润", "客户", "供应商", "混合"}) {
		return []string{"fin_bank_statement"}
	}
	return []string{"fin_journal", "fin_bank_statement"}
}
