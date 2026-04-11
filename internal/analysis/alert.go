package analysis

import "fmt"

type Alert struct {
	Level   string         `json:"level"`
	Type    string         `json:"type"`
	Title   string         `json:"title"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type AlertEngine struct {
	aging *AgingEngine
}

func NewAlertEngine(dbPath string) *AlertEngine {
	return &AlertEngine{aging: NewAgingEngine(dbPath)}
}

func (e *AlertEngine) Close() error {
	if e == nil || e.aging == nil {
		return nil
	}
	return e.aging.Close()
}

func (e *AlertEngine) Generate(company, period string) ([]Alert, error) {
	if e.aging == nil {
		return nil, fmt.Errorf("aging engine not available")
	}
	summary, err := e.aging.AnalyzeSummary(company, period)
	if err != nil {
		return nil, err
	}

	alerts := make([]Alert, 0, 2)
	overdue := 0.0
	for _, bucket := range summary.ReceivableBuckets {
		if bucket.Label != "0-30天" {
			overdue += bucket.Amount
		}
	}
	if overdue > 0 {
		level := "info"
		if overdue > summary.ReceivableTotal*0.3 {
			level = "warning"
		}
		if overdue > summary.ReceivableTotal*0.5 {
			level = "critical"
		}
		alerts = append(alerts, Alert{
			Level:   level,
			Type:    "overdue_receivable",
			Title:   "应收账款逾期预警",
			Message: fmt.Sprintf("逾期应收 %.2f 元", overdue),
			Details: map[string]any{
				"company":         company,
				"period":          period,
				"receivableTotal": summary.ReceivableTotal,
				"overdueAmount":   overdue,
			},
		})
	}
	if summary.HealthScore < 60 {
		alerts = append(alerts, Alert{
			Level:   "warning",
			Type:    "liquidity_risk",
			Title:   "资金健康度偏低",
			Message: fmt.Sprintf("健康评分 %d", summary.HealthScore),
			Details: map[string]any{
				"company":     company,
				"period":      period,
				"healthScore": summary.HealthScore,
			},
		})
	}
	return alerts, nil
}
