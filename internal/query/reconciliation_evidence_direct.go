package query

import "strings"

func (e *Engine) collectDirectCounterpartyEvidence(names []string, startDate, endDate string) []LedgerEvidence {
	if len(names) == 0 {
		return nil
	}
	evidence := make([]LedgerEvidence, 0, len(names)*4)
	evidence = append(evidence, e.queryExactCounterpartyEvidenceFromBank(names, startDate, endDate)...)
	evidence = append(evidence, e.queryExactCounterpartyEvidenceFromJournal(names, startDate, endDate)...)
	return evidence
}

func (e *Engine) queryExactCounterpartyEvidenceFromBank(names []string, startDate, endDate string) []LedgerEvidence {
	query, args := buildCounterpartyNameClause(`
SELECT transaction_date, counterparty_name, summary, COALESCE(debit_amount, 0), COALESCE(credit_amount, 0)
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND transaction_date BETWEEN ? AND ?
`, e.Company, e.Company, startDate, endDate, "counterparty_name", names)
	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	evidence := make([]LedgerEvidence, 0, len(names)*2)
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

func (e *Engine) queryExactCounterpartyEvidenceFromJournal(names []string, startDate, endDate string) []LedgerEvidence {
	query, args := buildCounterpartyNameClause(`
SELECT IFNULL(counterparty, ''), account_code, account_name, summary, direction,
       COALESCE(debit_amount, 0), COALESCE(credit_amount, 0), voucher_date, IFNULL(voucher_no, ''), IFNULL(period, '')
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
  AND COALESCE(TRIM(counterparty), '') <> ''
`, e.Company, e.Company, startDate, endDate, "counterparty", names)
	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	evidence := make([]LedgerEvidence, 0, len(names)*3)
	for rows.Next() {
		var counterparty, accountCode, accountName, summary, direction, voucherDate, voucherNo, period string
		var debitAmt, creditAmt float64
		if scanErr := rows.Scan(&counterparty, &accountCode, &accountName, &summary, &direction, &debitAmt, &creditAmt, &voucherDate, &voucherNo, &period); scanErr != nil {
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
	}
	return evidence
}

func buildCounterpartyNameClause(baseSQL string, companyArg1, companyArg2, startDate, endDate, column string, names []string) (string, []any) {
	var clause strings.Builder
	args := make([]any, 0, 4+len(names))
	clause.WriteString(baseSQL)
	clause.WriteString("  AND ")
	args = append(args, companyArg1, companyArg2, startDate, endDate)
	if len(names) == 1 {
		clause.WriteString(column + " = ?")
		args = append(args, names[0])
		return clause.String(), args
	}
	clause.WriteString(column + " IN (")
	for i, name := range names {
		if i > 0 {
			clause.WriteString(",")
		}
		clause.WriteString("?")
		args = append(args, name)
	}
	clause.WriteString(")")
	return clause.String(), args
}
