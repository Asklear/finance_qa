package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	dbschema "financeqa/internal/db"
)

func annotateImportedReportSource(ctx context.Context, dbPath, reportType, filePath string) error {
	db, err := dbschema.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open db for source metadata: %w", err)
	}
	defer func() { _ = db.Close() }()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin source metadata tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tableName, err := resolvePhysicalTableName(ctx, tx, reportType)
	if err != nil {
		return err
	}
	meta := dbschema.BuildImportedTableSourceMetadata(tableName, filePath, []string{reportType}, nil, "")
	if err := upsertImportedSourceMetadata(ctx, tx, dbPath, tableName, meta, "imported", true); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit source metadata tx: %w", err)
	}
	return nil
}

func annotateContractWorkbookSource(ctx context.Context, tx *sql.Tx, dbPath, filePath string, bundle contractImportBundle, opts ImportOptions) error {
	fileName := workbookDisplayName(filePath)
	incremental := opts.Incremental

	if len(bundle.ContractSourceSheets) > 0 {
		meta := dbschema.BuildImportedTableSourceMetadata(
			"fin_contracts",
			filePath,
			[]string{string(bundle.Kind)},
			bundle.ContractSourceSheets,
			"",
		)
		meta.Display = "《合同信息表》"
		if err := upsertImportedSourceMetadata(ctx, tx, dbPath, "fin_contracts", meta, "contract", incremental); err != nil {
			return err
		}
	}

	if sheets := bundle.TableSourceSheets["fin_fund_income"]; len(sheets) > 0 {
		meta := dbschema.BuildImportedTableSourceMetadata(
			"fin_fund_income",
			filePath,
			[]string{string(bundle.Kind)},
			sheets,
			formatWorkbookSheetDisplay(fileName, sheets),
		)
		if err := upsertImportedSourceMetadata(ctx, tx, dbPath, "fin_fund_income", meta, "imported", incremental); err != nil {
			return err
		}
	}

	if sheets := bundle.TableSourceSheets["fin_cost_settlements"]; len(sheets) > 0 {
		meta := dbschema.BuildImportedTableSourceMetadata(
			"fin_cost_settlements",
			filePath,
			[]string{string(bundle.Kind)},
			sheets,
			formatWorkbookSheetDisplay(fileName, sheets),
		)
		if err := upsertImportedSourceMetadata(ctx, tx, dbPath, "fin_cost_settlements", meta, "imported", incremental); err != nil {
			return err
		}
	}
	if err := upsertContractWorkbookFileMappings(ctx, tx, dbPath, filePath, bundle, opts); err != nil {
		return err
	}
	return nil
}

type contractWorkbookFileMapping struct {
	TableType   string
	Period      string
	Description string
}

func contractWorkbookFileMappings(bundle contractImportBundle) []contractWorkbookFileMapping {
	periodsByType := map[string]map[string]struct{}{}
	add := func(tableType, yearMonth string) {
		period := contractFinanceMappingPeriod(yearMonth)
		if tableType == "" || period == "" {
			return
		}
		if periodsByType[tableType] == nil {
			periodsByType[tableType] = map[string]struct{}{}
		}
		periodsByType[tableType][period] = struct{}{}
	}
	for _, row := range bundle.RevenueRows {
		add("fund-income", row.YearMonth)
	}
	for _, row := range bundle.FundRows {
		add("fund-income", row.YearMonth)
	}
	for _, row := range bundle.FundGroupRows {
		add("fund-income", row.YearMonth)
	}
	for _, row := range bundle.CostRows {
		add("cost-settlements", row.YearMonth)
	}
	for _, row := range bundle.CostGroupRows {
		add("cost-settlements", row.YearMonth)
	}

	tableTypes := make([]string, 0, len(periodsByType))
	for tableType := range periodsByType {
		tableTypes = append(tableTypes, tableType)
	}
	sort.Strings(tableTypes)
	out := make([]contractWorkbookFileMapping, 0)
	for _, tableType := range tableTypes {
		periods := make([]string, 0, len(periodsByType[tableType]))
		for period := range periodsByType[tableType] {
			periods = append(periods, period)
		}
		sort.Strings(periods)
		for _, period := range periods {
			out = append(out, contractWorkbookFileMapping{
				TableType:   tableType,
				Period:      period,
				Description: contractWorkbookFileMappingDescription(tableType, period),
			})
		}
	}
	return out
}

func contractFinanceMappingPeriod(yearMonth string) string {
	value := strings.TrimSpace(yearMonth)
	if value == "" {
		return ""
	}
	if len(value) == len("2006-01") && value[4] == '-' {
		year := value[:4]
		month := value[5:]
		switch month {
		case "01", "02", "03":
			return year + "-Q1"
		case "04", "05", "06":
			return year + "-Q2"
		case "07", "08", "09":
			return year + "-Q3"
		case "10", "11", "12":
			return year + "-Q4"
		}
	}
	return value
}

func contractWorkbookFileMappingDescription(tableType, period string) string {
	switch strings.TrimSpace(tableType) {
	case "fund-income":
		return strings.TrimSpace(period) + "资金收入表（飞书财务表自动同步）"
	case "cost-settlements":
		return strings.TrimSpace(period) + "合同成本表（飞书财务表自动同步）"
	default:
		return strings.TrimSpace(period) + "财务表（飞书财务表自动同步）"
	}
}

func contractWorkbookFileMappingTableTypes(mappings []contractWorkbookFileMapping) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, mapping := range mappings {
		tableType := strings.TrimSpace(mapping.TableType)
		if tableType == "" {
			continue
		}
		if _, ok := seen[tableType]; ok {
			continue
		}
		seen[tableType] = struct{}{}
		out = append(out, tableType)
	}
	sort.Strings(out)
	return out
}

func nullableFileSize(fileSize int64) any {
	if fileSize <= 0 {
		return nil
	}
	return fileSize
}

func upsertContractWorkbookFileMappings(ctx context.Context, tx *sql.Tx, dbPath, filePath string, bundle contractImportBundle, opts ImportOptions) error {
	mappings := contractWorkbookFileMappings(bundle)
	if len(mappings) == 0 {
		return nil
	}
	if err := ensureFinanceFileMappingsTable(ctx, tx, dbPath); err != nil {
		return err
	}
	fileName := strings.TrimSpace(opts.SourceFileName)
	if fileName == "" {
		fileName = workbookDisplayName(filePath)
	}
	storageKey := strings.TrimSpace(opts.SourceStorageKey)
	if storageKey == "" {
		storageKey = strings.TrimSpace(filePath)
	}
	if fileName == "" || storageKey == "" {
		return nil
	}
	company := strings.TrimSpace(opts.CompanyOverride)
	if company == "" {
		company = "DefaultCompany"
	}
	fileSize := opts.SourceFileSize
	if fileSize <= 0 {
		if info, err := os.Stat(filePath); err == nil {
			fileSize = info.Size()
		}
	}

	if !opts.Incremental {
		for _, tableType := range contractWorkbookFileMappingTableTypes(mappings) {
			if _, err := tx.ExecContext(ctx, `
DELETE FROM fin_file_mappings
WHERE LOWER(COALESCE(table_type, '')) = ?
  AND (
    COALESCE(company, '') = ''
    OR ? = ''
    OR ? LIKE '%' || COALESCE(company, '') || '%'
    OR COALESCE(company, '') LIKE '%' || ? || '%'
    OR COALESCE(file_name, '') = ?
    OR COALESCE(storage_key, '') = ?
  )
`, strings.ToLower(tableType), company, company, company, fileName, storageKey); err != nil {
				return fmt.Errorf("delete stale finance file mappings for %s: %w", tableType, err)
			}
		}
	}

	for _, mapping := range mappings {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM fin_file_mappings
WHERE LOWER(COALESCE(table_type, '')) = ?
  AND COALESCE(period, '') = ?
  AND (
    COALESCE(company, '') = ?
    OR ? LIKE '%' || COALESCE(company, '') || '%'
    OR COALESCE(company, '') LIKE '%' || ? || '%'
    OR COALESCE(file_name, '') = ?
    OR COALESCE(storage_key, '') = ?
  )
`, strings.ToLower(mapping.TableType), mapping.Period, company, company, company, fileName, storageKey); err != nil {
			return fmt.Errorf("dedupe finance file mapping %s/%s: %w", mapping.TableType, mapping.Period, err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO fin_file_mappings(table_type, period, company, storage_key, file_name, description, file_size, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, mapping.TableType, mapping.Period, company, storageKey, fileName, mapping.Description, nullableFileSize(fileSize)); err != nil {
			return fmt.Errorf("insert finance file mapping %s/%s: %w", mapping.TableType, mapping.Period, err)
		}
	}
	return nil
}

func ensureFinanceFileMappingsTable(ctx context.Context, tx *sql.Tx, dbPath string) error {
	ddl := `
CREATE TABLE IF NOT EXISTS fin_file_mappings (
	id BIGSERIAL PRIMARY KEY,
	table_type VARCHAR(64) NOT NULL,
	period VARCHAR(32) NOT NULL,
	company VARCHAR(255),
	storage_key VARCHAR(1024) NOT NULL,
	file_name VARCHAR(255) NOT NULL,
	description TEXT,
	file_size BIGINT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)`
	if looksLikeSQLiteImportPath(dbPath) {
		ddl = `
CREATE TABLE IF NOT EXISTS fin_file_mappings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	table_type TEXT NOT NULL,
	period TEXT NOT NULL,
	company TEXT,
	storage_key TEXT NOT NULL,
	file_name TEXT NOT NULL,
	description TEXT,
	file_size INTEGER,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)`
	}
	if _, err := tx.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("ensure fin_file_mappings table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_fin_files_table_type ON fin_file_mappings(table_type)`); err != nil {
		return fmt.Errorf("ensure fin_file_mappings table_type index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_fin_files_period ON fin_file_mappings(period)`); err != nil {
		return fmt.Errorf("ensure fin_file_mappings period index: %w", err)
	}
	return nil
}

func looksLikeSQLiteImportPath(dbPath string) bool {
	value := strings.ToLower(strings.TrimSpace(dbPath))
	if value == "" || value == ":memory:" {
		return true
	}
	if strings.HasSuffix(value, ".db") || strings.HasSuffix(value, ".sqlite") || strings.HasSuffix(value, ".sqlite3") {
		return true
	}
	if strings.Contains(value, "://") || strings.Contains(value, "host=") || strings.Contains(value, "postgres") {
		return false
	}
	return strings.Contains(value, string(os.PathSeparator))
}

func upsertImportedSourceMetadata(ctx context.Context, tx *sql.Tx, dbPath, tableName string, incoming dbschema.TableSourceMetadata, displayMode string, mergeExisting bool) error {
	if mergeExisting {
		loaded, err := dbschema.LoadTableSourceMetadata(ctx, tx, dbPath, []string{tableName})
		if err == nil {
			if existing, ok := loaded[tableName]; ok {
				incoming.FileNames = mergeSourceMetadataStrings(existing.FileNames, incoming.FileNames)
				incoming.SheetNames = mergeSourceMetadataStrings(existing.SheetNames, incoming.SheetNames)
				incoming.ReportTypes = mergeSourceMetadataStrings(existing.ReportTypes, incoming.ReportTypes)
				incoming.Notes = mergeSourceMetadataStrings(existing.Notes, incoming.Notes)
			}
		}
	}
	switch strings.TrimSpace(displayMode) {
	case "contract":
		incoming.Display = "《合同信息表》"
	default:
		incoming.Display = formatMergedWorkbookDisplay(incoming.FileNames, incoming.SheetNames, incoming.Display)
	}
	return dbschema.UpsertTableSourceMetadata(ctx, tx, dbPath, tableName, incoming)
}

func mergeSourceMetadataStrings(left, right []string) []string {
	out := make([]string, 0, len(left)+len(right))
	seen := map[string]struct{}{}
	for _, values := range [][]string{left, right} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}

func formatMergedWorkbookDisplay(fileNames, sheetNames []string, fallback string) string {
	fileNames = mergeSourceMetadataStrings(nil, fileNames)
	switch len(fileNames) {
	case 0:
		return strings.TrimSpace(fallback)
	case 1:
		return formatWorkbookSheetDisplay(fileNames[0], sheetNames)
	default:
		parts := make([]string, 0, len(fileNames))
		for _, fileName := range fileNames {
			parts = append(parts, "《"+fileName+"》")
		}
		return strings.Join(parts, "；")
	}
}

func workbookDisplayName(filePath string) string {
	name := strings.TrimSpace(filepath.Base(strings.TrimSpace(filePath)))
	name = strings.ReplaceAll(name, " - ", "-")
	name = strings.ReplaceAll(name, " – ", "-")
	return name
}

func formatWorkbookSheetDisplay(fileName string, sheets []string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		fileName = "来源Excel"
	}
	formattedSheets := make([]string, 0, len(sheets))
	for _, sheet := range sheets {
		sheet = strings.TrimSpace(sheet)
		if sheet == "" {
			continue
		}
		formattedSheets = append(formattedSheets, "【"+sheet+"】")
	}
	if len(formattedSheets) == 0 {
		return "《" + fileName + "》"
	}
	return "《" + fileName + "》的" + strings.Join(formattedSheets, "和")
}
