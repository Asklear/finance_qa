package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

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
	Incremental      bool
	CompanyOverride  string
	SourceFileName   string
	SourceStorageKey string
	SourceFileSize   int64
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
	if kind, ok, err := detectContractWorkbookKind(filePath); err != nil {
		return ImportSummary{}, err
	} else if ok {
		return i.importContractWorkbook(ctx, dbPath, filePath, kind, opts)
	}

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
	if err := annotateImportedReportSource(ctx, dbPath, result.Metadata, filePath, opts); err != nil {
		return ImportSummary{}, err
	}
	company := result.Metadata.Company
	if strings.TrimSpace(opts.CompanyOverride) != "" {
		company = strings.TrimSpace(opts.CompanyOverride)
	}
	return ImportSummary{
		FilePath:    filePath,
		ReportType:  result.Metadata.ReportType,
		Company:     company,
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
		if err := annotateImportedReportSource(ctx, dbPath, result.Metadata, filePath, opts); err != nil {
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
		return fmt.Errorf("bootstrap db: %w", err)
	}

	db, err := dbschema.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
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
	targetTable, err := resolvePhysicalTableName(ctx, tx, result.Metadata.ReportType)
	if err != nil {
		return err
	}

	policy, hasPolicy, err := loadIdempotencyPolicy(ctx, tx, result.Metadata.ReportType)
	if err != nil {
		return fmt.Errorf("load idempotency policy: %w", err)
	}
	shouldClear := !opts.Incremental
	if hasPolicy && policy.Enabled {
		shouldClear = policy.UpdateMode == "full_replace"
	}

	if shouldClear {
		if err := clearExisting(ctx, tx, targetTable, result); err != nil {
			return err
		}
	}

	existingKeys := map[string]struct{}{}
	fingerprintColumns := recordFingerprintColumns(result.Metadata.ReportType)
	if hasPolicy && policy.Enabled {
		existingKeys, err = loadExistingNormalizedKeys(ctx, tx, targetTable, result, fingerprintColumns, shouldClear)
		if err != nil {
			return fmt.Errorf("load existing dedupe keys: %w", err)
		}
	}
	batchKeys := map[string]struct{}{}

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

		if hasPolicy && policy.Enabled {
			key := normalizedRecordKey(row, fingerprintColumns)
			if key != "" {
				if _, ok := existingKeys[key]; ok {
					continue
				}
				if _, ok := batchKeys[key]; ok {
					continue
				}
				batchKeys[key] = struct{}{}
			}
		}

		if err := insertRecord(ctx, tx, targetTable, row); err != nil {
			return err
		}
	}

	if hasPolicy && policy.Enabled {
		if err := dedupeKeepLatest(ctx, tx, targetTable, policy.DedupeKeyColumns); err != nil {
			return fmt.Errorf("dedupe keep latest: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import: %w", err)
	}
	return nil
}

func loadIdempotencyPolicy(ctx context.Context, tx *sql.Tx, tableName string) (idempotencyPolicy, bool, error) {
	policyTable, err := resolvePolicyTableName(ctx, tx)
	if err != nil {
		return idempotencyPolicy{}, false, err
	}
	var mode, keys string
	var enabled int
	err = tx.QueryRowContext(ctx, fmt.Sprintf(`
SELECT update_mode, dedupe_key_columns, enabled
FROM %s
WHERE table_name = ? AND enabled = 1
`, policyTable), tableName).Scan(&mode, &keys, &enabled)
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

func recordFingerprintColumns(tableName string) []string {
	switch tableName {
	case "bank_statement":
		return []string{"company", "account_no", "account_name", "currency", "transaction_date", "transaction_time", "transaction_type", "debit_amount", "credit_amount", "balance", "summary", "counterparty_name", "counterparty_account"}
	case "journal":
		return []string{"company", "period", "voucher_date", "voucher_no", "account_code", "account_name", "summary", "direction", "amount", "debit_amount", "credit_amount", "counterparty"}
	case "income_statement":
		return []string{"company", "period", "item_name", "current_amount", "cumulative_amount"}
	case "balance_sheet":
		return []string{"company", "period", "account_code", "account_name", "opening_balance", "closing_balance"}
	case "balance_detail":
		return []string{"company", "year", "period", "opening_period", "account_code", "account_name", "account_level", "opening_debit", "opening_credit", "current_debit", "current_credit", "closing_debit", "closing_credit"}
	default:
		return defaultDedupeColumns(tableName)
	}
}

func dedupeKeepLatest(ctx context.Context, tx *sql.Tx, tableName string, keyColumns []string) error {
	keyColumns = sanitizeColumns(keyColumns)
	if len(keyColumns) == 0 {
		return nil
	}

	var keyExprs []string
	for _, col := range keyColumns {
		keyExprs = append(keyExprs, fmt.Sprintf("TRIM(COALESCE(CAST(%s AS TEXT), ''))", col))
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
		"year":                 {},
		"period":               {},
		"opening_period":       {},
		"account_name":         {},
		"account_code":         {},
		"account_level":        {},
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
		"direction":            {},
		"amount":               {},
		"counterparty":         {},
		"opening_balance":      {},
		"closing_balance":      {},
		"current_amount":       {},
		"cumulative_amount":    {},
		"opening_debit":        {},
		"opening_credit":       {},
		"current_debit":        {},
		"current_credit":       {},
		"closing_debit":        {},
		"closing_credit":       {},
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

func loadExistingNormalizedKeys(ctx context.Context, tx *sql.Tx, tableName string, result parser.ParseResult, keyColumns []string, shouldClear bool) (map[string]struct{}, error) {
	keys := map[string]struct{}{}
	if shouldClear {
		return keys, nil
	}

	cols := sanitizeColumns(keyColumns)
	if len(cols) == 0 {
		return keys, nil
	}

	whereSQL, args := existingKeyScope(result)
	query := fmt.Sprintf("SELECT %s FROM %s %s", strings.Join(cols, ", "), tableName, whereSQL)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		values, err := scanRowValues(rows, len(cols))
		if err != nil {
			return nil, err
		}
		key := normalizedValuesKey(values)
		if key == "" {
			continue
		}
		keys[key] = struct{}{}
	}
	return keys, rows.Err()
}

func existingKeyScope(result parser.ParseResult) (string, []any) {
	company := strings.TrimSpace(result.Metadata.Company)
	switch result.Metadata.ReportType {
	case "income_statement", "balance_sheet", "balance_detail":
		period := strings.TrimSpace(result.Metadata.PeriodEnd)
		if company != "" && period != "" {
			return "WHERE company = ? AND period = ?", []any{company, period}
		}
	case "journal":
		minDate, maxDate := recordDateRange(result.Data, "voucher_date", "date")
		if company != "" && minDate != "" && maxDate != "" {
			return "WHERE company = ? AND voucher_date BETWEEN ? AND ?", []any{company, minDate, maxDate}
		}
	case "bank_statement":
		minDate, maxDate := recordDateRange(result.Data, "transaction_date")
		if company != "" && minDate != "" && maxDate != "" {
			return "WHERE company = ? AND transaction_date BETWEEN ? AND ?", []any{company, minDate, maxDate}
		}
	}
	if company != "" {
		return "WHERE company = ?", []any{company}
	}
	return "", nil
}

func recordDateRange(rows []parser.Record, fields ...string) (string, string) {
	minDate := ""
	maxDate := ""
	for _, row := range rows {
		value := ""
		for _, field := range fields {
			value = strings.TrimSpace(fmt.Sprintf("%v", row[field]))
			if value != "" && value != "<nil>" {
				break
			}
		}
		if value == "" || value == "<nil>" {
			continue
		}
		if minDate == "" || value < minDate {
			minDate = value
		}
		if maxDate == "" || value > maxDate {
			maxDate = value
		}
	}
	return minDate, maxDate
}

func scanRowValues(rows *sql.Rows, count int) ([]any, error) {
	values := make([]any, count)
	dest := make([]any, count)
	for i := range values {
		dest[i] = &values[i]
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, err
	}
	return values, nil
}

func normalizedRecordKey(row parser.Record, keyColumns []string) string {
	cols := sanitizeColumns(keyColumns)
	if len(cols) == 0 {
		return ""
	}
	values := make([]any, 0, len(cols))
	for _, col := range cols {
		values = append(values, row[col])
	}
	return normalizedValuesKey(values)
}

func normalizedValuesKey(values []any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, normalizeKeyValue(value))
	}
	return strings.Join(parts, "\x1f")
}

func normalizeKeyValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []byte:
		return strings.TrimSpace(string(v))
	case float64:
		return fmt.Sprintf("%.6f", v)
	case float32:
		return fmt.Sprintf("%.6f", v)
	case int:
		return fmt.Sprintf("%.6f", float64(v))
	case int64:
		return fmt.Sprintf("%.6f", float64(v))
	case int32:
		return fmt.Sprintf("%.6f", float64(v))
	case uint:
		return fmt.Sprintf("%.6f", float64(v))
	case uint64:
		return fmt.Sprintf("%.6f", float64(v))
	case uint32:
		return fmt.Sprintf("%.6f", float64(v))
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
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
	bankTable, err := resolvePhysicalTableName(ctx, tx, "bank_statement")
	if err != nil {
		return nil, err
	}
	journalTable, err := resolvePhysicalTableName(ctx, tx, "journal")
	if err != nil {
		return nil, err
	}
	balanceSheetTable, err := resolvePhysicalTableName(ctx, tx, "balance_sheet")
	if err != nil {
		return nil, err
	}
	incomeTable, err := resolvePhysicalTableName(ctx, tx, "income_statement")
	if err != nil {
		return nil, err
	}
	balanceDetailTable, err := resolvePhysicalTableName(ctx, tx, "balance_detail")
	if err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`
SELECT DISTINCT company FROM (
  SELECT company FROM %s
  UNION ALL SELECT company FROM %s
  UNION ALL SELECT company FROM %s
  UNION ALL SELECT company FROM %s
  UNION ALL SELECT company FROM %s
)
WHERE company IS NOT NULL AND TRIM(company) <> ''
`, bankTable, journalTable, balanceSheetTable, incomeTable, balanceDetailTable))
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

func clearExisting(ctx context.Context, tx *sql.Tx, tableName string, result parser.ParseResult) error {
	company := result.Metadata.Company
	period := result.Metadata.PeriodEnd

	switch result.Metadata.ReportType {
	case "bank_statement":
		_, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE company = ?`, tableName), company)
		return err
	case "income_statement":
		// Clear all data for this company + period to avoid duplicates
		_, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE company = ? AND period = ?`, tableName), company, period)
		return err
	case "balance_sheet":
		_, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE company = ? AND period = ?`, tableName), company, period)
		return err
	case "balance_detail":
		// Clear ALL balance_detail for this company (余额表是全量替换)
		_, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE company = ?`, tableName), company)
		return err
	case "journal":
		_, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE company = ?`, tableName), company)
		return err
	default:
		return nil
	}
}

func insertRecord(ctx context.Context, tx *sql.Tx, tableName string, row parser.Record) error {
	switch {
	case strings.HasSuffix(tableName, "bank_statement"):
		_, err := tx.ExecContext(ctx, `
INSERT INTO `+tableName+`
  (company, account_no, account_name, currency, transaction_date, transaction_time, transaction_type, debit_amount, credit_amount, balance, summary, counterparty_name, counterparty_account)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, row["company"], row["account_no"], row["account_name"], row["currency"], row["transaction_date"], row["transaction_time"], row["transaction_type"], row["debit_amount"], row["credit_amount"], row["balance"], row["summary"], row["counterparty_name"], row["counterparty_account"])
		return wrapInsertErr(tableName, err)
	case strings.HasSuffix(tableName, "income_statement"):
		_, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO `+tableName+`
  (company, period, item_name, current_amount, cumulative_amount)
VALUES (?, ?, ?, ?, ?)
`, row["company"], row["period"], row["item_name"], row["current_amount"], row["cumulative_amount"])
		return wrapInsertErr(tableName, err)
	case strings.HasSuffix(tableName, "balance_sheet"):
		_, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO `+tableName+`
  (company, period, account_code, account_name, opening_balance, closing_balance)
VALUES (?, ?, ?, ?, ?, ?)
`, row["company"], row["period"], row["account_code"], row["account_name"], row["opening_balance"], row["closing_balance"])
		return wrapInsertErr(tableName, err)
	case strings.HasSuffix(tableName, "balance_detail"):
		_, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO `+tableName+`
  (company, year, period, opening_period, account_code, account_name, account_level, opening_debit, opening_credit, current_debit, current_credit, closing_debit, closing_credit)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, row["company"], row["year"], row["period"], row["opening_period"], row["account_code"], row["account_name"], row["account_level"], row["opening_debit"], row["opening_credit"], row["current_debit"], row["current_credit"], row["closing_debit"], row["closing_credit"])
		return wrapInsertErr(tableName, err)
	case strings.HasSuffix(tableName, "journal"):
		voucherDate := row["voucher_date"]
		if voucherDate == nil {
			voucherDate = row["date"]
		}
		_, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO `+tableName+`
  (company, period, voucher_date, voucher_no, account_code, account_name, summary, direction, amount, debit_amount, credit_amount, counterparty)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, row["company"], row["period"], voucherDate, row["voucher_no"], row["account_code"], row["account_name"], row["summary"], row["direction"], row["amount"], row["debit_amount"], row["credit_amount"], row["counterparty"])
		return wrapInsertErr(tableName, err)
	default:
		return fmt.Errorf("unsupported report table %q", tableName)
	}
}

func wrapInsertErr(reportType string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("insert %s row: %w", reportType, err)
}

func resolvePolicyTableName(ctx context.Context, tx *sql.Tx) (string, error) {
	return resolveTableNameWithFallback(ctx, tx, "fin_table_idempotency_policies", "table_idempotency_policies")
}

func resolvePhysicalTableName(ctx context.Context, tx *sql.Tx, logical string) (string, error) {
	return resolveTableNameWithFallback(ctx, tx, "fin_"+logical, logical)
}

func resolveTableNameWithFallback(ctx context.Context, tx *sql.Tx, primary, fallback string) (string, error) {
	ok, err := tableExists(ctx, tx, primary)
	if err != nil {
		return "", err
	}
	if ok {
		return primary, nil
	}
	ok, err = tableExists(ctx, tx, fallback)
	if err != nil {
		return "", err
	}
	if ok {
		return fallback, nil
	}
	return "", fmt.Errorf("table not found: %s or %s", primary, fallback)
}

func tableExists(ctx context.Context, tx *sql.Tx, tableName string) (bool, error) {
	var name string
	err := tx.QueryRowContext(ctx, `
SELECT table_name
FROM information_schema.tables
WHERE table_name = ?
LIMIT 1
`, tableName).Scan(&name)
	if err == nil {
		return true, nil
	}
	if err != sql.ErrNoRows {
		// sqlite fallback
		sqliteErr := tx.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tableName).Scan(&name)
		if sqliteErr == nil {
			return true, nil
		}
		if sqliteErr == sql.ErrNoRows {
			return false, nil
		}
		return false, sqliteErr
	}
	return false, nil
}
