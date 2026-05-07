package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	dbpkg "financeqa/internal/db"
	"financeqa/internal/storage"
	"financeqa/internal/support"
)

type auditMapping struct {
	TableType  string `json:"table_type"`
	Period     string `json:"period"`
	FileName   string `json:"file_name"`
	StorageKey string `json:"storage_key"`
	UpdatedAt  string `json:"updated_at"`
}

type auditAmount struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type auditSection struct {
	Name    string        `json:"name"`
	Amounts []auditAmount `json:"amounts"`
	Rows    int           `json:"rows,omitempty"`
}

type auditReport struct {
	WorkbookPath string         `json:"workbook_path"`
	Mappings     []auditMapping `json:"mappings"`
	SQL          []auditSection `json:"sql"`
	Excel        []auditSection `json:"excel"`
	Warnings     []string       `json:"warnings,omitempty"`
}

func runAuditAccuracy(args []string, stdout, stderr interface {
	Write([]byte) (int, error)
}) int {
	fs := flag.NewFlagSet("audit-accuracy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", "", "postgres dsn")
	workbookPath := fs.String("workbook", "", "local source workbook path")
	outPath := fs.String("out", "", "output json path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx := context.Background()
	dbTarget := resolveDBPath(*dbPath)
	db, err := dbpkg.Open(ctx, dbTarget)
	if err != nil {
		fmt.Fprintf(stderr, "open db failed: %s\n", support.SanitizeError(err))
		return 1
	}
	defer func() { _ = db.Close() }()

	report := auditReport{}
	mappings, err := loadAuditMappings(ctx, db)
	if err != nil {
		fmt.Fprintf(stderr, "load mappings failed: %v\n", err)
		return 1
	}
	report.Mappings = mappings
	report.SQL, err = buildSQLAuditSections(ctx, db)
	if err != nil {
		fmt.Fprintf(stderr, "build sql audit failed: %v\n", err)
		return 1
	}
	wbPath, warning := resolveAuditWorkbookPath(ctx, *workbookPath, mappings)
	if warning != "" {
		report.Warnings = append(report.Warnings, warning)
	}
	if wbPath != "" {
		report.WorkbookPath = wbPath
		excelSections, err := buildContractWorkbookAudit(wbPath)
		if err != nil {
			report.Warnings = append(report.Warnings, "parse workbook failed: "+err.Error())
		} else {
			report.Excel = excelSections
		}
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "marshal audit failed: %v\n", err)
		return 1
	}
	if strings.TrimSpace(*outPath) != "" {
		if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
			fmt.Fprintf(stderr, "mkdir output failed: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*outPath, raw, 0o644); err != nil {
			fmt.Fprintf(stderr, "write output failed: %v\n", err)
			return 1
		}
	}
	fmt.Fprintln(stdout, string(raw))
	return 0
}

func resolveAuditWorkbookPath(ctx context.Context, workbookPath string, mappings []auditMapping) (string, string) {
	wbPath := strings.TrimSpace(workbookPath)
	if wbPath != "" {
		return wbPath, ""
	}
	wbPath, err := downloadFinanceWorkbook(ctx, mappings)
	if err != nil {
		return "", "source workbook unavailable: " + err.Error()
	}
	return wbPath, ""
}

func loadAuditMappings(ctx context.Context, db *sql.DB) ([]auditMapping, error) {
	rows, err := db.QueryContext(ctx, `
SELECT COALESCE(CAST(table_type AS TEXT), ''),
       COALESCE(CAST(period AS TEXT), ''),
       COALESCE(CAST(file_name AS TEXT), ''),
       COALESCE(CAST(storage_key AS TEXT), ''),
       COALESCE(CAST(updated_at AS TEXT), '')
FROM fin_file_mappings
ORDER BY table_type, period, file_name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []auditMapping
	for rows.Next() {
		var row auditMapping
		if err := rows.Scan(&row.TableType, &row.Period, &row.FileName, &row.StorageKey, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func buildSQLAuditSections(ctx context.Context, db *sql.DB) ([]auditSection, error) {
	sections := []auditSection{}
	queries := []struct {
		name string
		sql  string
	}{
		{"contract_2026_03", contractAmountAuditSQL("fin_fund_income", "fin_fund_income_groups", "received_amount", "year_month='2026-03'")},
		{"cost_2026_03", contractAmountAuditSQL("fin_cost_settlements", "fin_cost_settlement_groups", "paid_amount", "year_month='2026-03'")},
		{"contract_2026_q1", contractAmountAuditSQL("fin_fund_income", "fin_fund_income_groups", "received_amount", "year_month BETWEEN '2026-01' AND '2026-03'")},
		{"cost_2026_q1", contractAmountAuditSQL("fin_cost_settlements", "fin_cost_settlement_groups", "paid_amount", "year_month BETWEEN '2026-01' AND '2026-03'")},
		{"contract_2026_03_direct", `SELECT COUNT(*), COALESCE(SUM(settlement_amount),0), COALESCE(SUM(received_amount),0), COALESCE(SUM(invoice_amount),0) FROM fin_fund_income WHERE year_month='2026-03'`},
		{"cost_2026_03_direct", `SELECT COUNT(*), COALESCE(SUM(settlement_amount),0), COALESCE(SUM(paid_amount),0), COALESCE(SUM(invoice_amount),0) FROM fin_cost_settlements WHERE year_month='2026-03'`},
		{"contract_2026_q1_direct", `SELECT COUNT(*), COALESCE(SUM(settlement_amount),0), COALESCE(SUM(received_amount),0), COALESCE(SUM(invoice_amount),0) FROM fin_fund_income WHERE year_month BETWEEN '2026-01' AND '2026-03'`},
		{"cost_2026_q1_direct", `SELECT COUNT(*), COALESCE(SUM(settlement_amount),0), COALESCE(SUM(paid_amount),0), COALESCE(SUM(invoice_amount),0) FROM fin_cost_settlements WHERE year_month BETWEEN '2026-01' AND '2026-03'`},
	}
	for _, q := range queries {
		var rows int
		var a, b, c float64
		if err := db.QueryRowContext(ctx, q.sql).Scan(&rows, &a, &b, &c); err != nil {
			return nil, err
		}
		names := []string{"settlement_amount", "cash_amount", "invoice_amount"}
		sections = append(sections, auditSection{Name: q.name, Rows: rows, Amounts: []auditAmount{{Name: names[0], Value: roundAuditAmount(a)}, {Name: names[1], Value: roundAuditAmount(b)}, {Name: names[2], Value: roundAuditAmount(c)}}})
	}
	periodQueries := []struct {
		name string
		sql  string
	}{
		{"bank_2026_03", `SELECT COUNT(*), COALESCE(SUM(credit_amount),0), COALESCE(SUM(debit_amount),0), COALESCE(SUM(credit_amount),0)-COALESCE(SUM(debit_amount),0) FROM fin_bank_statement WHERE transaction_date BETWEEN '2026-03-01' AND '2026-03-31'`},
		{"income_statement_2026_03", `SELECT 0,
COALESCE(MAX(CASE WHEN item_name LIKE '%营业收入%' THEN current_amount END),0),
COALESCE(MAX(CASE WHEN item_name LIKE '%净利润%' THEN current_amount END),0),
COALESCE(SUM(CASE WHEN item_name LIKE '%营业成本%' OR item_name LIKE '%税金及附加%' OR item_name LIKE '%管理费用%' OR item_name LIKE '%财务费用%' THEN current_amount ELSE 0 END),0)
FROM fin_income_statement WHERE period='2026-03'`},
	}
	for _, q := range periodQueries {
		var rows int
		var a, b, c float64
		if err := db.QueryRowContext(ctx, q.sql).Scan(&rows, &a, &b, &c); err != nil {
			return nil, err
		}
		sections = append(sections, auditSection{Name: q.name, Rows: rows, Amounts: []auditAmount{{Name: "amount_1", Value: roundAuditAmount(a)}, {Name: "amount_2", Value: roundAuditAmount(b)}, {Name: "amount_3", Value: roundAuditAmount(c)}}})
	}
	entityQueries := []struct {
		name string
		sql  string
	}{
		{"feiweiyunke_2026_q1", contractEntityAmountAuditSQL("fin_fund_income", "fin_fund_income_groups", "received_amount", "year_month BETWEEN '2026-01' AND '2026-03'", "飞未云科")},
		{"linyue_2026_03_cost", contractEntityAmountAuditSQL("fin_cost_settlements", "fin_cost_settlement_groups", "paid_amount", "year_month='2026-03'", "林悦")},
		{"zhongxin_2026_03_mixed", `SELECT 0,
COALESCE((SELECT SUM(settlement_amount) FROM (` + contractEntityAmountRowsSQL("fin_fund_income", "fin_fund_income_groups", "received_amount", "year_month='2026-03'", "众信数通") + `) zhongxin_revenue),0),
COALESCE((SELECT SUM(settlement_amount) FROM (` + contractEntityAmountRowsSQL("fin_cost_settlements", "fin_cost_settlement_groups", "paid_amount", "year_month='2026-03'", "众信数通") + `) zhongxin_cost),0),
COALESCE((SELECT SUM(cash_amount) FROM (` + contractEntityAmountRowsSQL("fin_cost_settlements", "fin_cost_settlement_groups", "paid_amount", "year_month='2026-03'", "众信数通") + `) zhongxin_cost_cash),0)`},
	}
	for _, q := range entityQueries {
		var rows int
		var a, b, c float64
		if err := db.QueryRowContext(ctx, q.sql).Scan(&rows, &a, &b, &c); err != nil {
			return nil, err
		}
		sections = append(sections, auditSection{Name: q.name, Rows: rows, Amounts: []auditAmount{{Name: "amount_1", Value: roundAuditAmount(a)}, {Name: "amount_2", Value: roundAuditAmount(b)}, {Name: "amount_3", Value: roundAuditAmount(c)}}})
	}
	return sections, nil
}

func contractAmountAuditSQL(directTable, groupTable, cashColumn, filter string) string {
	return `SELECT COUNT(*), COALESCE(SUM(settlement_amount),0), COALESCE(SUM(cash_amount),0), COALESCE(SUM(invoice_amount),0)
FROM (` + contractAmountRowsSQL(directTable, groupTable, cashColumn, filter) + `) contract_amount_audit`
}

func contractAmountRowsSQL(directTable, groupTable, cashColumn, filter string) string {
	return `SELECT settlement_amount, ` + cashColumn + ` AS cash_amount, invoice_amount
FROM ` + directTable + `
WHERE ` + filter + `
UNION ALL
SELECT settlement_amount, ` + cashColumn + ` AS cash_amount, invoice_amount
FROM ` + groupTable + `
WHERE ` + filter
}

func contractEntityAmountAuditSQL(directTable, groupTable, cashColumn, filter, entity string) string {
	return `SELECT COUNT(*), COALESCE(SUM(settlement_amount),0), COALESCE(SUM(cash_amount),0), COALESCE(SUM(invoice_amount),0)
FROM (` + contractEntityAmountRowsSQL(directTable, groupTable, cashColumn, filter, entity) + `) contract_entity_amount_audit`
}

func contractEntityAmountRowsSQL(directTable, groupTable, cashColumn, filter, entity string) string {
	like := "%" + strings.ReplaceAll(entity, "'", "''") + "%"
	return `SELECT f.settlement_amount, f.` + cashColumn + ` AS cash_amount, f.invoice_amount
FROM ` + directTable + ` f
JOIN fin_contracts c ON c.contract_id = f.contract_id
WHERE f.` + filter + ` AND (c.customer_name LIKE '` + like + `' OR c.contract_content LIKE '` + like + `')
UNION ALL
SELECT g.settlement_amount, g.` + cashColumn + ` AS cash_amount, g.invoice_amount
FROM ` + groupTable + ` g
WHERE g.` + filter + ` AND g.customer_name LIKE '` + like + `'`
}

func roundAuditAmount(v float64) float64 {
	return math.Round(v*100) / 100
}

func downloadFinanceWorkbook(ctx context.Context, mappings []auditMapping) (string, error) {
	key := ""
	for _, mapping := range mappings {
		if mapping.TableType == "fund-income" || mapping.TableType == "cost-settlements" {
			key = mapping.StorageKey
			break
		}
	}
	if key == "" {
		return "", fmt.Errorf("no finance workbook storage_key in fin_file_mappings")
	}
	client, err := storage.NewOSSClientFromEnv()
	if err != nil {
		return "", err
	}
	if client == nil {
		return "", storage.ErrOSSNotConfigured
	}
	dest := filepath.Join("tmp", "accuracy-audit", filepath.Base(key))
	if err := client.DownloadToFile(ctx, key, dest); err != nil {
		return "", err
	}
	return dest, nil
}
