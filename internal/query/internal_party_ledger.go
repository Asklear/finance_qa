package query

import (
	"fmt"
	"strings"
)

type voucherLedgerRow struct {
	VoucherDate  string
	VoucherNo    string
	AccountCode  string
	AccountName  string
	Direction    string
	Summary      string
	Counterparty string
	DebitAmount  float64
	CreditAmount float64
}

func (e *Engine) detectInternalBranchTransferCash(start, end string) (float64, string, []string) {
	cacheKey := strings.TrimSpace(e.Company) + "|" + start + "|" + end
	e.cacheMu.RLock()
	if cached, ok := e.branchTransferCache[cacheKey]; ok {
		e.cacheMu.RUnlock()
		logs := append([]string{}, cached.logs...)
		logs = append(logs, "[缓存] branch_transfer cache_hit")
		return cached.total, cached.query, logs
	}
	e.cacheMu.RUnlock()

	cfg := getRuleConfig()
	query := internalBranchTransferLedgerQuery()
	rows, err := e.db.Query(query, e.Company, e.Company, start, end)
	if err != nil {
		return 0, query, []string{fmt.Sprintf("[分公司内部转账] query error=%v", err)}
	}
	defer rows.Close()

	groups := make(map[string][]voucherLedgerRow)
	index := 0
	for rows.Next() {
		var row voucherLedgerRow
		if err := rows.Scan(
			&row.VoucherDate,
			&row.VoucherNo,
			&row.AccountCode,
			&row.AccountName,
			&row.Direction,
			&row.Summary,
			&row.Counterparty,
			&row.DebitAmount,
			&row.CreditAmount,
		); err != nil {
			return 0, query, []string{fmt.Sprintf("[分公司内部转账] scan error=%v", err)}
		}
		key := ledgerVoucherGroupKey(row, index)
		groups[key] = append(groups[key], row)
		index++
	}
	if err := rows.Err(); err != nil {
		return 0, query, []string{fmt.Sprintf("[分公司内部转账] iterate error=%v", err)}
	}

	total := 0.0
	logs := make([]string, 0)
	for key, group := range groups {
		if !voucherHasInternalSettlementDebit(group, cfg) {
			continue
		}
		internalParty, basis := inferInternalPartyFromVoucher(e.Company, group, cfg)
		if internalParty == "" {
			continue
		}
		amount := 0.0
		for _, row := range group {
			if isBankCreditVoucherRow(row) {
				amount += row.CreditAmount
			}
		}
		amount = round2(amount)
		if amount <= 0 {
			continue
		}
		total += amount
		logs = append(logs, fmt.Sprintf("[分公司内部转账] voucher=%s party=%s basis=%s amount=%.2f", displayVoucherGroupKey(key, group), internalParty, basis, amount))
	}

	if len(logs) == 0 {
		logs = append(logs, "[分公司内部转账] no matched internal transfer vouchers")
	}
	total = round2(total)
	e.cacheMu.Lock()
	e.branchTransferCache[cacheKey] = cachedBranchTransfer{
		total: total,
		query: query,
		logs:  append([]string{}, logs...),
	}
	e.cacheMu.Unlock()
	return total, query, logs
}

func internalBranchTransferLedgerQuery() string {
	selectColumns := []string{
		ledgerDateSelectExpr("voucher_date"),
		ledgerTextSelectExpr("voucher_no"),
		ledgerTextSelectExpr("account_code"),
		ledgerTextSelectExpr("account_name"),
		ledgerTextSelectExpr("direction"),
		ledgerTextSelectExpr("summary"),
		ledgerTextSelectExpr("counterparty"),
		"COALESCE(debit_amount, 0)",
		"COALESCE(credit_amount, 0)",
	}
	return `
SELECT
  ` + strings.Join(selectColumns, ",\n  ") + `
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
ORDER BY ` + ledgerVoucherOrderByClause() + `
`
}

func ledgerTextSelectExpr(column string) string {
	return "COALESCE(TRIM(" + column + "), '')"
}

func ledgerDateSelectExpr(column string) string {
	return "COALESCE(CAST(" + column + " AS TEXT), '')"
}

func ledgerVoucherOrderByClause() string {
	return strings.Join([]string{
		"voucher_date",
		"COALESCE(NULLIF(TRIM(voucher_no), ''), '')",
		"account_code",
		"COALESCE(NULLIF(TRIM(account_name), ''), '')",
		"COALESCE(NULLIF(TRIM(summary), ''), '')",
		"COALESCE(NULLIF(TRIM(counterparty), ''), '')",
		"COALESCE(debit_amount, 0)",
		"COALESCE(credit_amount, 0)",
	}, ", ")
}

func ledgerVoucherGroupKey(row voucherLedgerRow, index int) string {
	if strings.TrimSpace(row.VoucherNo) != "" {
		return row.VoucherDate + "|" + row.VoucherNo
	}
	return fmt.Sprintf("%s|row-%d", row.VoucherDate, index)
}

func displayVoucherGroupKey(key string, group []voucherLedgerRow) string {
	if len(group) == 0 {
		return key
	}
	if strings.TrimSpace(group[0].VoucherNo) != "" {
		return group[0].VoucherDate + "/" + group[0].VoucherNo
	}
	return group[0].VoucherDate + "/(no-voucher-no)"
}

func isBankCreditVoucherRow(row voucherLedgerRow) bool {
	if !(strings.HasPrefix(row.AccountCode, "1001") || strings.HasPrefix(row.AccountCode, "1002")) {
		return false
	}
	if row.CreditAmount <= 0 {
		return false
	}
	direction := strings.TrimSpace(row.Direction)
	return direction == "" || direction == "贷"
}

func voucherHasInternalSettlementDebit(rows []voucherLedgerRow, cfg RuleConfig) bool {
	for _, row := range rows {
		if row.DebitAmount <= 0 {
			continue
		}
		if strings.TrimSpace(row.Direction) != "" && strings.TrimSpace(row.Direction) != "借" {
			continue
		}
		switch {
		case strings.HasPrefix(row.AccountCode, "2211"):
			return true
		case strings.HasPrefix(row.AccountCode, "1221"):
			return true
		case strings.HasPrefix(row.AccountCode, "2241"):
			return true
		}
		text := normalizeEntityText(row.AccountName + row.Summary)
		if hasAny(text, cfg.InternalPartyAccountContextKeywords()) {
			return true
		}
	}
	return false
}
