package query

import (
	"fmt"
	"strings"

	"financeqa/internal/openitems"
)

func (e *Engine) queryAccountPayableReceivable(period, accountName, accountCodePrefix, typ, entity string) Result {
	if strings.TrimSpace(entity) != "" {
		if official := e.queryEntityARAPOfficialRollforward(period, accountName, accountCodePrefix, typ, entity); official.Success {
			return official
		}
	}

	if strings.TrimSpace(entity) == "" {
		official := e.queryAccountPayableReceivableFromBalanceSheet(period, accountName, accountCodePrefix, typ)
		if official.Success {
			if openSummary, openResult := e.queryAccountPayableReceivableOpenItems(period, accountName, accountCodePrefix, typ, entity); openSummary {
				if official.Data == nil {
					official.Data = map[string]any{}
				}
				official.Data["open_item_analysis"] = openResult.Data
				official.Data["open_item_match"] = approxEqual(anyToFloat64(official.Data["total"]), anyToFloat64(openResult.Data["total"]))
				official.CalculationLogs = append(official.CalculationLogs,
					fmt.Sprintf("[开放项补充] open_item_total=%.2f official_total=%.2f matched=%v",
						anyToFloat64(openResult.Data["total"]), anyToFloat64(official.Data["total"]), official.Data["open_item_match"]),
				)
			}
			return official
		}
	}

	if ok, openResult := e.queryAccountPayableReceivableOpenItems(period, accountName, accountCodePrefix, typ, entity); ok {
		return openResult
	}

	if strings.TrimSpace(entity) != "" {
		return Result{
			Success: false,
			Message: fmt.Sprintf("[%s] 未找到%s余额", entity, accountName),
			Data: map[string]any{
				"entity":  entity,
				"account": accountName,
				"period":  period,
			},
		}
	}

	return e.queryAccountPayableReceivableFromBalanceSheet(period, accountName, accountCodePrefix, typ)
}

func (e *Engine) queryEntityARAPOfficialRollforward(period, accountName, accountCodePrefix, typ, entity string) Result {
	kind := openitems.Receivable
	actionLabel := "回款/冲减"
	if typ == "payable" {
		kind = openitems.Payable
		actionLabel = "付款/冲减"
	}
	openSummary, err := e.getOpenItemSummaryCached(openitems.Options{
		Company:           e.Company,
		Period:            period,
		AccountCodePrefix: accountCodePrefix,
		Kind:              kind,
		Counterparty:      entity,
	})
	if err != nil || !openSummary.HasData || len(openSummary.CounterpartySummaries) == 0 {
		return Result{}
	}

	detail := openSummary.CounterpartySummaries[0]
	resultData := map[string]any{
		"type":                    typ,
		"period":                  period,
		"account":                 accountName,
		"entity":                  entity,
		"source":                  "journal_entity_rollforward",
		"total":                   round2(detail.OfficialClosingBalance),
		"opening_balance":         round2(detail.OfficialOpeningBalance),
		"current_increase":        round2(detail.OfficialCurrentIncrease),
		"current_decrease":        round2(detail.OfficialCurrentDecrease),
		"closing":                 round2(detail.OfficialClosingBalance),
		"open_item_closing_total": round2(detail.OpenItemClosingBalance),
		"details": []map[string]any{
			{
				"counterparty":              detail.Counterparty,
				"opening_balance":           round2(detail.OfficialOpeningBalance),
				"current_increase":          round2(detail.OfficialCurrentIncrease),
				"current_decrease":          round2(detail.OfficialCurrentDecrease),
				"closing_balance":           round2(detail.OfficialClosingBalance),
				"open_item_opening_balance": round2(detail.OpenItemOpeningBalance),
				"open_item_closing_balance": round2(detail.OpenItemClosingBalance),
				"historical_settlement":     round2(detail.HistoricalSettlement),
				"current_period_settlement": round2(detail.CurrentSettlement),
				"settlement_confidence":     settlementConfidenceMap(detail.SettlementConfidence),
				"open_items":                cloneOpenItemsForResult(detail.OpenItems),
			},
		},
		"open_item_analysis": map[string]any{
			"source":                    "journal_open_items",
			"total":                     round2(detail.OpenItemClosingBalance),
			"opening_balance":           round2(detail.OpenItemOpeningBalance),
			"historical_settlement":     round2(detail.HistoricalSettlement),
			"current_period_settlement": round2(detail.CurrentSettlement),
			"settlement_confidence":     settlementConfidenceMap(detail.SettlementConfidence),
			"open_items":                cloneOpenItemsForResult(detail.OpenItems),
		},
		"arithmetic_checks": map[string]any{
			"rollforward_check": CheckOpeningDeltaClosing(detail.OfficialOpeningBalance, detail.OfficialCurrentIncrease-detail.OfficialCurrentDecrease, detail.OfficialClosingBalance),
		},
	}

	return Result{
		Success: true,
		Message: fmt.Sprintf("%s %s %s期末余额 %.2f 元（期初 %.2f，本期新增 %.2f，本期%s %.2f，开放项残余 %.2f）。",
			period, entity, accountName, detail.OfficialClosingBalance, detail.OfficialOpeningBalance, detail.OfficialCurrentIncrease, actionLabel, detail.OfficialCurrentDecrease, detail.OpenItemClosingBalance),
		Data: resultData,
		ExecutedSQL: []string{
			"queryEntityARAPOfficialRollforward(journal): openitems.BuildSummary over journal rows with voucher sibling counterparty inference",
		},
		CalculationLogs: []string{
			fmt.Sprintf("[AR/AP实体滚动] entity=%s account=%s opening=%.2f increase=%.2f decrease=%.2f closing=%.2f open_item_residual=%.2f", entity, accountName, detail.OfficialOpeningBalance, detail.OfficialCurrentIncrease, detail.OfficialCurrentDecrease, detail.OfficialClosingBalance, detail.OpenItemClosingBalance),
		},
	}
}

func (e *Engine) queryAccountPayableReceivableFromBalanceSheet(period, accountName, accountCodePrefix, typ string) Result {
	rows, err := e.db.Query(`
SELECT account_name, opening_balance, closing_balance
FROM balance_sheet
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period = ?
  AND (account_name LIKE ? OR account_code LIKE ?)
`, e.Company, e.Company, period, accountName+"%", accountCodePrefix+"%")
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	defer rows.Close()

	details := make([]map[string]any, 0)
	openingTotal := 0.0
	total := 0.0
	for rows.Next() {
		var name string
		var opening float64
		var closing float64
		if err := rows.Scan(&name, &opening, &closing); err != nil {
			continue
		}
		openingTotal += opening
		total += closing
		details = append(details, map[string]any{"account": name, "opening_balance": opening, "closing_balance": closing})
	}
	if len(details) == 0 {
		return Result{Success: false, Message: "该期间未找到应收/应付余额"}
	}

	sumCheck := CheckSumEqualsTotal(extractClosingBalances(details), total)
	rollforward, hasRollforward := e.queryBalanceDetailRollforward(period, accountCodePrefix)
	rollforwardCheck := ArithmeticCheckResult{Passed: true, Diff: 0, Message: "balance_detail rollforward not available"}
	if hasRollforward {
		rollforwardCheck = CheckOpeningDeltaClosing(rollforward.OpeningNet, rollforward.DeltaNet, rollforward.ClosingNet)
	}

	msg := fmt.Sprintf("%s %s期末余额 %.2f 元", period, accountName, total)
	return Result{
		Success: true,
		Message: msg,
		Data: map[string]any{
			"type": typ, "period": period, "total": total, "details": details,
			"account": accountName, "source": "balance_sheet", "opening_balance": round2(openingTotal), "closing": total,
			"arithmetic_checks": map[string]any{
				"sum_equals_total":    sumCheck,
				"rollforward_check":   rollforwardCheck,
				"balance_rollforward": rollforward,
			},
		},
		ExecutedSQL: []string{
			"queryAccountPayableReceivable: SELECT account_name, opening_balance, closing_balance FROM balance_sheet WHERE ... AND (account_name LIKE ? OR account_code LIKE ?)",
			"queryAccountPayableReceivable(balance_detail): SELECT SUM(opening_debit), SUM(opening_credit), SUM(current_debit), SUM(current_credit), SUM(closing_debit), SUM(closing_credit) FROM balance_detail WHERE ... AND account_code LIKE ?",
		},
		CalculationLogs: []string{
			fmt.Sprintf("[AR/AP汇总] period=%s account=%s total=%.2f detail_count=%d", period, accountName, total, len(details)),
			fmt.Sprintf("[算术校验] sum_equals_total passed=%v diff=%.2f", sumCheck.Passed, sumCheck.Diff),
			fmt.Sprintf("[滚动校验] rollforward passed=%v diff=%.2f", rollforwardCheck.Passed, rollforwardCheck.Diff),
		},
	}
}

type balanceRollforward struct {
	OpeningNet float64 `json:"opening_net"`
	DeltaNet   float64 `json:"delta_net"`
	ClosingNet float64 `json:"closing_net"`
}

func (e *Engine) queryBalanceDetailRollforward(period, accountCodePrefix string) (balanceRollforward, bool) {
	var openingDebit, openingCredit, currentDebit, currentCredit, closingDebit, closingCredit float64
	err := e.db.QueryRow(`
SELECT
  COALESCE(SUM(opening_debit),0),
  COALESCE(SUM(opening_credit),0),
  COALESCE(SUM(current_debit),0),
  COALESCE(SUM(current_credit),0),
  COALESCE(SUM(closing_debit),0),
  COALESCE(SUM(closing_credit),0)
FROM balance_detail
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND period = ?
  AND account_code LIKE ?
`, e.Company, e.Company, period, accountCodePrefix+"%").Scan(&openingDebit, &openingCredit, &currentDebit, &currentCredit, &closingDebit, &closingCredit)
	if err != nil {
		return balanceRollforward{}, false
	}
	openingNet := openingDebit - openingCredit
	deltaNet := currentDebit - currentCredit
	closingNet := closingDebit - closingCredit
	if openingNet == 0 && deltaNet == 0 && closingNet == 0 {
		return balanceRollforward{}, false
	}
	return balanceRollforward{
		OpeningNet: round2(openingNet),
		DeltaNet:   round2(deltaNet),
		ClosingNet: round2(closingNet),
	}, true
}

func extractClosingBalances(details []map[string]any) []float64 {
	out := make([]float64, 0, len(details))
	for _, d := range details {
		if v, ok := d["closing_balance"].(float64); ok {
			out = append(out, v)
		}
	}
	return out
}
