package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	dbpkg "financeqa/internal/db"
	"financeqa/internal/openitems"
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
	db, err := dbpkg.Open(context.Background(), dbPath)
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
		return AgingSummary{}, fmt.Errorf("db not available")
	}

	endDate, err := periodEndDate(period)
	if err != nil {
		return AgingSummary{}, err
	}

	receivableBuckets, receivableTotal, err := e.loadBuckets(
		company,
		period,
		endDate,
		"1122",
		openitems.Receivable,
	)
	if err != nil {
		return AgingSummary{}, err
	}

	payableBuckets, payableTotal, err := e.loadBuckets(
		company,
		period,
		endDate,
		"2202",
		openitems.Payable,
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

func (e *AgingEngine) loadBuckets(company, period string, endDate time.Time, accountPrefix string, kind openitems.AccountKind) ([]AgingBucket, float64, error) {
	summary, err := openitems.BuildSummary(context.Background(), e.db, openitems.Options{
		Company:           company,
		Period:            period,
		AccountCodePrefix: accountPrefix,
		Kind:              kind,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("build aging open items: %w", err)
	}
	if summary.HasData {
		return bucketsFromOpenItems(summary.OpenItems), summary.ClosingBalance, nil
	}

	amountExpr := `CASE WHEN COALESCE(debit_amount, 0) - COALESCE(credit_amount, 0) > 0 THEN COALESCE(debit_amount, 0) - COALESCE(credit_amount, 0) ELSE 0 END`
	accountLike := accountPrefix + "%"
	if kind == openitems.Payable {
		amountExpr = `CASE WHEN COALESCE(credit_amount, 0) - COALESCE(debit_amount, 0) > 0 THEN COALESCE(credit_amount, 0) - COALESCE(debit_amount, 0) ELSE 0 END`
	}
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
		"61天以上":  0,
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

func bucketsFromOpenItems(items []openitems.OpenItem) []AgingBucket {
	totals := map[string]float64{
		"0-30天":  0,
		"31-60天": 0,
		"61天以上":  0,
	}
	for _, item := range items {
		switch {
		case item.AgeDays <= 30:
			totals["0-30天"] += item.Amount
		case item.AgeDays <= 60:
			totals["31-60天"] += item.Amount
		default:
			totals["61天以上"] += item.Amount
		}
	}
	return []AgingBucket{
		{Label: "0-30天", Amount: round2(totals["0-30天"])},
		{Label: "31-60天", Amount: round2(totals["31-60天"])},
		{Label: "61天以上", Amount: round2(totals["61天以上"])},
	}
}

func periodEndDate(period string) (time.Time, error) {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid period %q: %w", period, err)
	}
	return t.AddDate(0, 1, -1), nil
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
