package query

import (
	"database/sql"
	"fmt"
	"strings"
)

func (e *Engine) collectSourcePartitions(spec QuerySpec, data map[string]any, tables []string) []map[string]any {
	if data == nil || len(tables) == 0 {
		return nil
	}

	switch spec.QueryFamily {
	case QueryFamilyContractDimension:
		return e.collectContractSourcePartitions(data, tables)
	case QueryFamilyCoreMetric:
		if strings.TrimSpace(anyToString(data["source_priority"])) == "contract_first" {
			return e.collectAggregateContractSourcePartitions(spec, data, tables)
		}
	}
	return nil
}

func (e *Engine) collectContractSourcePartitions(data map[string]any, tables []string) []map[string]any {
	periodFrom := strings.TrimSpace(anyToString(data["period_from"]))
	periodTo := strings.TrimSpace(anyToString(data["period_to"]))
	contractIDs := contractIDsFromData(data["contracts"])
	if periodFrom == "" || periodTo == "" || len(contractIDs) == 0 {
		return nil
	}
	return e.querySourcePartitionsForTables(tables, periodFrom, periodTo, contractIDs)
}

func (e *Engine) collectAggregateContractSourcePartitions(spec QuerySpec, data map[string]any, tables []string) []map[string]any {
	periodFrom := strings.TrimSpace(spec.PeriodFrom)
	periodTo := strings.TrimSpace(spec.PeriodTo)
	if periodFrom == "" || periodTo == "" {
		periodFrom = strings.TrimSpace(anyToString(data["period_from"]))
		periodTo = strings.TrimSpace(anyToString(data["period_to"]))
	}
	if periodFrom == "" || periodTo == "" {
		return nil
	}
	return e.querySourcePartitionsForTables(tables, periodFrom, periodTo, nil)
}

func (e *Engine) querySourcePartitionsForTables(tables []string, periodFrom, periodTo string, contractIDs []string) []map[string]any {
	out := make([]map[string]any, 0, 4)
	seen := map[string]struct{}{}

	for _, tableName := range dedupeSourceTables(tables...) {
		baseTable := strings.TrimSpace(baseSourceTableName(tableName))
		if !e.tableColumns(baseTable)["source_report_type"] || !e.tableColumns(baseTable)["source_sheet_name"] {
			continue
		}

		rows, err := e.querySourcePartitionRows(baseTable, periodFrom, periodTo, contractIDs)
		if err != nil {
			continue
		}

		for _, row := range rows {
			key := strings.Join([]string{
				strings.TrimSpace(tableName),
				strings.TrimSpace(anyToString(row["source_report_type"])),
				strings.TrimSpace(anyToString(row["source_sheet_name"])),
			}, "|")
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			row["table"] = tableName
			out = append(out, row)
		}
	}

	return out
}

func (e *Engine) querySourcePartitionRows(tableName, periodFrom, periodTo string, contractIDs []string) ([]map[string]any, error) {
	args := make([]any, 0, len(contractIDs)+2)
	args = append(args, periodFrom, periodTo)
	sqlText := fmt.Sprintf(`
SELECT DISTINCT source_report_type, source_sheet_name
FROM %s
WHERE year_month BETWEEN ? AND ?
  AND COALESCE(source_report_type, '') <> ''
  AND COALESCE(source_sheet_name, '') <> ''
`, tableName)
	if len(contractIDs) > 0 {
		placeholders := make([]string, 0, len(contractIDs))
		for _, contractID := range contractIDs {
			placeholders = append(placeholders, "?")
			args = append(args, contractID)
		}
		sqlText += "\n  AND contract_id IN (" + strings.Join(placeholders, ", ") + ")\n"
	}
	sqlText += "ORDER BY source_report_type, source_sheet_name"

	rows, err := e.db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0, 4)
	for rows.Next() {
		var reportType, sheetName sql.NullString
		if err := rows.Scan(&reportType, &sheetName); err != nil {
			return nil, err
		}
		if strings.TrimSpace(reportType.String) == "" || strings.TrimSpace(sheetName.String) == "" {
			continue
		}
		out = append(out, map[string]any{
			"source_report_type": strings.TrimSpace(reportType.String),
			"source_sheet_name":  strings.TrimSpace(sheetName.String),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func contractIDsFromData(v any) []string {
	rows := anyToMapSlice(v)
	out := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		contractID := strings.TrimSpace(anyToString(row["contract_id"]))
		if contractID == "" {
			continue
		}
		if _, ok := seen[contractID]; ok {
			continue
		}
		seen[contractID] = struct{}{}
		out = append(out, contractID)
	}
	return out
}

func filterSourcePartitionsByTable(partitions []map[string]any, tables []string) []map[string]any {
	if len(partitions) == 0 || len(tables) == 0 {
		return nil
	}
	tableSet := map[string]struct{}{}
	baseSet := map[string]struct{}{}
	for _, tableName := range tables {
		trimmed := strings.TrimSpace(tableName)
		if trimmed == "" {
			continue
		}
		tableSet[trimmed] = struct{}{}
		baseSet[baseSourceTableName(trimmed)] = struct{}{}
	}

	out := make([]map[string]any, 0, len(partitions))
	for _, partition := range partitions {
		tableName := strings.TrimSpace(anyToString(partition["table"]))
		if _, ok := tableSet[tableName]; ok {
			out = append(out, cloneMap(partition))
			continue
		}
		if _, ok := baseSet[baseSourceTableName(tableName)]; ok {
			out = append(out, cloneMap(partition))
		}
	}
	return out
}
