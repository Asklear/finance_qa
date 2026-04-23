package ingest

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"financeqa/internal/parser"
)

type SyncSummary struct {
	Directory string          `json:"directory"`
	Processed []ImportSummary `json:"processed"`
	Skipped   []string        `json:"skipped"`
}

func (i *Importer) SyncDirectory(ctx context.Context, dbPath, dir string, incremental bool) (SyncSummary, error) {
	return i.SyncDirectoryWithOptions(ctx, dbPath, dir, ImportOptions{
		Incremental: incremental,
	})
}

func (i *Importer) SyncDirectoryWithOptions(ctx context.Context, dbPath, dir string, opts ImportOptions) (SyncSummary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return SyncSummary{}, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "~") || strings.HasPrefix(name, ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".xls" && ext != ".xlsx" && ext != ".csv" && ext != ".txt" && ext != ".tsv" {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)

	summary := SyncSummary{
		Directory: dir,
		Processed: []ImportSummary{},
		Skipped:   []string{},
	}
	for _, file := range files {
		if _, ok, err := detectContractWorkbookKind(file); err != nil {
			summary.Skipped = append(summary.Skipped, file)
			continue
		} else if ok {
			imported, err := i.ImportFileWithOptions(ctx, dbPath, file, opts)
			if err != nil {
				return summary, err
			}
			summary.Processed = append(summary.Processed, imported)
			continue
		}

		if _, err := parser.ParseFile(file); err != nil {
			summary.Skipped = append(summary.Skipped, file)
			continue
		}
		imported, err := i.ImportFileWithOptions(ctx, dbPath, file, opts)
		if err != nil {
			return summary, err
		}
		summary.Processed = append(summary.Processed, imported)
	}
	return summary, nil
}
