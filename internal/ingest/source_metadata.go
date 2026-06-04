package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	dbschema "financeqa/internal/db"
	"financeqa/internal/parser"
)

func annotateImportedReportSource(ctx context.Context, dbPath string, metadata parser.FileMetadata, filePath string, opts ImportOptions) error {
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

	reportType := strings.TrimSpace(metadata.ReportType)
	tableName, err := resolvePhysicalTableName(ctx, tx, reportType)
	if err != nil {
		return err
	}
	meta := dbschema.BuildImportedTableSourceMetadata(tableName, filePath, []string{reportType}, nil, "")
	if err := upsertImportedSourceMetadata(ctx, tx, dbPath, tableName, meta, "imported", true); err != nil {
		return err
	}
	if err := upsertImportedReportFileMappings(ctx, tx, dbPath, metadata, filePath, opts); err != nil {
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

type financeSourceFileIdentity struct {
	FileName        string
	StorageKey      string
	FileHash        string
	SourceVersionID string
	FileSize        int64
}

func buildFinanceSourceFileIdentity(filePath string, opts ImportOptions) financeSourceFileIdentity {
	fileName := strings.TrimSpace(opts.SourceFileName)
	if fileName == "" {
		fileName = workbookDisplayName(filePath)
	}
	storageKey := strings.TrimSpace(opts.SourceStorageKey)
	if storageKey == "" {
		storageKey = strings.TrimSpace(filePath)
	}
	fileSize := opts.SourceFileSize
	if fileSize <= 0 {
		if info, err := os.Stat(filePath); err == nil {
			fileSize = info.Size()
		}
	}
	fileHash := sourceFileHashPrefix(filePath, 12)
	versionID := ""
	if fileName != "" && fileHash != "" {
		versionID = fileName + ":" + fileHash
	}
	return financeSourceFileIdentity{
		FileName:        fileName,
		StorageKey:      storageKey,
		FileHash:        fileHash,
		SourceVersionID: versionID,
		FileSize:        fileSize,
	}
}

func sourceFileHashPrefix(filePath string, length int) string {
	if length <= 0 {
		length = 12
	}
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if len(sum) < length {
		return sum
	}
	return sum[:length]
}

func upsertImportedReportFileMappings(ctx context.Context, tx *sql.Tx, dbPath string, metadata parser.FileMetadata, filePath string, opts ImportOptions) error {
	tableType := importedReportMappingTableType(metadata.ReportType)
	if tableType == "" {
		return nil
	}
	if err := ensureFinanceFileMappingsTable(ctx, tx, dbPath); err != nil {
		return err
	}
	identity := buildFinanceSourceFileIdentity(filePath, opts)
	if identity.FileName == "" || identity.StorageKey == "" {
		return nil
	}
	company := strings.TrimSpace(opts.CompanyOverride)
	if company == "" {
		company = strings.TrimSpace(metadata.Company)
	}
	if company == "" {
		company = "DefaultCompany"
	}
	periods := importedReportMappingPeriods(metadata.PeriodStart, metadata.PeriodEnd)
	if len(periods) == 0 {
		periods = []string{""}
	}
	if !opts.Incremental {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM fin_file_mappings
WHERE LOWER(COALESCE(table_type, '')) = ?
  AND (
    COALESCE(company, '') = ''
    OR ? = ''
    OR ? LIKE '%' || COALESCE(company, '') || '%'
    OR COALESCE(company, '') LIKE '%' || ? || '%'
  )
`, strings.ToLower(tableType), company, company, company); err != nil {
			return fmt.Errorf("delete stale imported file mappings for %s: %w", tableType, err)
		}
	}
	for _, period := range periods {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM fin_file_mappings
WHERE LOWER(COALESCE(table_type, '')) = ?
  AND COALESCE(period, '') = ?
  AND (
    COALESCE(source_version_id, '') = ?
    OR COALESCE(storage_key, '') = ?
    OR (COALESCE(source_version_id, '') = '' AND COALESCE(file_name, '') = ?)
  )
`, strings.ToLower(tableType), period, identity.SourceVersionID, identity.StorageKey, identity.FileName); err != nil {
			return fmt.Errorf("dedupe imported file mapping %s/%s: %w", tableType, period, err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO fin_file_mappings(table_type, period, company, storage_key, file_name, description, file_size, source_file_hash, source_version_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, tableType, period, company, identity.StorageKey, identity.FileName, importedReportFileMappingDescription(tableType, period), nullableFileSize(identity.FileSize), identity.FileHash, identity.SourceVersionID); err != nil {
			return fmt.Errorf("insert imported file mapping %s/%s: %w", tableType, period, err)
		}
	}
	return nil
}

func importedReportMappingTableType(reportType string) string {
	switch strings.TrimSpace(reportType) {
	case "bank_statement":
		return "bank-statement"
	case "journal":
		return "journal"
	case "income_statement":
		return "income-statement"
	case "balance_sheet":
		return "balance-sheet"
	case "balance_detail":
		return "balance-detail"
	default:
		return ""
	}
}

func importedReportFileMappingDescription(tableType, period string) string {
	if strings.TrimSpace(period) == "" {
		return strings.TrimSpace(tableType) + " 财务文件导入"
	}
	return strings.TrimSpace(period) + " " + strings.TrimSpace(tableType) + " 财务文件导入"
}

func importedReportMappingPeriods(periodStart, periodEnd string) []string {
	from := strings.TrimSpace(periodStart)
	to := strings.TrimSpace(periodEnd)
	if to == "" {
		to = from
	}
	if from == "" || to == "" {
		return nil
	}
	fromYear, fromMonth := parseYearMonthForMapping(from)
	toYear, toMonth := parseYearMonthForMapping(to)
	if fromYear == 0 || fromMonth == 0 || toYear == 0 || toMonth == 0 {
		return []string{from, to}
	}
	quarters := map[string]struct{}{}
	for y, m := fromYear, fromMonth; y < toYear || (y == toYear && m <= toMonth); {
		quarters[fmt.Sprintf("%04d-Q%d", y, ((m-1)/3)+1)] = struct{}{}
		m++
		if m > 12 {
			y++
			m = 1
		}
		if len(quarters) > 40 {
			break
		}
	}
	out := make([]string, 0, len(quarters))
	for quarter := range quarters {
		out = append(out, quarter)
	}
	sort.Strings(out)
	return out
}

func parseYearMonthForMapping(value string) (int, int) {
	value = strings.TrimSpace(value)
	if len(value) != len("2006-01") || value[4] != '-' {
		return 0, 0
	}
	var year, month int
	if _, err := fmt.Sscanf(value, "%04d-%02d", &year, &month); err != nil {
		return 0, 0
	}
	if month < 1 || month > 12 {
		return 0, 0
	}
	return year, month
}

func upsertContractWorkbookFileMappings(ctx context.Context, tx *sql.Tx, dbPath, filePath string, bundle contractImportBundle, opts ImportOptions) error {
	mappings := contractWorkbookFileMappings(bundle)
	if len(mappings) == 0 {
		return nil
	}
	if err := ensureFinanceFileMappingsTable(ctx, tx, dbPath); err != nil {
		return err
	}
	identity := buildFinanceSourceFileIdentity(filePath, opts)
	if identity.FileName == "" || identity.StorageKey == "" {
		return nil
	}
	company := strings.TrimSpace(opts.CompanyOverride)
	if company == "" {
		company = "DefaultCompany"
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
`, strings.ToLower(tableType), company, company, company, identity.FileName, identity.StorageKey); err != nil {
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
    COALESCE(source_version_id, '') = ?
    OR COALESCE(storage_key, '') = ?
    OR (COALESCE(source_version_id, '') = '' AND COALESCE(file_name, '') = ?)
  )
`, strings.ToLower(mapping.TableType), mapping.Period, identity.SourceVersionID, identity.StorageKey, identity.FileName); err != nil {
			return fmt.Errorf("dedupe finance file mapping %s/%s: %w", mapping.TableType, mapping.Period, err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO fin_file_mappings(table_type, period, company, storage_key, file_name, description, file_size, source_file_hash, source_version_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`, mapping.TableType, mapping.Period, company, identity.StorageKey, identity.FileName, mapping.Description, nullableFileSize(identity.FileSize), identity.FileHash, identity.SourceVersionID); err != nil {
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
	source_file_hash VARCHAR(64),
	source_version_id VARCHAR(512),
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
	source_file_hash TEXT,
	source_version_id TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)`
	}
	if _, err := tx.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("ensure fin_file_mappings table: %w", err)
	}
	if err := ensureFinanceFileMappingsColumn(ctx, tx, dbPath, "source_file_hash", "TEXT"); err != nil {
		return err
	}
	if err := ensureFinanceFileMappingsColumn(ctx, tx, dbPath, "source_version_id", "TEXT"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_fin_files_table_type ON fin_file_mappings(table_type)`); err != nil {
		return fmt.Errorf("ensure fin_file_mappings table_type index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_fin_files_period ON fin_file_mappings(period)`); err != nil {
		return fmt.Errorf("ensure fin_file_mappings period index: %w", err)
	}
	return nil
}

func ensureFinanceFileMappingsColumn(ctx context.Context, tx *sql.Tx, dbPath, columnName, columnType string) error {
	columnName = strings.TrimSpace(columnName)
	columnType = strings.TrimSpace(columnType)
	if columnName == "" || columnType == "" {
		return nil
	}
	if looksLikeSQLiteImportPath(dbPath) {
		rows, err := tx.QueryContext(ctx, `PRAGMA table_info(fin_file_mappings)`)
		if err != nil {
			return fmt.Errorf("inspect fin_file_mappings columns: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, typ string
			var notNull int
			var defaultValue any
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
				return fmt.Errorf("scan fin_file_mappings column: %w", err)
			}
			if name == columnName {
				return nil
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE fin_file_mappings ADD COLUMN %s %s`, columnName, columnType)); err != nil {
			return fmt.Errorf("add fin_file_mappings.%s: %w", columnName, err)
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE fin_file_mappings ADD COLUMN IF NOT EXISTS %s %s`, columnName, columnType)); err != nil {
		return fmt.Errorf("add fin_file_mappings.%s: %w", columnName, err)
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
