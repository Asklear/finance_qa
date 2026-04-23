package query

import (
	"fmt"

	"financeqa/internal/analysis"
)

func (e *Engine) queryAnalysis(period string) Result {
	aging := analysis.NewAgingEngine(e.dbPath)
	defer aging.Close()
	summary, err := aging.AnalyzeSummary(e.Company, period)
	if err != nil {
		return Result{Success: false, Message: "analysis failed"}
	}
	return Result{
		Success: true,
		Message: "账龄分析成功",
		Data: map[string]any{
			"health":             summary.HealthScore,
			"receivable_total":   summary.ReceivableTotal,
			"payable_total":      summary.PayableTotal,
			"receivable_buckets": summary.ReceivableBuckets,
			"payable_buckets":    summary.PayableBuckets,
		},
		ExecutedSQL: []string{
			"queryAnalysis: internal aging engine SQL over journal with account_code LIKE '1122%'/'2202%'",
		},
		CalculationLogs: []string{
			fmt.Sprintf("[账龄分析] period=%s health=%d AR=%.2f AP=%.2f", period, summary.HealthScore, summary.ReceivableTotal, summary.PayableTotal),
		},
	}
}

func (e *Engine) counterpartyBankReceipts(entity, from, to string) float64 {
	amount, _ := summarizeCounterpartyCashEvidence(e.collectCounterpartyEvidence(entity, from, to))
	return round2(amount)
}
