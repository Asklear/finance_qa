package feishu_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/feishu"
	"financeqa/internal/support"
)

func TestLiveFeishuDriveSmoke(t *testing.T) {
	if strings.TrimSpace(os.Getenv("RUN_LIVE_FEISHU_SMOKE")) != "1" {
		t.Skip("set RUN_LIVE_FEISHU_SMOKE=1 to run live Feishu smoke test")
	}
	_ = support.LoadDotEnv(".env")
	_ = support.LoadDotEnv("../../.env")

	client, err := feishu.NewHTTPClientFromEnv()
	if err != nil {
		t.Fatalf("NewHTTPClientFromEnv: %v", err)
	}

	ctx := context.Background()
	outputDir := filepath.Join("..", "..", "tmp", "feishu-live-smoke")
	folders := splitTokens(os.Getenv("FEISHU_SMOKE_FOLDER_TOKENS"))
	workbookToken := strings.TrimSpace(os.Getenv("FEISHU_SMOKE_WORKBOOK_TOKEN"))
	if len(folders) == 0 && workbookToken == "" {
		t.Skip("set FEISHU_SMOKE_FOLDER_TOKENS or FEISHU_SMOKE_WORKBOOK_TOKEN to choose live Feishu targets")
	}

	for _, folderToken := range folders {
		folderToken := folderToken
		t.Run("folder_"+folderToken, func(t *testing.T) {
			files, err := client.ListFolderFiles(ctx, folderToken)
			if err != nil {
				t.Fatalf("ListFolderFiles(%s): %v", folderToken, err)
			}
			t.Logf("folder %s: listed %d files", folderToken, len(files))
			if len(files) == 0 {
				return
			}

			pdf := firstPDF(files)
			if strings.TrimSpace(pdf.Token) == "" {
				t.Logf("folder %s: no PDF file found for download smoke", folderToken)
				return
			}
			dest := filepath.Join(outputDir, "pdf", folderToken, safeFileName(pdf.Token+"_"+pdf.Name))
			if !strings.EqualFold(filepath.Ext(dest), ".pdf") {
				dest += ".pdf"
			}
			if err := client.DownloadFile(ctx, pdf.Token, dest); err != nil {
				t.Fatalf("DownloadFile(%s/%s): %v", folderToken, pdf.Token, err)
			}
			assertNonEmptyFile(t, dest)
			t.Logf("folder %s: downloaded %s (%d bytes)", folderToken, filepath.Base(dest), fileSize(t, dest))
		})
	}

	if workbookToken == "" {
		return
	}
	t.Run("workbook_"+workbookToken, func(t *testing.T) {
		meta, err := client.GetFileMetadata(ctx, workbookToken)
		if err != nil {
			t.Fatalf("GetFileMetadata(%s): %v", workbookToken, err)
		}
		t.Logf("workbook %s: metadata name=%q mime=%q type=%q revision=%q", workbookToken, meta.Name, meta.MimeType, meta.Type, meta.Revision)

		workbookDest := filepath.Join(outputDir, "workbook", workbookToken+".xlsx")
		if isXLSX(meta) {
			err = client.DownloadFile(ctx, workbookToken, workbookDest)
		} else {
			err = client.ExportToXLSX(ctx, workbookToken, workbookDest)
		}
		if err != nil {
			t.Fatalf("download/export workbook %s: %v", workbookToken, err)
		}
		assertNonEmptyFile(t, workbookDest)
		t.Logf("workbook %s: downloaded/exported %s (%d bytes)", workbookToken, workbookDest, fileSize(t, workbookDest))
	})
}

func splitTokens(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstPDF(files []feishu.DriveFile) feishu.DriveFile {
	for _, file := range files {
		if strings.EqualFold(strings.TrimSpace(file.MimeType), "application/pdf") ||
			strings.EqualFold(filepath.Ext(strings.TrimSpace(file.Name)), ".pdf") {
			return file
		}
	}
	return feishu.DriveFile{}
}

func isXLSX(file feishu.DriveFile) bool {
	return strings.EqualFold(filepath.Ext(strings.TrimSpace(file.Name)), ".xlsx") ||
		strings.EqualFold(strings.TrimSpace(file.MimeType), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
}

func safeFileName(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "_"))
	value = strings.ReplaceAll(value, "/", "_")
	if value == "" {
		return "download"
	}
	return value
}

func assertNonEmptyFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("%s is empty", path)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}
