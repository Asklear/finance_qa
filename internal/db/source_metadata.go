package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

const tableSourceCommentPrefix = "financeqa_source: "

var (
	legacyWorkbookPattern = regexp.MustCompile(`[^《》【】；，。:：\s]+(?:[^《》【】；，。:：]*?)\.(?:xlsx|xls|csv|tsv)`)
	legacySheetPattern    = regexp.MustCompile(`【([^】]+)】`)
	feishuTokenPattern    = regexp.MustCompile(`^[A-Za-z0-9]{20,}$`)
)

type TableSourceMetadata struct {
	Version      string   `json:"version,omitempty"`
	Display      string   `json:"display,omitempty"`
	LogicalLabel string   `json:"logical_label,omitempty"`
	FileNames    []string `json:"file_names,omitempty"`
	SheetNames   []string `json:"sheet_names,omitempty"`
	ReportTypes  []string `json:"report_types,omitempty"`
	Notes        []string `json:"notes,omitempty"`
	Description  string   `json:"description,omitempty"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
}

type SourceMetadataExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func DefaultTableSourceMetadata(tableName string) TableSourceMetadata {
	base := baseTableName(tableName)
	description := defaultTableDescription(base)
	switch base {
	case "fin_journal", "journal":
		return TableSourceMetadata{Version: "v1", Display: "《序时帐》", LogicalLabel: "journal", ReportTypes: []string{"journal"}, Description: description}
	case "fin_bank_statement", "bank_statement":
		return TableSourceMetadata{Version: "v1", Display: "《银行流水》", LogicalLabel: "bank_statement", ReportTypes: []string{"bank_statement"}, Description: description}
	case "fin_income_statement", "income_statement":
		return TableSourceMetadata{Version: "v1", Display: "《利润表》", LogicalLabel: "income_statement", ReportTypes: []string{"income_statement"}, Description: description}
	case "fin_balance_sheet", "balance_sheet":
		return TableSourceMetadata{Version: "v1", Display: "《资产负债表》", LogicalLabel: "balance_sheet", ReportTypes: []string{"balance_sheet"}, Description: description}
	case "fin_balance_detail", "balance_detail":
		return TableSourceMetadata{Version: "v1", Display: "《科目余额表》", LogicalLabel: "balance_detail", ReportTypes: []string{"balance_detail"}, Description: description}
	case "fin_contracts":
		return TableSourceMetadata{Version: "v1", Display: "《合同信息表》", LogicalLabel: "fin_contracts", ReportTypes: []string{"contract_dimension"}, Description: description}
	case "fin_fund_income":
		return TableSourceMetadata{Version: "v1", Display: "《合同资金收入表》", LogicalLabel: "fin_fund_income", ReportTypes: []string{"contract_fund_income", "contract_revenue_cost"}, Description: description}
	case "fin_cost_settlements":
		return TableSourceMetadata{Version: "v1", Display: "《合同成本结算表》", LogicalLabel: "fin_cost_settlements", ReportTypes: []string{"contract_revenue_cost"}, Description: description}
	case "fin_revenue_settlements":
		return TableSourceMetadata{Version: "v1", Display: "《已废弃合同收入表》", LogicalLabel: "fin_revenue_settlements", ReportTypes: []string{"deprecated"}, Notes: []string{"DEPRECATED: 暂停使用，合同收入统一以 fin_fund_income 为准；代码已停止读取该表"}, Description: description}
	default:
		return TableSourceMetadata{Version: "v1", Description: description}
	}
}

func BuildImportedTableSourceMetadata(tableName, filePath string, reportTypes, sheetNames []string, displayOverride string) TableSourceMetadata {
	meta := DefaultTableSourceMetadata(tableName)
	fileName := normalizeWorkbookDisplayName(lastPathSegment(filePath))
	if strings.TrimSpace(displayOverride) != "" {
		meta.Display = strings.TrimSpace(displayOverride)
	}
	if fileName != "" {
		meta.FileNames = appendDedup(meta.FileNames, fileName)
	}
	meta.SheetNames = appendDedup(meta.SheetNames, normalizeSheetNames(sheetNames)...)
	meta.ReportTypes = appendDedup(meta.ReportTypes, trimNonEmpty(reportTypes)...)
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return meta
}

func EnsureDefaultTableSourceMetadata(ctx context.Context, runner SourceMetadataExecutor, dbPath string, tableNames []string) error {
	for _, tableName := range tableNames {
		base := baseTableName(tableName)
		if base == "" {
			continue
		}
		existing, ok, err := loadSingleTableSourceMetadata(ctx, runner, dbPath, base)
		if err != nil {
			return err
		}
		if ok && strings.TrimSpace(existing.Display) != "" {
			continue
		}
		if err := UpsertTableSourceMetadata(ctx, runner, dbPath, base, DefaultTableSourceMetadata(base)); err != nil {
			return err
		}
	}
	return nil
}

func EnsureStructuredTableSourceMetadata(ctx context.Context, runner SourceMetadataExecutor, dbPath string, tableNames []string) error {
	qualified := map[string]string{}
	rawComments := map[string]string{}
	current := map[string]TableSourceMetadata{}
	rawParsed := map[string]TableSourceMetadata{}
	structured := map[string]bool{}

	for _, tableName := range tableNames {
		base := baseTableName(tableName)
		if base == "" {
			continue
		}
		if _, ok := qualified[base]; !ok {
			qualified[base] = tableName
		}
		raw, ok, err := loadRawTableComment(ctx, runner, dbPath, tableName)
		if err != nil {
			return err
		}
		if !ok {
			current[base] = DefaultTableSourceMetadata(base)
			continue
		}
		rawComments[base] = raw
		structured[base] = strings.HasPrefix(strings.TrimSpace(raw), tableSourceCommentPrefix)
		meta, _, err := loadSingleTableSourceMetadataDetailed(ctx, runner, dbPath, tableName)
		if err != nil {
			return err
		}
		rawParsed[base] = meta
		current[base] = mergeSourceMetadata(DefaultTableSourceMetadata(base), meta)
	}

	normalized := normalizeTableSourceMetadataSet(rawComments, current)
	for base, tableName := range qualified {
		target := normalized[base]
		if strings.TrimSpace(target.Display) == "" {
			continue
		}
		existingRaw := rawParsed[base]
		if !structured[base] || !sourceMetadataSemanticallyEqual(existingRaw, target) {
			if err := UpsertTableSourceMetadata(ctx, runner, dbPath, tableName, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func UpsertTableSourceMetadata(ctx context.Context, runner SourceMetadataExecutor, dbPath, tableName string, meta TableSourceMetadata) error {
	base := baseTableName(tableName)
	if base == "" {
		return nil
	}
	if strings.TrimSpace(meta.Version) == "" {
		meta.Version = "v1"
	}
	if strings.TrimSpace(meta.UpdatedAt) == "" {
		meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	comment, err := formatTableSourceComment(meta)
	if err != nil {
		return err
	}
	return UpsertTableComment(ctx, runner, dbPath, tableName, comment)
}

func LoadTableSourceMetadata(ctx context.Context, runner SourceMetadataExecutor, dbPath string, tableNames []string) (map[string]TableSourceMetadata, error) {
	out := make(map[string]TableSourceMetadata, len(tableNames))
	for _, tableName := range tableNames {
		base := baseTableName(tableName)
		if base == "" {
			continue
		}
		meta, ok, err := loadSingleTableSourceMetadata(ctx, runner, dbPath, tableName)
		if err != nil {
			return nil, err
		}
		if ok {
			out[tableName] = mergeSourceMetadata(DefaultTableSourceMetadata(base), meta)
			continue
		}
		out[tableName] = DefaultTableSourceMetadata(base)
	}
	return out, nil
}

func loadSingleTableSourceMetadata(ctx context.Context, runner SourceMetadataExecutor, dbPath, tableName string) (TableSourceMetadata, bool, error) {
	meta, ok, err := loadSingleTableSourceMetadataDetailed(ctx, runner, dbPath, tableName)
	if err != nil {
		return TableSourceMetadata{}, false, err
	}
	return meta, ok, nil
}

func loadSingleTableSourceMetadataDetailed(ctx context.Context, runner SourceMetadataExecutor, dbPath, tableName string) (TableSourceMetadata, bool, error) {
	base := baseTableName(tableName)
	if base == "" {
		return TableSourceMetadata{}, false, nil
	}
	raw, ok, err := loadRawTableComment(ctx, runner, dbPath, tableName)
	if err != nil {
		return TableSourceMetadata{}, false, err
	}
	if !ok {
		return TableSourceMetadata{}, false, nil
	}
	meta, ok := parseTableSourceComment(raw)
	return meta, ok, nil
}

func loadRawTableComment(ctx context.Context, runner SourceMetadataExecutor, dbPath, tableName string) (string, bool, error) {
	base := baseTableName(tableName)
	if base == "" {
		return "", false, nil
	}
	var raw sql.NullString
	if looksLikeSQLitePath(strings.TrimSpace(dbPath)) {
		err := runner.QueryRowContext(ctx, `SELECT comment FROM meta_table_comments WHERE table_name = ?`, base).Scan(&raw)
		if err != nil {
			if err == sql.ErrNoRows || strings.Contains(strings.ToLower(err.Error()), "no such table") {
				return "", false, nil
			}
			return "", false, fmt.Errorf("load sqlite table comment %s: %w", base, err)
		}
	} else {
		schema, rel := splitQualifiedTableName(tableName)
		if schema == "" {
			schema = effectiveSchemaWithRunner(ctx, runner, dbPath)
		}
		err := runner.QueryRowContext(ctx, `
SELECT COALESCE(pgd.description, '')
FROM pg_catalog.pg_statio_all_tables st
LEFT JOIN pg_catalog.pg_description pgd ON pgd.objoid = st.relid AND pgd.objsubid = 0
WHERE st.schemaname = ? AND st.relname = ?
LIMIT 1
`, schema, rel).Scan(&raw)
		if err != nil {
			if err == sql.ErrNoRows {
				return "", false, nil
			}
			return "", false, fmt.Errorf("load pg table comment %s.%s: %w", schema, rel, err)
		}
	}
	return strings.TrimSpace(raw.String), strings.TrimSpace(raw.String) != "", nil
}

func formatTableSourceComment(meta TableSourceMetadata) (string, error) {
	if strings.TrimSpace(meta.Version) == "" {
		meta.Version = "v1"
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshal table source metadata: %w", err)
	}
	return tableSourceCommentPrefix + string(payload), nil
}

func parseTableSourceComment(comment string) (TableSourceMetadata, bool) {
	raw := strings.TrimSpace(comment)
	if raw == "" {
		return TableSourceMetadata{}, false
	}
	if !strings.HasPrefix(raw, tableSourceCommentPrefix) {
		return TableSourceMetadata{Version: "v1", Display: raw}, true
	}
	raw = strings.TrimSpace(strings.TrimPrefix(raw, tableSourceCommentPrefix))
	if raw == "" {
		return TableSourceMetadata{}, false
	}
	var meta TableSourceMetadata
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return TableSourceMetadata{Version: "v1", Display: raw}, true
	}
	if strings.TrimSpace(meta.Version) == "" {
		meta.Version = "v1"
	}
	return meta, true
}

func mergeSourceMetadata(base, override TableSourceMetadata) TableSourceMetadata {
	out := base
	if strings.TrimSpace(override.Version) != "" {
		out.Version = override.Version
	}
	if strings.TrimSpace(override.Display) != "" {
		out.Display = override.Display
	}
	if strings.TrimSpace(override.LogicalLabel) != "" {
		out.LogicalLabel = override.LogicalLabel
	}
	if strings.TrimSpace(override.Description) != "" {
		out.Description = override.Description
	}
	out.FileNames = appendDedup(out.FileNames, override.FileNames...)
	out.SheetNames = appendDedup(out.SheetNames, override.SheetNames...)
	out.ReportTypes = appendDedup(out.ReportTypes, override.ReportTypes...)
	out.Notes = appendDedup(out.Notes, override.Notes...)
	if strings.TrimSpace(override.UpdatedAt) != "" {
		out.UpdatedAt = override.UpdatedAt
	}
	return out
}

func effectiveSchemaWithRunner(ctx context.Context, runner SourceMetadataExecutor, dsn string) string {
	if schema := strings.TrimSpace(schemaFromDSN(dsn)); schema != "" {
		return schema
	}
	var schema string
	if err := runner.QueryRowContext(ctx, `SELECT CURRENT_SCHEMA()`).Scan(&schema); err == nil {
		schema = strings.TrimSpace(schema)
		if schema != "" {
			return schema
		}
	}
	if schema := strings.TrimSpace(defaultSchemaFromEnv()); schema != "" {
		return schema
	}
	return "public"
}

func defaultSchemaFromEnv() string {
	return strings.TrimSpace(os.Getenv("FINANCEQA_PG_SCHEMA"))
}

func splitQualifiedTableName(tableName string) (string, string) {
	parts := strings.Split(strings.TrimSpace(tableName), ".")
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", strings.TrimSpace(tableName)
}

func baseTableName(tableName string) string {
	_, rel := splitQualifiedTableName(tableName)
	return strings.TrimSpace(rel)
}

func escapeIdentifier(v string) string {
	return strings.ReplaceAll(strings.TrimSpace(v), `"`, `""`)
}

func appendDedup(base []string, values ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(base)+len(values))
	for _, item := range append(base, values...) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func trimNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func normalizeSheetNames(sheetNames []string) []string {
	cleaned := trimNonEmpty(sheetNames)
	sort.Strings(cleaned)
	return cleaned
}

func lastPathSegment(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, `\`, `/`)
	parts := strings.Split(path, "/")
	return strings.TrimSpace(parts[len(parts)-1])
}

func normalizeWorkbookDisplayName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, " - ", "-")
	name = strings.ReplaceAll(name, " – ", "-")
	return name
}

func formatPostgresTableCommentSQL(schema, tableName, comment string) string {
	return fmt.Sprintf(
		`COMMENT ON TABLE "%s"."%s" IS '%s'`,
		escapeIdentifier(schema),
		escapeIdentifier(tableName),
		strings.ReplaceAll(comment, `'`, `''`),
	)
}

func normalizeTableSourceMetadataSet(rawComments map[string]string, current map[string]TableSourceMetadata) map[string]TableSourceMetadata {
	out := make(map[string]TableSourceMetadata, len(current))
	for base, meta := range current {
		out[base] = normalizeSingleTableSourceMetadata(base, rawComments[base], meta)
	}

	contracts := out["fin_contracts"]
	contracts.FileNames = appendDedup(contracts.FileNames, out["fin_fund_income"].FileNames...)
	contracts.FileNames = appendDedup(contracts.FileNames, out["fin_cost_settlements"].FileNames...)
	contracts.SheetNames = appendDedup(contracts.SheetNames, out["fin_fund_income"].SheetNames...)
	contracts.SheetNames = appendDedup(contracts.SheetNames, out["fin_cost_settlements"].SheetNames...)
	contracts.Display = normalizedDisplayForTable("fin_contracts", contracts)
	out["fin_contracts"] = contracts

	return out
}

func normalizeSingleTableSourceMetadata(tableName, raw string, meta TableSourceMetadata) TableSourceMetadata {
	base := baseTableName(tableName)
	out := mergeSourceMetadata(DefaultTableSourceMetadata(base), meta)
	inferenceSource := strings.TrimSpace(raw)
	if strings.HasPrefix(inferenceSource, tableSourceCommentPrefix) {
		inferenceSource = out.Display
	}
	out.FileNames = appendDedup(out.FileNames, inferWorkbookNamesFromComment(inferenceSource)...)
	out.FileNames = filterSourceWorkbookNames(out.FileNames)
	out.SheetNames = appendDedup(out.SheetNames, inferSheetNamesFromComment(inferenceSource)...)
	out.ReportTypes = appendDedup(out.ReportTypes, DefaultTableSourceMetadata(base).ReportTypes...)
	if strings.TrimSpace(out.LogicalLabel) == "" {
		out.LogicalLabel = DefaultTableSourceMetadata(base).LogicalLabel
	}
	if strings.TrimSpace(out.Description) == "" {
		out.Description = DefaultTableSourceMetadata(base).Description
	}
	out.Display = normalizedDisplayForTable(base, out)
	return out
}

func filterSourceWorkbookNames(fileNames []string) []string {
	out := make([]string, 0, len(fileNames))
	for _, fileName := range appendDedup(nil, fileNames...) {
		if isFeishuTokenOnlyWorkbookName(fileName) {
			continue
		}
		out = append(out, fileName)
	}
	return out
}

func isFeishuTokenOnlyWorkbookName(fileName string) bool {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return false
	}
	ext := strings.ToLower(filepathExt(fileName))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" && ext != ".tsv" {
		return false
	}
	base := strings.TrimSuffix(fileName, filepathExt(fileName))
	return feishuTokenPattern.MatchString(base)
}

func normalizedDisplayForTable(tableName string, meta TableSourceMetadata) string {
	base := baseTableName(tableName)
	defaultMeta := DefaultTableSourceMetadata(base)
	switch base {
	case "fin_fund_income", "fin_cost_settlements":
		if len(meta.FileNames) == 1 {
			if len(meta.SheetNames) > 0 {
				return formatWorkbookSheetDisplayLocal(meta.FileNames[0], meta.SheetNames)
			}
			return "《" + meta.FileNames[0] + "》"
		}
		if len(meta.FileNames) > 1 {
			return formatWorkbookListDisplay(meta.FileNames)
		}
		return defaultMeta.Display
	case "fin_contracts":
		return defaultMeta.Display
	default:
		return defaultMeta.Display
	}
}

func inferWorkbookNamesFromComment(comment string) []string {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return nil
	}
	candidates := []string{comment}
	if idx := strings.Index(comment, "来自"); idx >= 0 {
		candidates = append([]string{comment[idx+len("来自"):]}, candidates...)
	}
	files := make([]string, 0, 2)
	for _, candidate := range candidates {
		matches := legacyWorkbookPattern.FindAllString(candidate, -1)
		for _, match := range matches {
			match = strings.TrimSpace(match)
			match = strings.TrimPrefix(match, "来自")
			match = strings.TrimPrefix(match, "整合")
			match = strings.Trim(match, `"'()（）《》 `)
			match = normalizeWorkbookDisplayName(match)
			if match == "" {
				continue
			}
			files = append(files, match)
		}
	}
	return appendDedup(nil, files...)
}

func filepathExt(name string) string {
	idx := strings.LastIndex(name, ".")
	if idx < 0 {
		return ""
	}
	return name[idx:]
}

func inferSheetNamesFromComment(comment string) []string {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return nil
	}
	matches := legacySheetPattern.FindAllStringSubmatch(comment, -1)
	sheets := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		sheets = append(sheets, strings.TrimSpace(match[1]))
	}
	return normalizeSheetNames(sheets)
}

func formatWorkbookListDisplay(fileNames []string) string {
	parts := make([]string, 0, len(fileNames))
	for _, fileName := range appendDedup(nil, fileNames...) {
		parts = append(parts, "《"+fileName+"》")
	}
	return strings.Join(parts, "；")
}

func formatWorkbookSheetDisplayLocal(fileName string, sheets []string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		fileName = "来源Excel"
	}
	formattedSheets := make([]string, 0, len(sheets))
	for _, sheet := range normalizeSheetNames(sheets) {
		formattedSheets = append(formattedSheets, "【"+sheet+"】")
	}
	if len(formattedSheets) == 0 {
		return "《" + fileName + "》"
	}
	return "《" + fileName + "》的" + strings.Join(formattedSheets, "和")
}

func sourceMetadataSemanticallyEqual(left, right TableSourceMetadata) bool {
	left = normalizeSourceMetadataForCompare(left)
	right = normalizeSourceMetadataForCompare(right)
	return reflect.DeepEqual(left, right)
}

func normalizeSourceMetadataForCompare(meta TableSourceMetadata) TableSourceMetadata {
	meta.UpdatedAt = ""
	meta.FileNames = append([]string{}, appendDedup(nil, meta.FileNames...)...)
	meta.SheetNames = append([]string{}, normalizeSheetNames(meta.SheetNames)...)
	meta.ReportTypes = append([]string{}, appendDedup(nil, meta.ReportTypes...)...)
	meta.Notes = append([]string{}, appendDedup(nil, meta.Notes...)...)
	sort.Strings(meta.FileNames)
	sort.Strings(meta.ReportTypes)
	sort.Strings(meta.Notes)
	return meta
}
