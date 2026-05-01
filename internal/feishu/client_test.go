package feishu_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"financeqa/internal/feishu"
)

func TestHTTPClientFetchesTenantTokenAndListsFolderFiles(t *testing.T) {
	t.Parallel()

	var tokenRequests int
	var seenAuthHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			tokenRequests++
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode token body: %v", err)
			}
			if body["app_id"] != "app-id" || body["app_secret"] != "app-secret" {
				t.Fatalf("token body = %#v", body)
			}
			writeFeishuJSON(t, w, map[string]any{
				"code":                0,
				"tenant_access_token": "tenant-token",
				"expire":              7200,
			})
		case "/open-apis/drive/v1/files":
			seenAuthHeader = r.Header.Get("Authorization")
			if got := r.URL.Query().Get("folder_token"); got != "folder-token" {
				t.Fatalf("folder_token = %q", got)
			}
			if r.URL.Query().Get("page_token") == "" {
				writeFeishuJSON(t, w, map[string]any{
					"code": 0,
					"data": map[string]any{
						"has_more":   true,
						"page_token": "next-page",
						"files": []map[string]any{
							{
								"token":         "file-1",
								"name":          "合同1.pdf",
								"type":          "file",
								"mime_type":     "application/pdf",
								"parent_token":  "folder-token",
								"size":          "12",
								"modified_time": "1714560000",
							},
						},
					},
				})
				return
			}
			writeFeishuJSON(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"has_more": false,
					"files": []map[string]any{
						{
							"token":        "file-2",
							"name":         "合同2.pdf",
							"type":         "file",
							"parent_token": "folder-token",
							"size":         34,
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := feishu.NewHTTPClient("app-id", "app-secret", feishu.WithBaseURL(server.URL))
	files, err := client.ListFolderFiles(t.Context(), "folder-token")
	if err != nil {
		t.Fatalf("list folder files: %v", err)
	}

	if tokenRequests != 1 {
		t.Fatalf("token requests = %d, want 1", tokenRequests)
	}
	if seenAuthHeader != "Bearer tenant-token" {
		t.Fatalf("authorization header = %q", seenAuthHeader)
	}
	if len(files) != 2 {
		t.Fatalf("files = %#v", files)
	}
	if files[0].Token != "file-1" || files[0].Size != 12 || files[1].Token != "file-2" || files[1].Size != 34 {
		t.Fatalf("files = %#v", files)
	}
}

func TestHTTPClientDownloadsFile(t *testing.T) {
	t.Parallel()

	server := newTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/drive/v1/files/file-token/download" {
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tenant-token" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte("pdf bytes"))
	})
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "download.pdf")
	client := feishu.NewHTTPClient("app-id", "app-secret", feishu.WithBaseURL(server.URL))
	if err := client.DownloadFile(t.Context(), "file-token", dest); err != nil {
		t.Fatalf("download file: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != "pdf bytes" {
		t.Fatalf("downloaded bytes = %q", got)
	}
}

func TestHTTPClientGetsFileMetadata(t *testing.T) {
	t.Parallel()

	server := newTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/drive/v1/files/file-token" {
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
		writeFeishuJSON(t, w, map[string]any{
			"code": 0,
			"data": map[string]any{
				"file": map[string]any{
					"token":         "file-token",
					"name":          "财务表.xlsx",
					"type":          "file",
					"mime_type":     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
					"parent_token":  "folder-token",
					"size":          "1024",
					"modified_time": "1714560000",
				},
			},
		})
	})
	defer server.Close()

	client := feishu.NewHTTPClient("app-id", "app-secret", feishu.WithBaseURL(server.URL))
	meta, err := client.GetFileMetadata(t.Context(), "file-token")
	if err != nil {
		t.Fatalf("get file metadata: %v", err)
	}
	if meta.Token != "file-token" || meta.Name != "财务表.xlsx" || meta.Size != 1024 {
		t.Fatalf("metadata = %#v", meta)
	}
}

func TestHTTPClientGetsDirectFileMetadata(t *testing.T) {
	t.Parallel()

	server := newTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/drive/v1/files/file-token" {
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
		writeFeishuJSON(t, w, map[string]any{
			"code": 0,
			"data": map[string]any{
				"token": "file-token",
				"name":  "财务表.xlsx",
				"type":  "file",
				"size":  2048,
			},
		})
	})
	defer server.Close()

	client := feishu.NewHTTPClient("app-id", "app-secret", feishu.WithBaseURL(server.URL))
	meta, err := client.GetFileMetadata(t.Context(), "file-token")
	if err != nil {
		t.Fatalf("get file metadata: %v", err)
	}
	if meta.Token != "file-token" || meta.Size != 2048 {
		t.Fatalf("metadata = %#v", meta)
	}
}

func TestHTTPClientExportsToXLSX(t *testing.T) {
	t.Parallel()

	var taskChecks int
	server := newTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/drive/v1/export_tasks":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode export body: %v", err)
			}
			if body["token"] != "sheet-token" || body["file_extension"] != "xlsx" {
				t.Fatalf("export body = %#v", body)
			}
			writeFeishuJSON(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{"ticket": "ticket-1"},
			})
		case "/open-apis/drive/v1/export_tasks/ticket-1":
			taskChecks++
			if taskChecks == 1 {
				writeFeishuJSON(t, w, map[string]any{
					"code": 0,
					"data": map[string]any{
						"result": map[string]any{"job_status": 1},
					},
				})
				return
			}
			writeFeishuJSON(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"result": map[string]any{
						"job_status": 0,
						"file_token": "export-file-token",
					},
				},
			})
		case "/open-apis/drive/v1/export_tasks/file/export-file-token/download":
			_, _ = w.Write([]byte("xlsx bytes"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	})
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "export.xlsx")
	client := feishu.NewHTTPClient("app-id", "app-secret",
		feishu.WithBaseURL(server.URL),
		feishu.WithExportPollInterval(time.Millisecond),
	)
	if err := client.ExportToXLSX(t.Context(), "sheet-token", dest); err != nil {
		t.Fatalf("export to xlsx: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if string(got) != "xlsx bytes" {
		t.Fatalf("export bytes = %q", got)
	}
	if taskChecks != 2 {
		t.Fatalf("task checks = %d, want 2", taskChecks)
	}
}

func TestHTTPClientFromEnvRequiresCredentials(t *testing.T) {
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")

	if _, err := feishu.NewHTTPClientFromEnv(); err == nil {
		t.Fatalf("expected missing env error")
	}
}

func newTokenServer(t *testing.T, next http.HandlerFunc) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			writeFeishuJSON(t, w, map[string]any{
				"code":                0,
				"tenant_access_token": "tenant-token",
				"expire":              7200,
			})
			return
		}
		next(w, r)
	}))
}

func writeFeishuJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
