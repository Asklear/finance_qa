package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	dbschema "financeqa/internal/db"
	"financeqa/internal/dimensions"
	"financeqa/internal/parser"
)

type ImportSummary struct {
	FilePath    string `json:"filePath"`
	ReportType  string `json:"reportType"`
	Company     string `json:"company"`
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
	RecordCount int    `json:"recordCount"`
}

type Importer struct {
	dim *dimensions.Manager
}

type ImportOptions struct {
	Incremental     bool
	CompanyOverride string
}

type idempotencyPolicy struct {
	UpdateMode       string
	DedupeKeyColumns []string
	Enabled          bool
}

func NewImporter(dim *dimensions.Manager) *Importer {
	return &Importer{dim: dim}
}

func (i *Importer) ParseFile(path string) (parser.ParseResult, error) {
	return parser.ParseFile(path)
}

func (i *Importer) ImportFile(ctx context.Context, dbPath, filePath string, incremental bool) (ImportSummary, error) {
	return i.ImportFileWithOptions(ctx, dbPath, filePath, ImportOptions{
		Incremental: incremental,
	})
}

func (i *Importer) ImportFileWithOptions(ctx context.Context, dbPath, filePath string, opts ImportOptions) (ImportSummary, error) {
	// Compatibility mode for merged "财报" files:
	// try importing both balance_sheet(sheet1) and income_statement(sheet2)
	// while keeping legacy separate-file imports unchanged.
	if strings.Contains(strings.ToLower(filepath.Base(filePath)), "财报") {
		return i.importMergedFinancialReport(ctx, dbPath, filePath, opts)
	}

	result, err := parser.ParseFile(filePath)
	if err != nil {
		return ImportSummary{}, err
	}
	if err := i.ImportParsedWithOptions(ctx, dbPath, result, opts); err != nil {
		return ImportSummary{}, err
	}
	return ImportSummary{
		FilePath:    filePath,
		ReportType:  result.Metadata.ReportType,
		Company:     result.Metadata.Company,
		PeriodStart: result.Metadata.PeriodStart,
		PeriodEnd:   result.Metadata.PeriodEnd,
		RecordCount: len(result.Data),
	}, nil
}

func (i *Importer) importMergedFinancialReport(ctx context.Context, dbPath, filePath string, opts ImportOptions) (ImportSummary, error) {
	totalRecords := 0
	company := ""
	periodStart := ""
	periodEnd := ""
	importedTypes := make([]string, 0, 2)

	types := []string{"balance_sheet", "income_statement"}
	for _, typ := range types {
		result, err := parser.ParseFileAsType(filePath, typ)
		if err != nil {
			// keep compatibility: if one sheet format differs, continue with the other type
			continue
		}
		if len(result.Data) == 0 {
			continue
		}
		if err := i.ImportParsedWithOptions(ctx, dbPath, result, opts); err != nil {
			return ImportSummary{}, err
		}
		totalRecords += len(result.Data)
		importedTypes = append(importedTypes, typ)
		if company == "" {
			company = result.Metadata.Company
		}
		if periodStart == "" {
			periodStart = result.Metadata.PeriodStart
		}
		if periodEnd == "" {
			periodEnd = result.Metadata.PeriodEnd
		}
	}

	if len(importedTypes) == 0 {
		return ImportSummary{}, fmt.Errorf("merged financial report parse failed: no sheet imported from %s", filePath)
	}

	return ImportSummary{
		FilePath:    filePath,
		ReportType:  strings.Join(importedTypes, "+"),
		Company:     company,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		RecordCount: totalRecords,
	}, nil
}

func (i *Importer) ImportParsed(ctx context.Context, dbPath string, result parser.ParseResult, incremental bool) error {
	return i.ImportParsedWithOptions(ctx, dbPath, result, ImportOptions{
		Incremental: incremental,
	})
}

func (i *Importer) ImportParsedWithOptions(ctx context.Context, dbPath string, result parser.ParseResult, opts ImportOptions) error {
	if err := dbschema.Bootstrap(ctx, dbPath); err != nil {
		return fmt.Errorf("bootstrap sqlite db: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite db: %w", err)
	}
	defer func() { _ = db.Close() }()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	resolvedCompany, err := resolveCompanyForImport(ctx, tx, result.Metadata.Company, opts.CompanyOverride)
	if err != nil {
		return err
	}
	result.Metadata.Company = resolvedCompany

	policy, hasPolicy, err := loadIdempotencyPolicy(ctx, tx, result.Metadata.ReportType)
	if err != nil {
		return fmt.Errorf("load idempotency policy: %w", err)
	}
	shouldClear := !opts.Incremental
	if hasPolicy && policy.Enabled {
		shouldClear = policy.UpdateMode == "full_replace"
	}

	if shouldClear {
		if err := clearExisting(ctx, tx, result); err != nil {
			return err
		}
	}

	for _, row := range result.Data {
		if resolvedCompany != "" {
			row["company"] = resolvedCompany
		}
		// Auto-map missing account codes across all report types using dimension manager
		if (row["account_code"] == nil || row["account_code"] == "") && row["account_name"] != nil && row["account_name"] != "" {
			if i.dim != nil {
				accName := fmt.Sprintf("%v", row["account_name"])
				// Clean company-specific prefixes if needed (e.g. trim whitespace)
				accName = strings.TrimSpace(accName)
				if m, err := i.dim.ResolveMemberByName(ctx, result.Metadata.Company, "CAS", accName); err == nil {
					row["account_code"] = m.Code
				}
			}
		}

		if err := insertRecord(ctx, tx, result.Metadata.ReportType, row); err != nil {
			return err
		}
	}

	if hasPolicy && policy.Enabled {
		if err := dedupeKeepLatest(ctx, tx, result.Metadata.ReportType, policy.DedupeKeyColumns); err != nil {
			return fmt.Errorf("dedupe keep latest: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import: %w", err)
	}
	return nil
}

func loadIdempotencyPolicy(ctx context.Context, tx *sql.Tx, tableName string) (idempotencyPolicy, bool, error) {
	var mode, keys string
	var enabled int
	err := tx.QueryRowContext(ctx, `
SELECT update_mode, dedupe_key_columns, enabled
FROM table_idempotency_policies
WHERE table_name = ? AND enabled = 1
`, tableName).Scan(&mode, &keys, &enabled)
	if err != nil {
		if err == sql.ErrNoRows {
			return idempotencyPolicy{}, false, nil
		}
		return idempotencyPolicy{}, false, err
	}

	keyCols := parseColumns(keys)
	if len(keyCols) == 0 {
		keyCols = defaultDedupeColumns(tableName)
	}
	return idempotencyPolicy{
		UpdateMode:       mode,
		DedupeKeyColumns: keyCols,
		Enabled:          enabled == 1,
	}, true, nil
}

func parseColumns(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		col := strings.TrimSpace(part)
		if col == "" {
			continue
		}
		out = append(out, col)
	}
	return out
}

func defaultDedupeColumns(tableName string) []string {
	switch tableName {
	case "bank_statement":
		return []string{"company", "account_no", "account_name", "currency", "transaction_date", "transaction_time", "transaction_type", "debit_amount", "credit_amount", "balance", "summary", "counterparty_name", "counterparty_account"}
	case "journal":
		return []string{"company", "voucher_date", "voucher_no", "account_code", "summary", "debit_amount", "credit_amount"}
	case "income_statement":
		return []string{"company", "period", "item_name"}
	case "balance_sheet":
		return []string{"company", "period", "account_name"}
	case "balance_detail":
		return []string{"company", "period", "account_code"}
	default:
		return nil
	}
}

func dedupeKeepLatest(ctx context.Context, tx *sql.Tx, tableName string, keyColumns []string) error {
	keyColumns = sanitizeColumns(keyColumns)
	if len(keyColumns) == 0 {
		return nil
	}

	var keyExprs []string
	for _, col := range keyColumns {
		keyExprs = append(keyExprs, fmt.Sprintf("COALESCE(CAST(%s AS TEXT), '')", col))
	}
	partition := strings.Join(keyExprs, ", ")
	query := fmt.Sprintf(`
DELETE FROM %s
WHERE id IN (
  SELECT id FROM (
    SELECT id,
           ROW_NUMBER() OVER (
             PARTITION BY %s
             ORDER BY imported_at DESC, id DESC
           ) AS rn
    FROM %s
  )
  WHERE rn > 1
)
`, tableName, partition, tableName)
	_, err := tx.ExecContext(ctx, query)
	return err
}

func sanitizeColumns(cols []string) []string {
	allowed := map[string]struct{}{
		"company":              {},
		"period":               {},
		"account_name":         {},
		"account_code":         {},
		"item_name":            {},
		"account_no":           {},
		"currency":             {},
		"transaction_date":     {},
		"transaction_time":     {},
		"transaction_type":     {},
		"debit_amount":         {},
		"credit_amount":        {},
		"balance":              {},
		"summary":              {},
		"counterparty_name":    {},
		"counterparty_account": {},
		"voucher_date":         {},
		"voucher_no":           {},
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(cols))
	for _, col := range cols {
		c := strings.TrimSpace(col)
		if _, ok := allowed[c]; !ok {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func resolveCompanyForImport(ctx context.Context, tx *sql.Tx, parsedCompany, override string) (string, error) {
	override = strings.TrimSpace(override)
	if override != "" {
		return override, nil
	}

	parsed := strings.TrimSpace(parsedCompany)
	if parsed == "" || isInvalidDerivedCompany(parsed) {
		inferred, err := inferSingleKnownCompany(ctx, tx)
		if err != nil {
			return "", err
		}
		if inferred != "" {
			return inferred, nil
		}
		return "", fmt.Errorf("unable to determine company from file metadata (%q), please pass --company", parsedCompany)
	}

	canonical, err := canonicalizeCompanyAlias(ctx, tx, parsed)
	if err != nil {
		return "", err
	}
	if canonical != "" {
		return canonical, nil
	}
	return parsed, nil
}

func isInvalidDerivedCompany(company string) bool {
	c := strings.TrimSpace(company)
	if c == "" {
		return true
	}
	if strings.EqualFold(c, "defaultcompany") {
		return true
	}
	matched, _ := regexp.MatchString(`^\d{8}-\d{8}$`, c)
	return matched
}

func canonicalizeCompanyAlias(ctx context.Context, tx *sql.Tx, company string) (string, error) {
	candidates, err := listKnownCompanies(ctx, tx)
	if err != nil {
		return "", err
	}
	matches := make([]string, 0, 2)
	for _, c := range candidates {
		if strings.Contains(c, company) || strings.Contains(company, c) {
			matches = append(matches, c)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", nil
}

func inferSingleKnownCompany(ctx context.Context, tx *sql.Tx) (string, error) {
	candidates, err := listKnownCompanies(ctx, tx)
	if err != nil {
		return "", err
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return "", nil
}

func listKnownCompanies(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT DISTINCT company FROM (
  SELECT company FROM bank_statement
  UNION ALL SELECT company FROM journal
  UNION ALL SELECT company FROM balance_sheet
  UNION ALL SELECT company FROM income_statement
  UNION ALL SELECT company FROM balance_detail
)
WHERE company IS NOT NULL AND TRIM(company) <> ''
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	uniq := map[string]struct{}{}
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		c = strings.TrimSpace(c)
		if c == "" || isInvalidDerivedCompany(c) {
			continue
		}
		if _, ok := uniq[c]; ok {
			continue
		}
		uniq[c] = struct{}{}
		out = append(out, c)
	}
	sort.Strings(out)
	return out, rows.Err()
}

func clearExisting(ctx context.Context, tx *sql.Tx, result parser.ParseResult) error {
	company := result.Metadata.Company
	period := result.Metadata.PeriodEnd

	switch result.Metadata.ReportType {
	case "bank_statement":
		_, err := tx.ExecContext(ctx, `DELETE FROM bank_statement WHERE company = ?`, company)
		return err
	case "income_statement":
		// Clear all data for this company + period to avoid duplicates
		_, err := tx.ExecContext(ctx, `DELETE FROM income_statement WHERE company = ? AND period = ?`, company, period)
		return err
	case "balance_sheet":
		_, err := tx.ExecContext(ctx, `DELETE FROM balance_sheet WHERE company = ? AND period = ?`, company, period)
		return err
	case "balance_detail":
		// Clear ALL balance_detail for this company (余额表是全量替换)
		_, err := tx.ExecContext(ctx, `DELETE FROM balance_detail WHERE company = ?`, company)
		return err
	case "journal":
		_, err := tx.ExecContext(ctx, `DELETE FROM journal WHERE company = ?`, company)
		return err
	default:
		return nil
	}
}

func insertRecord(ctx context.Context, tx *sql.Tx, reportType string, row parser.Record) error {
	switch reportType {
	case "bank_statement":
		_, err := tx.ExecContext(ctx, `
INSERT INTO bank_statement
  (company, account_no, account_name, currency, transaction_date, transaction_time, transaction_type, debit_amount, credit_amount, balance, summary, counterparty_name, counterparty_account)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, row["company"], row["account_no"], row["account_name"], row["currency"], row["transaction_date"], row["transaction_time"], row["transaction_type"], row["debit_amount"], row["credit_amount"], row["balance"], row["summary"], row["counterparty_name"], row["counterparty_account"])
		return wrapInsertErr(reportType, err)
	case "income_statement":
		_, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO income_statement
  (company, period, item_name, current_amount, cumulative_amount)
VALUES (?, ?, ?, ?, ?)
`, row["company"], row["period"], row["item_name"], row["current_amount"], row["cumulative_amount"])
		return wrapInsertErr(reportType, err)
	case "balance_sheet":
		_, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO balance_sheet
  (company, period, account_code, account_name, opening_balance, closing_balance)
VALUES (?, ?, ?, ?, ?, ?)
`, row["company"], row["period"], row["account_code"], row["account_name"], row["opening_balance"], row["closing_balance"])
		return wrapInsertErr(reportType, err)
	case "balance_detail":
		_, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO balance_detail
  (company, year, period, account_code, account_name, account_level, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, row["company"], row["year"], row["period"], row["account_code"], row["account_name"], row["account_level"], row["opening_debit"], row["opening_credit"], row["current_debit"], row["current_credit"], row["closing_debit"], row["closing_credit"])
		return wrapInsertErr(reportType, err)
	case "journal":
		voucherDate := row["voucher_date"]
		if voucherDate == nil {
			voucherDate = row["date"]
		}
		_, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO journal
  (company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, row["company"], row["period"], voucherDate, row["voucher_no"], row["account_code"], row["account_name"], row["summary"], row["direction"], row["amount"], row["debit_amount"], row["credit_amount"], row["counterparty"])
		return wrapInsertErr(reportType, err)
	default:
		return fmt.Errorf("unsupported report type %q", reportType)
	}
}

func wrapInsertErr(reportType string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("insert %s row: %w", reportType, err)
}
