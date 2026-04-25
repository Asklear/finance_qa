package query

import (
	"fmt"
	"strings"
)

type supplierPaymentSummary struct {
	Period      string
	Total       float64
	Suppliers   []map[string]any
	Excluded    []map[string]any
	ExecutedSQL []string
	Logs        []string
}

type supplierPaymentRow struct {
	Name     string
	OutAmt   float64
	InAmt    float64
	TxnCount int
}

func (e *Engine) collectSupplierPaymentSummary(from, to string) (supplierPaymentSummary, error) {
	startDate := from + "-01"
	endDate := monthEndDay(to)
	periodLabel := displayPeriod(from, to)
	sqlTxt := `
SELECT counterparty_name,
       ROUND(COALESCE(SUM(debit_amount),0),2) AS out_amt,
       ROUND(COALESCE(SUM(credit_amount),0),2) AS in_amt,
       COUNT(*) AS txn_count
FROM bank_statement
WHERE (? LIKE '%' || company || '%' OR company LIKE '%' || ? || '%')
  AND transaction_date BETWEEN ? AND ?
  AND COALESCE(TRIM(counterparty_name),'') <> ''
  AND COALESCE(debit_amount,0) > 0
GROUP BY counterparty_name
ORDER BY out_amt DESC, counterparty_name
`
	rows, err := e.db.Query(sqlTxt, e.Company, e.Company, startDate, endDate)
	if err != nil {
		return supplierPaymentSummary{}, err
	}
	defer rows.Close()

	summary := supplierPaymentSummary{
		Period:    periodLabel,
		Suppliers: make([]map[string]any, 0),
		Excluded:  make([]map[string]any, 0),
		Logs: []string{
			fmt.Sprintf("[供应商付款] period=%s start=%s end=%s", periodLabel, startDate, endDate),
		},
		ExecutedSQL: []string{
			fmt.Sprintf("querySupplierPayments(bank_statement): %s [args: %s, %s, %s]", sqlTxt, e.Company, startDate, endDate),
			"supplier_payment_classification: collectCounterpartyEvidence + ClassifyCounterparty + internal-party filter per counterparty",
		},
	}
	cfg := getRuleConfig()
	aggregates := make([]supplierPaymentRow, 0, 32)
	candidateNames := make([]string, 0, 32)
	prefiltered := make(map[string]string, 8)

	for rows.Next() {
		var name string
		var outAmt, inAmt float64
		var txnCount int
		if scanErr := rows.Scan(&name, &outAmt, &inAmt, &txnCount); scanErr != nil {
			continue
		}
		aggregates = append(aggregates, supplierPaymentRow{
			Name:     name,
			OutAmt:   outAmt,
			InAmt:    inAmt,
			TxnCount: txnCount,
		})

		switch {
		case looksLikeSupplierPaymentExcludedName(name, cfg):
			prefiltered[name] = "non_counterparty_flow"
		case internalPartyMatchesCompany(e.Company, name) || looksLikeInternalOrgUnit(name, cfg):
			prefiltered[name] = "internal_party"
		default:
			candidateNames = append(candidateNames, name)
		}
	}

	groupedEvidence := e.collectExactCounterpartyEvidenceMap(candidateNames, startDate, endDate)
	for _, aggregate := range aggregates {
		name := aggregate.Name

		var (
			evidence       []LedgerEvidence
			classification CounterpartyClassification
			include        bool
			reason         string
		)
		if preReason, ok := prefiltered[name]; ok {
			include, reason = false, preReason
			} else {
				evidence = groupedEvidence[name]
				classification = ClassifyCounterparty(name, evidence)
				include, reason = e.shouldIncludeSupplierPaymentCounterparty(name, classification)
				if !include && !hasCompleteStructuredCounterpartyEvidence(evidence) {
					evidence = e.collectCounterpartyEvidence(name, from, to)
					classification = ClassifyCounterparty(name, evidence)
					include, reason = e.shouldIncludeSupplierPaymentCounterparty(name, classification)
				}
			}
		row := map[string]any{
			"name":       name,
			"out_amount": round2(aggregate.OutAmt),
			"in_amount":  round2(aggregate.InAmt),
			"txn_count":  aggregate.TxnCount,
			"role":       string(classification.Role),
			"confidence": classification.Confidence,
			"signals":    classification.Signals,
		}
		if include {
			summary.Suppliers = append(summary.Suppliers, row)
			summary.Total += aggregate.OutAmt
			summary.Logs = append(summary.Logs, fmt.Sprintf("[供应商付款-纳入] %s out=%.2f role=%s reason=%s", name, round2(aggregate.OutAmt), classification.Role, reason))
			continue
		}
		row["exclude_reason"] = reason
		summary.Excluded = append(summary.Excluded, row)
		summary.Logs = append(summary.Logs, fmt.Sprintf("[供应商付款-剔除] %s out=%.2f role=%s reason=%s", name, round2(aggregate.OutAmt), classification.Role, reason))
	}
	summary.Total = round2(summary.Total)

	return summary, nil
}

func (e *Engine) collectExactCounterpartyEvidenceMap(names []string, startDate, endDate string) map[string][]LedgerEvidence {
	grouped := make(map[string][]LedgerEvidence, len(names))
	if len(names) == 0 {
		return grouped
	}
	for _, ev := range e.collectDirectCounterpartyEvidence(names, startDate, endDate) {
		key := strings.TrimSpace(ev.Counterparty)
		if key == "" {
			continue
		}
		grouped[key] = append(grouped[key], ev)
	}
	return grouped
}
