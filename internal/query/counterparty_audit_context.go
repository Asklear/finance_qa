package query

import "fmt"

type counterpartyAuditContext struct {
	question           string
	q                  string
	entity             string
	from               string
	to                 string
	snap               counterpartySnapshot
	evidence           []LedgerEvidence
	classification     CounterpartyClassification
	taxReport          TaxNormalizationReport
	role               string
	roleLabel          string
	periodLabel        string
	receiptPeriodLabel string
	usedRetro          bool
	logs               []string
	sqls               []string
	resultData         map[string]any
}

func buildCounterpartyAuditContext(question, entity, from, to string, snap counterpartySnapshot, classification CounterpartyClassification, taxReport TaxNormalizationReport, evidence []LedgerEvidence, usedRetro bool) counterpartyAuditContext {
	q := NormalizeQuestion(question)
	role := string(classification.Role)
	if role == "" {
		role = snap.Role
	}
	ctx := counterpartyAuditContext{
		question:           question,
		q:                  q,
		entity:             entity,
		from:               from,
		to:                 to,
		snap:               snap,
		evidence:           evidence,
		classification:     classification,
		taxReport:          taxReport,
		role:               role,
		roleLabel:          fmt.Sprintf("（识别为[%s]）", role),
		periodLabel:        displayPeriod(from, to),
		receiptPeriodLabel: displayReceiptPeriodLabel(q, from, to),
		usedRetro:          usedRetro,
		logs: []string{
			fmt.Sprintf("[对手方识别] entity=%s role=%s confidence=%.3f signals=%v", entity, role, classification.Confidence, classification.Signals),
			fmt.Sprintf("[往来快照] bank_in=%.2f bank_out=%.2f revenue_net=%.2f cost=%.2f expense=%.2f output_vat=%.2f input_vat=%.2f basis=%s", snap.BankIn, snap.BankOut, snap.RevenueNet, snap.BookCost, snap.BookExpense, snap.OutputVAT, snap.InputVAT, snap.ComparisonBasis),
			TraceTaxNormalization(taxReport),
		},
		sqls: []string{
			"counterparty(bank_statement): SELECT counterparty_name, summary, debit_amount, credit_amount FROM bank_statement WHERE ... AND counterparty_name LIKE ?",
			"counterparty(journal): SELECT counterparty, account_code, account_name, summary, direction, debit_amount, credit_amount FROM journal WHERE ... AND (summary LIKE ? OR counterparty LIKE ?)",
		},
		resultData: map[string]any{
			"entity":            entity,
			"role":              role,
			"bank_in":           round2(snap.BankIn),
			"bank_out":          round2(snap.BankOut),
			"revenue_net":       round2(snap.RevenueNet),
			"book_cost":         round2(snap.BookCost),
			"book_expense":      round2(snap.BookExpense),
			"output_vat":        round2(snap.OutputVAT),
			"input_vat":         round2(snap.InputVAT),
			"difference_reason": snap.DifferenceReason,
			"comparison_basis":  snap.ComparisonBasis,
			"evidence":          evidence,
			"tax_breakdown":     taxReport,
			"tax_basis": map[string]any{
				"cash_receipts":      "gross_bank_cash",
				"book_revenue":       "net_of_output_vat",
				"book_cost":          "book_entry_basis",
				"comparison_note":    "银行回款默认按含税到账口径，账上收入默认按不含税确认口径；跨口径比较时要先说明税基。",
				"comparison_allowed": snap.ComparisonBasis == "vat_gap_only",
			},
		},
	}
	if usedRetro {
		ctx.logs = append(ctx.logs, fmt.Sprintf("[年度回溯] %s 当月无记录，已回溯到 %s~%s", entity, from[:4]+"-01", to))
	}
	return ctx
}

func (ctx counterpartyAuditContext) cloneResultData() map[string]any {
	return cloneAnyMap(ctx.resultData)
}

func (ctx counterpartyAuditContext) cloneLogs() []string {
	return append([]string{}, ctx.logs...)
}

func (ctx counterpartyAuditContext) cloneSQLs() []string {
	return append([]string{}, ctx.sqls...)
}
