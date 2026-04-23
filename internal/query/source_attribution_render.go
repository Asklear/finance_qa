package query

import (
	"context"
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

	collected := e.collectSourceTables(spec, result.Data)
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

	if !strings.Contains(result.Message, "来源：") {
		result.Message = strings.TrimSpace(result.Message) + "\n" + sourceNote
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
