package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
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

func annotateContractWorkbookSource(ctx context.Context, tx *sql.Tx, dbPath, filePath string, bundle contractImportBundle, incremental bool) error {
	fileName := workbookDisplayName(filePath)

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
	return nil
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
