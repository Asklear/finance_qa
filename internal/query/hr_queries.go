package query

import (
	"fmt"
	"strings"
)

func (e *Engine) queryHRCost(from, to string) Result {
	return e.queryHRBreakdown(from, to)
}

func (e *Engine) queryHRBreakdown(from, to string) Result {
	cacheKey := strings.TrimSpace(e.Company) + "|" + from + "|" + to
	e.cacheMu.RLock()
	if cached, ok := e.hrBreakdownCache[cacheKey]; ok {
		e.cacheMu.RUnlock()
		result := cloneResult(cached)
		result.CalculationLogs = append(result.CalculationLogs, "[缓存] hr_breakdown cache_hit")
		return result
	}
	e.cacheMu.RUnlock()

	start := from + "-01"
	end := monthEndDay(to)
	periodLabel := displayPeriod(from, to)

	accountSQL := `
SELECT
  COALESCE(SUM(CASE WHEN direction='借' AND account_code IN ('66020101','66022301') THEN COALESCE(debit_amount,0) ELSE 0 END),0) AS wage,
  COALESCE(SUM(CASE WHEN direction='借' AND account_code IN ('66020102','66022302') THEN COALESCE(debit_amount,0) ELSE 0 END),0) AS social,
  COALESCE(SUM(CASE WHEN direction='借' AND account_code IN ('66020103','66022303') THEN COALESCE(debit_amount,0) ELSE 0 END),0) AS housing
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
`
	cashSQL := `
SELECT
  COALESCE(SUM(CASE WHEN account_code LIKE '1002%' AND direction='贷' AND summary LIKE '%工资%' THEN COALESCE(credit_amount,0) ELSE 0 END),0) AS wage,
  COALESCE(SUM(CASE WHEN account_code LIKE '1002%' AND direction='贷' AND (summary LIKE '%社保%' OR summary LIKE '%社保扣款%') THEN COALESCE(credit_amount,0) ELSE 0 END),0) AS social,
  COALESCE(SUM(CASE WHEN account_code LIKE '1002%' AND direction='贷' AND summary LIKE '%公积金%' THEN COALESCE(credit_amount,0) ELSE 0 END),0) AS housing,
  0 AS branch_transfer
FROM journal
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND voucher_date BETWEEN ? AND ?
`
	if e.journalHasVoucherGrouping() {
		cashSQL = `
SELECT
  COALESCE(SUM(CASE WHEN (base.account_code LIKE '1001%' OR base.account_code LIKE '1002%') AND base.direction='贷' AND (
    NOT (base.summary LIKE '%分公司%' OR IFNULL(base.counterparty,'') LIKE '%分公司%') AND (
      base.summary LIKE '%工资%' OR
      base.summary LIKE '%薪酬%' OR
      EXISTS (
        SELECT 1 FROM journal sibling
        WHERE (? LIKE '%' || sibling.company || '%' OR sibling.company LIKE '%' || ? || '%')
          AND sibling.voucher_date = base.voucher_date
          AND sibling.voucher_no = base.voucher_no
          AND sibling.direction = '借'
          AND (sibling.account_code LIKE '2211%' OR sibling.account_name LIKE '%应付职工薪酬%')
          AND (sibling.summary LIKE '%工资%' OR sibling.summary LIKE '%薪酬%' OR sibling.account_name LIKE '%工资%' OR sibling.account_name LIKE '%薪酬%')
      )
    )
  ) THEN COALESCE(base.credit_amount,0) ELSE 0 END),0) AS wage,
  COALESCE(SUM(CASE WHEN (base.account_code LIKE '1001%' OR base.account_code LIKE '1002%') AND base.direction='贷' AND (
    base.summary LIKE '%社保%' OR
    base.summary LIKE '%社保扣款%' OR
    EXISTS (
      SELECT 1 FROM journal sibling
      WHERE (? LIKE '%' || sibling.company || '%' OR sibling.company LIKE '%' || ? || '%')
        AND sibling.voucher_date = base.voucher_date
        AND sibling.voucher_no = base.voucher_no
        AND sibling.direction = '借'
        AND (sibling.account_code LIKE '2211%' OR sibling.account_name LIKE '%应付职工薪酬%')
        AND (sibling.summary LIKE '%社保%' OR sibling.account_name LIKE '%社保%')
    )
  ) THEN COALESCE(base.credit_amount,0) ELSE 0 END),0) AS social,
  COALESCE(SUM(CASE WHEN (base.account_code LIKE '1001%' OR base.account_code LIKE '1002%') AND base.direction='贷' AND (
    base.summary LIKE '%公积金%' OR
    EXISTS (
      SELECT 1 FROM journal sibling
      WHERE (? LIKE '%' || sibling.company || '%' OR sibling.company LIKE '%' || ? || '%')
        AND sibling.voucher_date = base.voucher_date
        AND sibling.voucher_no = base.voucher_no
        AND sibling.direction = '借'
        AND (sibling.account_code LIKE '2211%' OR sibling.account_name LIKE '%应付职工薪酬%')
        AND (sibling.summary LIKE '%公积金%' OR sibling.account_name LIKE '%公积金%')
    )
  ) THEN COALESCE(base.credit_amount,0) ELSE 0 END),0) AS housing
FROM journal base
WHERE (? LIKE '%' || base.company || '%' OR base.company LIKE '%' || ? || '%')
  AND base.voucher_date BETWEEN ? AND ?
`
	}

	var accountWage, accountSocial, accountHousing float64
	var cashWage, cashSocial, cashHousing, cashBranchTransfer float64
	branchTransferSQL := ""
	branchTransferLogs := []string{"[分公司内部转账] voucher grouping unavailable"}
	e.db.QueryRow(accountSQL, e.Company, e.Company, start, end).Scan(&accountWage, &accountSocial, &accountHousing)
	if e.journalHasVoucherGrouping() {
		e.db.QueryRow(
			cashSQL,
			e.Company, e.Company,
			e.Company, e.Company,
			e.Company, e.Company,
			e.Company, e.Company, start, end,
		).Scan(&cashWage, &cashSocial, &cashHousing)
		cashBranchTransfer, branchTransferSQL, branchTransferLogs = e.detectInternalBranchTransferCash(start, end)
	} else {
		e.db.QueryRow(cashSQL, e.Company, e.Company, start, end).Scan(&cashWage, &cashSocial, &cashHousing, &cashBranchTransfer)
	}

	accountTotal := round2(accountWage + accountSocial + accountHousing)
	cashTotal := round2(cashWage + cashSocial + cashHousing + cashBranchTransfer)
	usedFallback := false
	if accountTotal == 0 {
		var fallbackTotal float64
		e.db.QueryRow(`
SELECT COALESCE(SUM(closing_balance), 0)
FROM balance_sheet
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period = ?
  AND (account_name LIKE '%应付职工薪酬%' OR account_code LIKE '2211%')
`, e.Company, e.Company, to).Scan(&fallbackTotal)
		if fallbackTotal > 0 {
			accountTotal = round2(fallbackTotal)
			usedFallback = true
		}
	}

	result := Result{
		Success: true,
		Message: fmt.Sprintf("%s 人力成本（账上）%.2f 元；银行卡实际支出 %.2f 元。工资/社保/公积金已拆分返回。", periodLabel, accountTotal, cashTotal),
		Data: map[string]any{
			"period": periodLabel,
			"total":  accountTotal,
			"hr_breakdown": map[string]any{
				"accounting": map[string]any{
					"工资":  round2(accountWage),
					"社保":  round2(accountSocial),
					"公积金": round2(accountHousing),
					"合计":  accountTotal,
				},
				"cash": map[string]any{
					"工资":      round2(cashWage),
					"社保":      round2(cashSocial),
					"公积金":     round2(cashHousing),
					"分公司内部转账": round2(cashBranchTransfer),
					"合计":      cashTotal,
				},
			},
		},
		ExecutedSQL: []string{
			fmt.Sprintf("queryHRBreakdown(accounting): %s [args: %s, %s, %s]", accountSQL, e.Company, start, end),
			fmt.Sprintf("queryHRBreakdown(cash): %s [args: %s, %s, %s]", cashSQL, e.Company, start, end),
			fmt.Sprintf("queryHRBreakdown(branch_transfer): %s [args: %s, %s, %s]", branchTransferSQL, e.Company, start, end),
		},
		CalculationLogs: []string{
			fmt.Sprintf("[人力成本-账上] 工资=%.2f 社保=%.2f 公积金=%.2f 合计=%.2f", round2(accountWage), round2(accountSocial), round2(accountHousing), accountTotal),
			fmt.Sprintf("[人力成本-现金] 工资=%.2f 社保=%.2f 公积金=%.2f 分公司内部转账=%.2f 合计=%.2f", round2(cashWage), round2(cashSocial), round2(cashHousing), round2(cashBranchTransfer), cashTotal),
			fmt.Sprintf("[兜底触发] %v", usedFallback),
			strings.Join(branchTransferLogs, " | "),
		},
	}
	e.cacheMu.Lock()
	e.hrBreakdownCache[cacheKey] = cloneResult(result)
	e.cacheMu.Unlock()
	return result
}

func (e *Engine) journalHasVoucherGrouping() bool {
	cols := e.tableColumns("journal")
	return cols["voucher_date"] && cols["voucher_no"]
}

func (e *Engine) tableColumns(table string) map[string]bool {
	normalizedTable := strings.ToLower(strings.TrimSpace(table))
	e.cacheMu.RLock()
	if cached, ok := e.tableColumnCache[normalizedTable]; ok {
		out := make(map[string]bool, len(cached))
		for k, v := range cached {
			out[k] = v
		}
		e.cacheMu.RUnlock()
		return out
	}
	e.cacheMu.RUnlock()

	rows, err := e.db.Query(fmt.Sprintf("SELECT * FROM %s LIMIT 0", table))
	if err != nil {
		return map[string]bool{}
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(cols))
	for _, col := range cols {
		out[strings.ToLower(strings.TrimSpace(col))] = true
	}
	e.cacheMu.Lock()
	e.tableColumnCache[normalizedTable] = out
	e.cacheMu.Unlock()
	return out
}
