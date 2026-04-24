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
	cfg := getRuleConfig()

	accountSQL := buildHRAccountingQuery(cfg)
	cashSQL := buildHRCashQuery(cfg, false)
	if e.journalHasVoucherGrouping() {
		cashSQL = buildHRCashQuery(cfg, true)
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
		fallbackSQL := fmt.Sprintf(`
SELECT COALESCE(SUM(closing_balance), 0)
	FROM balance_sheet
	WHERE (? LIKE '%%' || company || '%%' OR company LIKE '%%' || ? || '%%')
	  AND period = ?
	  AND (%s OR %s)
`, hrKeywordLikeExpr([]string{"account_name"}, cfg.HRPayrollLiabilityNameKeywords()), hrPrefixLikeExpr("account_code", cfg.HRPayrollLiabilityPrefixes()))
		e.db.QueryRow(fallbackSQL, e.Company, e.Company, to).Scan(&fallbackTotal)
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

func buildHRAccountingQuery(cfg RuleConfig) string {
	return fmt.Sprintf(`
SELECT
  COALESCE(SUM(CASE WHEN direction='借' AND %s THEN COALESCE(debit_amount,0) ELSE 0 END),0) AS wage,
  COALESCE(SUM(CASE WHEN direction='借' AND %s THEN COALESCE(debit_amount,0) ELSE 0 END),0) AS social,
  COALESCE(SUM(CASE WHEN direction='借' AND %s THEN COALESCE(debit_amount,0) ELSE 0 END),0) AS housing
FROM journal
WHERE (? LIKE '%%' || company || '%%' OR company LIKE '%%' || ? || '%%')
  AND voucher_date BETWEEN ? AND ?
`,
		hrInExpr("account_code", cfg.HRBreakdownAccountCodes("wage")),
		hrInExpr("account_code", cfg.HRBreakdownAccountCodes("social")),
		hrInExpr("account_code", cfg.HRBreakdownAccountCodes("housing")),
	)
}

func buildHRCashQuery(cfg RuleConfig, grouped bool) string {
	bankExpr := hrPrefixLikeExpr("base.account_code", cfg.HRCashBankAccountPrefixes())
	if !grouped {
		bankExpr = hrPrefixLikeExpr("account_code", cfg.HRCashBankAccountPrefixes())
		return fmt.Sprintf(`
SELECT
  COALESCE(SUM(CASE WHEN %s AND direction='贷' AND %s THEN COALESCE(credit_amount,0) ELSE 0 END),0) AS wage,
  COALESCE(SUM(CASE WHEN %s AND direction='贷' AND %s THEN COALESCE(credit_amount,0) ELSE 0 END),0) AS social,
  COALESCE(SUM(CASE WHEN %s AND direction='贷' AND %s THEN COALESCE(credit_amount,0) ELSE 0 END),0) AS housing,
  0 AS branch_transfer
FROM journal
WHERE (? LIKE '%%' || company || '%%' OR company LIKE '%%' || ? || '%%')
  AND voucher_date BETWEEN ? AND ?
`,
			bankExpr, hrKeywordLikeExpr([]string{"summary", "account_name"}, cfg.HRCategoryKeywords("wage")),
			bankExpr, hrKeywordLikeExpr([]string{"summary", "account_name"}, cfg.HRCategoryKeywords("social")),
			bankExpr, hrKeywordLikeExpr([]string{"summary", "account_name"}, cfg.HRCategoryKeywords("housing")),
		)
	}

	liabilityExpr := hrJoinOr(
		hrPrefixLikeExpr("sibling.account_code", cfg.HRPayrollLiabilityPrefixes()),
		hrKeywordLikeExpr([]string{"sibling.account_name"}, cfg.HRPayrollLiabilityNameKeywords()),
	)
	notBranchExpr := fmt.Sprintf("NOT (%s)", hrKeywordLikeExpr([]string{"base.summary", "base.counterparty"}, cfg.InternalPartyOrgSuffixes()))

	categoryExpr := func(category string) string {
		baseKeywords := hrKeywordLikeExpr([]string{"base.summary", "base.account_name"}, cfg.HRCategoryKeywords(category))
		siblingKeywords := hrKeywordLikeExpr([]string{"sibling.summary", "sibling.account_name"}, cfg.HRCategoryKeywords(category))
		return fmt.Sprintf(`(
    %s AND (
      %s OR
      EXISTS (
        SELECT 1 FROM journal sibling
        WHERE (? LIKE '%%' || sibling.company || '%%' OR sibling.company LIKE '%%' || ? || '%%')
          AND sibling.voucher_date = base.voucher_date
          AND sibling.voucher_no = base.voucher_no
          AND sibling.direction = '借'
          AND %s
          AND %s
      )
    )
  )`, notBranchExpr, baseKeywords, liabilityExpr, siblingKeywords)
	}

	return fmt.Sprintf(`
SELECT
  COALESCE(SUM(CASE WHEN %s AND base.direction='贷' AND %s THEN COALESCE(base.credit_amount,0) ELSE 0 END),0) AS wage,
  COALESCE(SUM(CASE WHEN %s AND base.direction='贷' AND %s THEN COALESCE(base.credit_amount,0) ELSE 0 END),0) AS social,
  COALESCE(SUM(CASE WHEN %s AND base.direction='贷' AND %s THEN COALESCE(base.credit_amount,0) ELSE 0 END),0) AS housing
FROM journal base
WHERE (? LIKE '%%' || base.company || '%%' OR base.company LIKE '%%' || ? || '%%')
  AND base.voucher_date BETWEEN ? AND ?
`,
		bankExpr, categoryExpr("wage"),
		bankExpr, categoryExpr("social"),
		bankExpr, categoryExpr("housing"),
	)
}

func hrInExpr(column string, values []string) string {
	if len(values) == 0 {
		return "1=0"
	}
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		quoted = append(quoted, hrQuotedLiteral(value))
	}
	if len(quoted) == 0 {
		return "1=0"
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(quoted, ","))
}

func hrPrefixLikeExpr(column string, prefixes []string) string {
	if len(prefixes) == 0 {
		return "1=0"
	}
	parts := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s LIKE %s", column, hrQuotedLiteral(prefix+"%")))
	}
	return hrJoinOr(parts...)
}

func hrKeywordLikeExpr(columns, keywords []string) string {
	if len(columns) == 0 || len(keywords) == 0 {
		return "1=0"
	}
	parts := make([]string, 0, len(columns)*len(keywords))
	for _, column := range columns {
		column = strings.TrimSpace(column)
		if column == "" {
			continue
		}
		for _, keyword := range keywords {
			keyword = strings.TrimSpace(keyword)
			if keyword == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s LIKE %s", column, hrQuotedLiteral("%"+keyword+"%")))
		}
	}
	return hrJoinOr(parts...)
}

func hrJoinOr(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	if len(filtered) == 0 {
		return "1=0"
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return "(" + strings.Join(filtered, " OR ") + ")"
}

func hrQuotedLiteral(raw string) string {
	return "'" + strings.ReplaceAll(raw, "'", "''") + "'"
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
