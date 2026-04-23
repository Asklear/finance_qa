package query

import "strings"

func (e *Engine) collectVoucherSiblingEvidence(contexts []voucherContext) []LedgerEvidence {
	if len(contexts) == 0 {
		return nil
	}

	const chunkSize = 80
	evidence := make([]LedgerEvidence, 0, len(contexts)*2)
	for start := 0; start < len(contexts); start += chunkSize {
		end := start + chunkSize
		if end > len(contexts) {
			end = len(contexts)
		}
		chunk := contexts[start:end]

		var clause strings.Builder
		args := make([]any, 0, 2+len(chunk)*3)
		args = append(args, e.Company, e.Company)
		clause.WriteString(`
SELECT IFNULL(counterparty, ''), account_code, account_name, summary, direction,
       COALESCE(debit_amount, 0), COALESCE(credit_amount, 0), voucher_date, IFNULL(voucher_no, '')
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND (
`)
		for i, ctx := range chunk {
			if i > 0 {
				clause.WriteString(" OR ")
			}
			clause.WriteString("(IFNULL(period, '') = ? AND voucher_date = ? AND IFNULL(voucher_no, '') = ?)")
			args = append(args, ctx.Period, ctx.VoucherDate, ctx.VoucherNo)
		}
		clause.WriteString(")")

		rows, err := e.db.Query(clause.String(), args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var counterparty, accountCode, accountName, summary, direction, voucherDate, voucherNo string
			var debitAmt, creditAmt float64
			if scanErr := rows.Scan(&counterparty, &accountCode, &accountName, &summary, &direction, &debitAmt, &creditAmt, &voucherDate, &voucherNo); scanErr != nil {
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
		_ = rows.Close()
	}
	return evidence
}
