package query

import "strings"

func (e *Engine) collectLikeMatchedCounterpartyEvidence(name, startDate, endDate string) []LedgerEvidence {
	like := "%" + name + "%"
	evidence := make([]LedgerEvidence, 0, 24)
	evidence = append(evidence, e.queryLikeCounterpartyEvidenceFromBank(like, startDate, endDate)...)

	if !e.journalHasVoucherGrouping() {
		evidence = append(evidence, e.queryLikeCounterpartyEvidenceFromJournal(like, startDate, endDate)...)
		return evidence
	}

	journalEvidence, contexts := e.queryLikeCounterpartyEvidenceWithContexts(like, startDate, endDate)
	evidence = append(evidence, journalEvidence...)
	if len(contexts) == 0 {
		return evidence
	}
	return append(evidence, e.collectVoucherSiblingEvidence(contexts)...)
}

func (e *Engine) queryLikeCounterpartyEvidenceFromBank(like, startDate, endDate string) []LedgerEvidence {
	rows, err := e.db.Query(`
SELECT transaction_date, counterparty_name, summary, COALESCE(debit_amount, 0), COALESCE(credit_amount, 0)
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND transaction_date BETWEEN ? AND ?
  AND counterparty_name LIKE ?
`, e.Company, e.Company, startDate, endDate, like)
	if err != nil {
		return nil
	}
	defer rows.Close()

	evidence := make([]LedgerEvidence, 0, 8)
	for rows.Next() {
		var voucherDate, counterparty, summary string
		var debitAmt, creditAmt float64
		if scanErr := rows.Scan(&voucherDate, &counterparty, &summary, &debitAmt, &creditAmt); scanErr != nil {
			continue
		}
		evidence = append(evidence, LedgerEvidence{
			Source:       "bank_statement",
			VoucherDate:  voucherDate,
			Counterparty: counterparty,
			Summary:      summary,
			DebitAmount:  debitAmt,
			CreditAmount: creditAmt,
		})
	}
	return evidence
}

func (e *Engine) queryLikeCounterpartyEvidenceFromJournal(like, startDate, endDate string) []LedgerEvidence {
	rows, err := e.db.Query(`
SELECT IFNULL(counterparty, ''), account_code, account_name, summary, direction, COALESCE(debit_amount, 0), COALESCE(credit_amount, 0)
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
  AND (summary LIKE ? OR IFNULL(counterparty, '') LIKE ?)
`, e.Company, e.Company, startDate, endDate, like, like)
	if err != nil {
		return nil
	}
	defer rows.Close()

	evidence := make([]LedgerEvidence, 0, 12)
	for rows.Next() {
		var counterparty, accountCode, accountName, summary, direction string
		var debitAmt, creditAmt float64
		if scanErr := rows.Scan(&counterparty, &accountCode, &accountName, &summary, &direction, &debitAmt, &creditAmt); scanErr != nil {
			continue
		}
		evidence = append(evidence, LedgerEvidence{
			Source:       "journal",
			Counterparty: counterparty,
			AccountCode:  accountCode,
			AccountName:  accountName,
			Summary:      summary,
			Direction:    direction,
			DebitAmount:  debitAmt,
			CreditAmount: creditAmt,
		})
	}
	return evidence
}

func (e *Engine) queryLikeCounterpartyEvidenceWithContexts(like, startDate, endDate string) ([]LedgerEvidence, []voucherContext) {
	rows, err := e.db.Query(`
SELECT IFNULL(counterparty, ''), account_code, account_name, summary, direction,
       COALESCE(debit_amount, 0), COALESCE(credit_amount, 0), IFNULL(period, ''), voucher_date, IFNULL(voucher_no, '')
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
  AND (summary LIKE ? OR IFNULL(counterparty, '') LIKE ?)
`, e.Company, e.Company, startDate, endDate, like, like)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	evidence := make([]LedgerEvidence, 0, 12)
	contextMap := make(map[string]voucherContext)
	for rows.Next() {
		var counterparty, accountCode, accountName, summary, direction, period, voucherDate, voucherNo string
		var debitAmt, creditAmt float64
		if scanErr := rows.Scan(&counterparty, &accountCode, &accountName, &summary, &direction, &debitAmt, &creditAmt, &period, &voucherDate, &voucherNo); scanErr != nil {
			continue
		}
		evidence = append(evidence, LedgerEvidence{
			Source:       "journal",
			VoucherDate:  voucherDate,
			VoucherNo:    voucherNo,
			Counterparty: counterparty,
			AccountCode:  accountCode,
			AccountName:  accountName,
			Summary:      summary,
			Direction:    direction,
			DebitAmount:  debitAmt,
			CreditAmount: creditAmt,
		})
		if voucherDate == "" || voucherNo == "" {
			continue
		}
		ctxKey := strings.Join([]string{period, voucherDate, voucherNo}, "\x1f")
		contextMap[ctxKey] = voucherContext{Period: period, VoucherDate: voucherDate, VoucherNo: voucherNo}
	}

	contexts := make([]voucherContext, 0, len(contextMap))
	for _, ctx := range contextMap {
		contexts = append(contexts, ctx)
	}
	return evidence, contexts
}
