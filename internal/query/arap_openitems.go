package query

import (
	"context"
	"fmt"
	"strings"

	"financeqa/internal/openitems"
)

func (e *Engine) queryAccountPayableReceivableOpenItems(period, accountName, accountCodePrefix, typ, entity string) (bool, Result) {
	kind := openitems.Receivable
	historyLabel := "历史应收"
	currentLabel := "当月新增应收"
	if typ == "payable" {
		kind = openitems.Payable
		historyLabel = "历史应付"
		currentLabel = "当月新增应付"
	}
	openSummary, err := e.getOpenItemSummaryCached(openitems.Options{
		Company:           e.Company,
		Period:            period,
		AccountCodePrefix: accountCodePrefix,
		Kind:              kind,
		Counterparty:      entity,
	})
	if err == nil && openSummary.HasData {
		details := make([]map[string]any, 0, len(openSummary.CounterpartySummaries))
		for _, item := range openSummary.CounterpartySummaries {
			detailOpenItems := make([]map[string]any, 0, len(item.OpenItems))
			for _, openItem := range item.OpenItems {
				detailOpenItems = append(detailOpenItems, map[string]any{
					"counterparty": openItem.Counterparty,
					"source_date":  openItem.SourceDate,
					"voucher_no":   openItem.VoucherNo,
					"amount":       openItem.Amount,
					"age_days":     openItem.AgeDays,
				})
			}
			details = append(details, map[string]any{
				"counterparty":              item.Counterparty,
				"opening_balance":           item.OpeningBalance,
				"current_increase":          item.CurrentIncrease,
				"current_decrease":          item.CurrentDecrease,
				"official_opening_balance":  item.OfficialOpeningBalance,
				"official_current_increase": item.OfficialCurrentIncrease,
				"official_current_decrease": item.OfficialCurrentDecrease,
				"official_closing_balance":  item.OfficialClosingBalance,
				"open_item_opening_balance": item.OpenItemOpeningBalance,
				"open_item_closing_balance": item.OpenItemClosingBalance,
				"historical_settlement":     item.HistoricalSettlement,
				"current_period_settlement": item.CurrentSettlement,
				"settlement_confidence":     settlementConfidenceMap(item.SettlementConfidence),
				"closing_balance":           item.ClosingBalance,
				"open_items":                detailOpenItems,
			})
		}

		openItemMaps := make([]map[string]any, 0, len(openSummary.OpenItems))
		for _, item := range openSummary.OpenItems {
			openItemMaps = append(openItemMaps, map[string]any{
				"counterparty": item.Counterparty,
				"source_date":  item.SourceDate,
				"voucher_no":   item.VoucherNo,
				"amount":       item.Amount,
				"age_days":     item.AgeDays,
			})
		}

		sumCheck := CheckSumEqualsTotal(extractClosingBalances(details), openSummary.ClosingBalance)
		rollforwardCheck := CheckOpeningDeltaClosing(openSummary.OpeningBalance, openSummary.CurrentIncrease-openSummary.CurrentDecrease, openSummary.ClosingBalance)
		msg := formatARAPOpenItemSummaryMessage(period, accountName, entity, openSummary, historyLabel, currentLabel)
		return true, Result{
			Success: true,
			Message: msg,
			Data: map[string]any{
				"type":                      typ,
				"period":                    period,
				"total":                     openSummary.ClosingBalance,
				"details":                   details,
				"account":                   accountName,
				"entity":                    entity,
				"closing":                   openSummary.ClosingBalance,
				"opening_balance":           openSummary.OpeningBalance,
				"current_increase":          openSummary.CurrentIncrease,
				"current_decrease":          openSummary.CurrentDecrease,
				"official_opening_balance":  openSummary.OfficialOpeningBalance,
				"official_current_increase": openSummary.OfficialCurrentIncrease,
				"official_current_decrease": openSummary.OfficialCurrentDecrease,
				"official_closing_balance":  openSummary.OfficialClosingBalance,
				"open_item_opening_balance": openSummary.OpenItemOpeningBalance,
				"open_item_closing_balance": openSummary.OpenItemClosingBalance,
				"historical_settlement":     openSummary.HistoricalSettlement,
				"current_period_settlement": openSummary.CurrentSettlement,
				"settlement_confidence":     settlementConfidenceMap(openSummary.SettlementConfidence),
				"open_items":                openItemMaps,
				"source":                    "journal_open_items",
				"arithmetic_checks": map[string]any{
					"sum_equals_total":  sumCheck,
					"rollforward_check": rollforwardCheck,
				},
			},
			ExecutedSQL: []string{
				"queryAccountPayableReceivable(open_items): SELECT voucher_date, account_code, voucher_no, account_name, summary, counterparty, debit_amount, credit_amount FROM journal WHERE ... AND account_code LIKE ? AND voucher_date <= ? ORDER BY DATE(voucher_date), voucher_no, account_code, account_name, summary, counterparty, debit_amount, credit_amount",
			},
			CalculationLogs: []string{
				fmt.Sprintf("[AR/AP开放项] period=%s account=%s opening=%.2f increase=%.2f decrease=%.2f historical_settlement=%.2f current_settlement=%.2f probable_historical=%.2f probable_current=%.2f unmatched=%.2f closing=%.2f counterparty_count=%d", period, accountName, openSummary.OpeningBalance, openSummary.CurrentIncrease, openSummary.CurrentDecrease, openSummary.HistoricalSettlement, openSummary.CurrentSettlement, openSummary.SettlementConfidence.ProbableHistoricalSettlement, openSummary.SettlementConfidence.ProbableCurrentSettlement, openSummary.SettlementConfidence.UnmatchedDecrease, openSummary.ClosingBalance, len(details)),
				fmt.Sprintf("[算术校验] sum_equals_total passed=%v diff=%.2f", sumCheck.Passed, sumCheck.Diff),
				fmt.Sprintf("[滚动校验] rollforward passed=%v diff=%.2f", rollforwardCheck.Passed, rollforwardCheck.Diff),
			},
		}
	}
	return false, Result{}
}

func (e *Engine) getOpenItemSummaryCached(opts openitems.Options) (openitems.Summary, error) {
	cacheKey := strings.Join([]string{
		strings.TrimSpace(opts.Company),
		strings.TrimSpace(opts.Period),
		strings.TrimSpace(opts.AccountCodePrefix),
		string(opts.Kind),
		strings.TrimSpace(opts.Counterparty),
	}, "|")
	e.cacheMu.RLock()
	if cached, ok := e.openItemSummary[cacheKey]; ok {
		e.cacheMu.RUnlock()
		return cloneOpenItemSummary(cached), nil
	}
	e.cacheMu.RUnlock()

	summary, err := openitems.BuildSummary(context.Background(), e.db, opts)
	if err != nil {
		return openitems.Summary{}, err
	}
	e.cacheMu.Lock()
	e.openItemSummary[cacheKey] = cloneOpenItemSummary(summary)
	e.cacheMu.Unlock()
	return summary, nil
}

func cloneOpenItemSummary(in openitems.Summary) openitems.Summary {
	out := in
	out.CounterpartySummaries = make([]openitems.CounterpartySummary, 0, len(in.CounterpartySummaries))
	for _, item := range in.CounterpartySummaries {
		itemCopy := item
		itemCopy.OpenItems = append([]openitems.OpenItem{}, item.OpenItems...)
		out.CounterpartySummaries = append(out.CounterpartySummaries, itemCopy)
	}
	out.OpenItems = append([]openitems.OpenItem{}, in.OpenItems...)
	return out
}

func mapsFromAnySlice(v any) []map[string]any {
	raw, ok := v.([]map[string]any)
	if ok {
		return raw
	}
	return []map[string]any{}
}

func settlementConfidenceMap(v openitems.SettlementConfidence) map[string]any {
	return map[string]any{
		"confirmed_historical_settlement": v.ConfirmedHistoricalSettlement,
		"probable_historical_settlement":  v.ProbableHistoricalSettlement,
		"confirmed_current_settlement":    v.ConfirmedCurrentSettlement,
		"probable_current_settlement":     v.ProbableCurrentSettlement,
		"unmatched_decrease":              v.UnmatchedDecrease,
	}
}

func cloneOpenItemsForResult(items []openitems.OpenItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"counterparty": item.Counterparty,
			"source_date":  item.SourceDate,
			"voucher_no":   item.VoucherNo,
			"amount":       item.Amount,
			"age_days":     item.AgeDays,
		})
	}
	return out
}
