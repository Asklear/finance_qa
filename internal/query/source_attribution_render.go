package query

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	dbpkg "financeqa/internal/db"
)

func (e *Engine) annotateSourceAttribution(spec QuerySpec, result Result) Result {
	if !result.Success {
		return result
	}
	if result.Data == nil {
		result.Data = map[string]any{}
	}

	plan := resolveSourceAttributionPlan(spec, result.Data)
	collected := plan.tables
	if len(collected) == 0 {
		return result
	}

	metadata, err := dbpkg.LoadTableSourceMetadata(context.Background(), e.db, e.dbPath, collected)
	if err != nil {
		metadata = make(map[string]dbpkg.TableSourceMetadata, len(collected))
		for _, tableName := range collected {
			metadata[tableName] = dbpkg.DefaultTableSourceMetadata(tableName)
		}
	}

	primaryTables, supportingTables := choosePrimaryAndSupportingTables(spec, result.Data, collected)
	primaryDocs := sourceDisplaysForTables(primaryTables, metadata)
	supportingDocs := sourceDisplaysForTables(supportingTables, metadata)
	sourceNote := buildSourceNote(primaryDocs, supportingDocs)
	if sourceNote == "" {
		return result
	}
	sourceUpdateNote := e.buildSourceUpdateNote(collected, metadata)

	result.Data["source_tables"] = collected
	result.Data["primary_source_tables"] = primaryTables
	result.Data["source_documents"] = primaryDocs
	if len(supportingDocs) > 0 {
		result.Data["supporting_source_documents"] = supportingDocs
	}
	if partitions := e.collectSourcePartitions(spec, result.Data, collected); len(partitions) > 0 {
		primaryPartitions := filterSourcePartitionsByTable(partitions, primaryTables)
		supportingPartitions := filterSourcePartitionsByTable(partitions, supportingTables)
		if len(primaryPartitions) > 0 {
			result.Data["source_partitions"] = primaryPartitions
			result.Data["primary_source_partitions"] = primaryPartitions
		} else {
			result.Data["source_partitions"] = partitions
		}
		if len(supportingPartitions) > 0 {
			result.Data["supporting_source_partitions"] = supportingPartitions
		}
	}
	result.Data["source_note"] = sourceNote
	result.Data["source_summary"] = sourceNote
	if sourceUpdateNote != "" {
		result.Data["source_update_note"] = sourceUpdateNote
	}

	if !strings.Contains(result.Message, "来源：") {
		result.Message = strings.TrimSpace(result.Message) + "\n" + sourceNote
	}
	if sourceUpdateNote != "" && !strings.Contains(result.Message, "来源更新时间：") {
		result.Message = strings.TrimSpace(result.Message) + "\n" + sourceUpdateNote
	}
	return result
}

func buildSourceNote(primaryDocs, supportingDocs []string) string {
	if len(primaryDocs) == 0 && len(supportingDocs) == 0 {
		return ""
	}
	if len(primaryDocs) == 0 {
		return "来源：" + strings.Join(supportingDocs, "；")
	}
	note := "来源：" + strings.Join(primaryDocs, "；")
	if len(supportingDocs) > 0 {
		note += "；补充参考：" + strings.Join(supportingDocs, "；")
	}
	return note
}

func sourceDisplaysForTables(tables []string, metadata map[string]dbpkg.TableSourceMetadata) []string {
	out := make([]string, 0, len(tables))
	for _, tableName := range tables {
		if shouldHideSourceTableFromBossNote(tableName) {
			continue
		}
		meta, ok := metadata[tableName]
		if !ok {
			meta = dbpkg.DefaultTableSourceMetadata(tableName)
		}
		display := strings.TrimSpace(meta.Display)
		if display == "" {
			continue
		}
		out = append(out, display)
	}
	return dedupeSourceTables(out...)
}

func shouldHideSourceTableFromBossNote(tableName string) bool {
	switch baseSourceTableName(tableName) {
	case "fin_fund_income_groups",
		"fin_fund_income_group_members",
		"fin_cost_settlement_groups",
		"fin_cost_settlement_group_members":
		return true
	default:
		return false
	}
}

func (e *Engine) buildSourceUpdateNote(tables []string, metadata map[string]dbpkg.TableSourceMetadata) string {
	updatedAt := e.latestSourceUpdatedAt(tables, metadata)
	if updatedAt == "" {
		return ""
	}
	return "来源更新时间：" + updatedAt
}

func (e *Engine) latestSourceUpdatedAt(tables []string, metadata map[string]dbpkg.TableSourceMetadata) string {
	rowLatest := ""
	metaLatest := ""
	for _, tableName := range tables {
		if !shouldSkipSourceTableRowUpdate(tableName) {
			if fromRows := e.latestTableUpdatedAt(tableName); fromRows != "" && fromRows > rowLatest {
				rowLatest = fromRows
				continue
			}
		}
		if shouldHideSourceTableFromBossNote(tableName) {
			continue
		}
		if meta, ok := metadata[tableName]; ok {
			if fromMeta := normalizeSourceUpdatedAt(meta.UpdatedAt); fromMeta != "" && fromMeta > metaLatest {
				metaLatest = fromMeta
			}
		}
	}
	if rowLatest != "" {
		return rowLatest
	}
	return metaLatest
}

func shouldSkipSourceTableRowUpdate(tableName string) bool {
	switch baseSourceTableName(tableName) {
	case "fin_fund_income_group_members", "fin_cost_settlement_group_members":
		return true
	default:
		return false
	}
}

func (e *Engine) latestTableUpdatedAt(tableName string) string {
	base := baseSourceTableName(tableName)
	cols := e.tableColumns(base)
	if len(cols) == 0 {
		cols = e.tableColumns(tableName)
	}
	col := ""
	switch {
	case cols["updated_at"]:
		col = "updated_at"
	case cols["created_at"]:
		col = "created_at"
	default:
		return ""
	}
	target := tableName
	if len(e.tableColumns(target)) == 0 && len(e.tableColumns(base)) > 0 {
		target = base
	}
	var raw sql.NullString
	query := fmt.Sprintf("SELECT COALESCE(CAST(MAX(%s) AS TEXT), '') FROM %s", col, target)
	if err := e.db.QueryRow(query).Scan(&raw); err != nil {
		return ""
	}
	return normalizeSourceUpdatedAt(raw.String)
}

func normalizeSourceUpdatedAt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, "T"); idx > 0 {
		value = strings.Replace(value, "T", " ", 1)
	}
	value = strings.TrimSuffix(value, "Z")
	if idx := strings.Index(value, "."); idx > 0 {
		value = value[:idx]
	}
	if len(value) >= len("2006-01-02 15:04:05") {
		return value[:len("2006-01-02 15:04:05")]
	}
	return value
}
