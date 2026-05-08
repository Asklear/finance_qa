package query

func buildContractDimensionFactSet(spec QuerySpec, summary contractDimensionSummary) FactSet {
	tracePayload := map[string]any{
		"entity":       summary.Entity,
		"role":         summary.Role,
		"period":       summary.Period,
		"contracts":    summary.Contracts,
		"data":         summary.Data,
		"executed_sql": append([]string{}, summary.ExecutedSQL...),
		"logs":         append([]string{}, summary.CalculationLog...),
	}
	facts := []Fact{
		{
			Source:         "contracts",
			MetricKey:      "contract_match_count",
			Entity:         summary.Entity,
			PeriodFrom:     spec.PeriodFrom,
			PeriodTo:       spec.PeriodTo,
			Value:          float64(len(summary.Contracts)),
			AuthorityLevel: AuthoritySupporting,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		},
	}

	bookView, _ := summary.Data["book_view"].(map[string]any)
	cashView, _ := summary.Data["cash_view"].(map[string]any)
	appendFact := func(metricKey string, value float64) {
		facts = append(facts, Fact{
			Source:         "contracts",
			MetricKey:      metricKey,
			Entity:         summary.Entity,
			PeriodFrom:     spec.PeriodFrom,
			PeriodTo:       spec.PeriodTo,
			Value:          round2(value),
			AuthorityLevel: AuthoritySupporting,
			CoverageStatus: CoverageFull,
			Confidence:     1,
			TracePayload:   tracePayload,
		})
	}

	switch summary.Role {
	case "customer_contract":
		appendFact("contract_book_settlement", anyToFloat64(bookView["settlement_amount"]))
		appendFact("contract_book_invoice", anyToFloat64(bookView["invoice_amount"]))
		appendFact("contract_cash_received", anyToFloat64(cashView["received_amount"]))
		if summary.SubPeriod != "" {
			appendFact("contract_cash_received_subperiod", anyToFloat64(summary.Data["sub_period_receipts"]))
		}
	case "supplier_contract":
		appendFact("contract_book_cost", anyToFloat64(bookView["contract_cost"]))
		appendFact("contract_cash_paid", anyToFloat64(cashView["cash_paid_amount"]))
	case "mixed_contract":
		appendFact("contract_book_revenue_settlement", anyToFloat64(bookView["revenue_settlement"]))
		appendFact("contract_book_cost", anyToFloat64(bookView["cost_settlement"]))
		appendFact("contract_cash_received", anyToFloat64(cashView["received_amount"]))
		appendFact("contract_cash_paid", anyToFloat64(cashView["cash_paid_amount"]))
	}

	return FactSet{Source: "contracts", Facts: facts}
}

func applyContractPerspectiveAliases(data map[string]any) {
	cashView, hasCash := data["cash_view"]
	bookView, hasBook := data["book_view"]
	if hasCash {
		data["money_view"] = cashView
	}
	if hasBook {
		data["account_view"] = bookView
	}
	if hasCash || hasBook {
		data["dual_perspective"] = map[string]any{
			"cash_label":    "现金口径",
			"cash_view":     cashView,
			"account_label": "财务口径",
			"account_view":  bookView,
		}
	}
}
