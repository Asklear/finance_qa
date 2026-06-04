package query

import (
	"math"
	"sort"
	"strings"
)

func rollupContractAggregateItemsByName(items []contractAggregateOpenItem, total float64) []contractAggregateDimensionRow {
	byName := map[string]*contractAggregateDimensionRow{}
	for _, item := range items {
		name := strings.TrimSpace(item.CustomerName)
		if name == "" {
			name = "未命名对象"
		}
		row := byName[name]
		if row == nil {
			row = &contractAggregateDimensionRow{Name: name}
			byName[name] = row
		}
		row.SettlementAmount += item.SettlementAmount
		row.InvoiceAmount += item.InvoiceAmount
		row.MovementAmount += item.ReceivedAmount
		row.OpenAmount += item.OpenAmount
	}

	rows := make([]contractAggregateDimensionRow, 0, len(byName))
	for _, row := range byName {
		row.SettlementAmount = round2(row.SettlementAmount)
		row.InvoiceAmount = round2(row.InvoiceAmount)
		row.MovementAmount = round2(row.MovementAmount)
		row.OpenAmount = round2(row.OpenAmount)
		if total > 0 {
			row.Share = roundRatio(row.SettlementAmount / total)
		}
		rows = append(rows, *row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SettlementAmount == rows[j].SettlementAmount {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].SettlementAmount > rows[j].SettlementAmount
	})
	return rows
}

func topNContractAggregateShare(rows []contractAggregateDimensionRow, n int) float64 {
	total := 0.0
	for _, row := range rows {
		total += row.SettlementAmount
	}
	top := topNContractAggregateSettlement(rows, n)
	if total <= 0 {
		return 0
	}
	return roundRatio(top / total)
}

func topNContractAggregateSettlement(rows []contractAggregateDimensionRow, n int) float64 {
	if n <= 0 || len(rows) == 0 {
		return 0
	}
	if n > len(rows) {
		n = len(rows)
	}
	top := 0.0
	for i := 0; i < n; i++ {
		top += rows[i].SettlementAmount
	}
	return round2(top)
}

func rollupContractAggregateOpenItemsByName(items []contractAggregateOpenItem, totalOpen float64) []contractAggregateDimensionRow {
	rows := rollupContractAggregateItemsByName(items, 0)
	for i := range rows {
		if totalOpen > 0 {
			rows[i].Share = roundRatio(rows[i].OpenAmount / totalOpen)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].OpenAmount == rows[j].OpenAmount {
			if rows[i].SettlementAmount == rows[j].SettlementAmount {
				return rows[i].Name < rows[j].Name
			}
			return rows[i].SettlementAmount > rows[j].SettlementAmount
		}
		return rows[i].OpenAmount > rows[j].OpenAmount
	})
	return rows
}

func roundRatio(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func (e *Engine) collectRevenueOpenBuckets(periodFrom, periodTo string, ranking []contractAggregateDimensionRow) []contractAggregateOpenBucket {
	if len(ranking) == 0 {
		return nil
	}
	priorFrom, priorTo, currentFrom, currentTo, ok := splitContractAggregateOpenBucketPeriods(periodFrom, periodTo)
	if !ok {
		return nil
	}
	priorRows := e.revenueOpenByCustomer(priorFrom, priorTo)
	currentRows := e.revenueOpenByCustomer(currentFrom, currentTo)
	out := make([]contractAggregateOpenBucket, 0, len(ranking))
	for _, row := range ranking {
		prior := priorRows[row.Name]
		current := currentRows[row.Name]
		if prior == 0 && current == 0 {
			continue
		}
		out = append(out, contractAggregateOpenBucket{
			Name:         row.Name,
			PriorLabel:   displayPeriod(priorFrom, priorTo),
			PriorFrom:    priorFrom,
			PriorTo:      priorTo,
			PriorOpen:    round2(prior),
			CurrentLabel: displayPeriod(currentFrom, currentTo),
			CurrentFrom:  currentFrom,
			CurrentTo:    currentTo,
			CurrentOpen:  round2(current),
			TotalOpen:    round2(prior + current),
		})
	}
	return out
}

func (e *Engine) revenueOpenByCustomer(from, to string) map[string]float64 {
	out := map[string]float64{}
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" || from > to {
		return out
	}
	items, err := e.collectRevenueItems(from, to, "")
	if err != nil {
		return out
	}
	rows := rollupContractAggregateOpenItemsByName(filterOpenContractAggregateItems(items), 0)
	for _, row := range rows {
		out[row.Name] = row.OpenAmount
	}
	return out
}

func splitContractAggregateOpenBucketPeriods(periodFrom, periodTo string) (string, string, string, string, bool) {
	fromYear, fromMonth := parsePeriod(periodFrom)
	toYear, toMonth := parsePeriod(periodTo)
	if fromYear == 0 || fromMonth == 0 || toYear == 0 || toMonth == 0 {
		return "", "", "", "", false
	}
	currentStartMonth := ((toMonth-1)/3)*3 + 1
	currentFrom := formatYearMonth(toYear, currentStartMonth)
	currentTo := periodTo
	if periodFrom >= currentFrom {
		return "", "", "", "", false
	}
	priorTo := previousPeriod(currentFrom)
	if priorTo == "" || periodFrom > priorTo {
		return "", "", "", "", false
	}
	return periodFrom, priorTo, currentFrom, currentTo, true
}

func previousPeriod(period string) string {
	year, month := parsePeriod(period)
	if year == 0 || month == 0 {
		return ""
	}
	month--
	if month == 0 {
		year--
		month = 12
	}
	return formatYearMonth(year, month)
}
