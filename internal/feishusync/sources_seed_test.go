package feishusync_test

import (
	"strings"
	"testing"

	"financeqa/internal/feishusync"
)

func TestDefaultSourcesRequireConfiguredSources(t *testing.T) {
	t.Setenv("FEISHU_SYNC_SOURCES_JSON", "")
	t.Setenv("FEISHU_SYNC_SOURCES_FILE", "")

	sources, err := feishusync.DefaultSources()
	if err == nil {
		t.Fatalf("DefaultSources error = nil, sources = %#v", sources)
	}
	if !strings.Contains(err.Error(), "FEISHU_SYNC_SOURCES_JSON") {
		t.Fatalf("error should mention config envs: %v", err)
	}
}

func TestDefaultSourcesLoadFromJSONEnv(t *testing.T) {
	t.Setenv("FEISHU_SYNC_SOURCES_FILE", "")
	t.Setenv("FEISHU_SYNC_SOURCES_JSON", `[
		{
			"source_type": "finance_workbook",
			"source_token": "workbook-token",
			"source_url": "https://example.feishu.cn/file/workbook-token",
			"display_name": "财务表",
			"metadata_json": {"oss_prefix": "tenant/uhub/finance/2026"}
		},
		{
			"source_type": "finance_workbook_folder",
			"source_token": "finance-folder-token",
			"source_url": "https://example.feishu.cn/drive/folder/finance-folder-token",
			"display_name": "财务表文件夹",
			"metadata_json": {"oss_prefix": "tenant/uhub/finance"}
		},
		{
			"source_type": "pdf_folder",
			"source_token": "folder-token",
			"source_url": "https://example.feishu.cn/drive/folder/folder-token",
			"display_name": "合同目录",
			"metadata_json": {"oss_prefix": "tenant/uhub/contract"}
		}
	]`)

	sources, err := feishusync.DefaultSources()
	if err != nil {
		t.Fatalf("DefaultSources: %v", err)
	}
	if len(sources) != 3 {
		t.Fatalf("source count = %d, want 3", len(sources))
	}
	if sources[0].SourceToken != "workbook-token" || sources[0].SyncMode != "active_scan" || sources[0].SyncStatus != feishusync.SyncStatusActive {
		t.Fatalf("workbook source defaults not normalized: %#v", sources[0])
	}
	if sources[0].MetadataJSON != `{"oss_prefix":"tenant/uhub/finance/2026"}` {
		t.Fatalf("metadata json = %q", sources[0].MetadataJSON)
	}
	if sources[1].SourceType != feishusync.SourceTypeFinanceWorkbookFolder || sources[1].SourceToken != "finance-folder-token" {
		t.Fatalf("finance folder source = %#v", sources[1])
	}
	if sources[2].SourceType != feishusync.SourceTypePDFFolder || sources[2].SourceToken != "folder-token" {
		t.Fatalf("folder source = %#v", sources[2])
	}
}

func TestDefaultSourcesRejectInvalidJSONEnv(t *testing.T) {
	t.Setenv("FEISHU_SYNC_SOURCES_FILE", "")
	t.Setenv("FEISHU_SYNC_SOURCES_JSON", `{"source_type":"pdf_folder"}`)

	_, err := feishusync.DefaultSources()
	if err == nil {
		t.Fatalf("DefaultSources error = nil")
	}
	if !strings.Contains(err.Error(), "array") {
		t.Fatalf("error should explain expected JSON array: %v", err)
	}
}

func TestDefaultSourcesRejectUnsupportedSourceType(t *testing.T) {
	t.Setenv("FEISHU_SYNC_SOURCES_FILE", "")
	t.Setenv("FEISHU_SYNC_SOURCES_JSON", `[{"source_type":"doc","source_token":"token"}]`)

	_, err := feishusync.DefaultSources()
	if err == nil {
		t.Fatalf("DefaultSources error = nil")
	}
	if !strings.Contains(err.Error(), "unsupported source_type") {
		t.Fatalf("error = %v", err)
	}
}
