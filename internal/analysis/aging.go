package analysis

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type AgingBucket struct {
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
}

type AgingSummary struct {
	Company           string        `json:"company"`
	Period            string        `json:"period"`
	ReceivableTotal   float64       `json:"receivable_total"`
	PayableTotal      float64       `json:"payable_total"`
	ReceivableBuckets []AgingBucket `json:"receivable_buckets"`
	PayableBuckets    []AgingBucket `json:"payable_buckets"`
	HealthScore       int           `json:"health_score"`
}

type AgingEngine struct {
	db *sql.DB
}

func NewAgingEngine(dbPath string) *AgingEngine {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return &AgingEngine{}
	}
	return &AgingEngine{db: db}
}

func (e *AgingEngine) Close() error {
	if e == nil || e.db == nil {
		return nil
	}
	return e.db.Close()
}

func (e *AgingEngine) AnalyzeSummary(company, period string) (AgingSummary, error) {
	if e.db == nil {
		return AgingSummary{}, fmt.Errorf("sqlite db not available")
	}

	endDate, err := periodEndDate(period)
	if err != nil {
		return AgingSummary{}, err
	}

	receivableBuckets, receivableTotal, err := e.loadBuckets(
		company,
		endDate,
		"1122%",
		`CASE WHEN COALESCE(debit_amount, 0) - COALESCE(credit_amount, 0) > 0 THEN COALESCE(debit_amount, 0) - COALESCE(credit_amount, 0) ELSE 0 END`,
	)
	if err != nil {
		return AgingSummary{}, err
	}

	payableBuckets, payableTotal, err := e.loadBuckets(
		company,
		endDate,
		"2202%",
		`CASE WHEN COALESCE(credit_amount, 0) - COALESCE(debit_amount, 0) > 0 THEN COALESCE(credit_amount, 0) - COALESCE(debit_amount, 0) ELSE 0 END`,
	)
	if err != nil {
		return AgingSummary{}, err
	}

	overdue := 0.0
	for _, bucket := range receivableBuckets {
		if bucket.Label != "0-30天" {
			overdue += bucket.Amount
		}
	}

	score := 100
	if receivableTotal > 0 {
		score -= int((overdue / receivableTotal) * 60)
	}
	if payableTotal > receivableTotal {
		score -= 10
	}
	if score < 1 {
		score = 1
	}
	if score > 100 {
		score = 100
	}

	return AgingSummary{
		Company:           company,
		Period:            period,
		ReceivableTotal:   receivableTotal,
		PayableTotal:      payableTotal,
		ReceivableBuckets: receivableBuckets,
		PayableBuckets:    payableBuckets,
		HealthScore:       score,
	}, nil
}

func (e *AgingEngine) loadBuckets(company string, endDate time.Time, accountLike string, amountExpr string) ([]AgingBucket, float64, error) {
	query := fmt.Sprintf(`
SELECT
  voucher_date,
  %s AS amount
FROM journal
WHERE company = ?
  AND account_code LIKE ?
  AND DATE(voucher_date) <= DATE(?)
`, amountExpr)

	rows, err := e.db.Query(query, company, accountLike, endDate.Format("2006-01-02"))
	if err != nil {
		return nil, 0, fmt.Errorf("query aging buckets: %w", err)
	}
	defer rows.Close()

	totals := map[string]float64{
		"0-30天":  0,
		"31-60天": 0,
		"61天以上": 0,
	}
	total := 0.0

	for rows.Next() {
		var voucherDate string
		var amount float64
		if err := rows.Scan(&voucherDate, &amount); err != nil {
			return nil, 0, fmt.Errorf("scan aging row: %w", err)
		}
		if amount <= 0 {
			continue
		}

		parsedDate, err := time.Parse("2006-01-02", voucherDate)
		if err != nil {
			continue
		}
		ageDays := int(endDate.Sub(parsedDate).Hours() / 24)
		switch {
		case ageDays <= 30:
			totals["0-30天"] += amount
		case ageDays <= 60:
			totals["31-60天"] += amount
		default:
			totals["61天以上"] += amount
		}
		total += amount
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate aging rows: %w", err)
	}

	buckets := []AgingBucket{
		{Label: "0-30天", Amount: totals["0-30天"]},
		{Label: "31-60天", Amount: totals["31-60天"]},
		{Label: "61天以上", Amount: totals["61天以上"]},
	}
	return buckets, total, nil
}

func periodEndDate(period string) (time.Time, error) {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid period %q: %w", period, err)
	}
	return t.AddDate(0, 1, -1), nil
}
