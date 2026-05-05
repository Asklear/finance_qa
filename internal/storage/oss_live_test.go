package storage_test

import (
	"bytes"
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/storage"
)

func TestLiveOSSUploadDownloadSmoke(t *testing.T) {
	if strings.TrimSpace(os.Getenv("RUN_LIVE_OSS_SMOKE")) != "1" {
		t.Skip("set RUN_LIVE_OSS_SMOKE=1 to run live OSS smoke test")
	}

	client, err := storage.NewOSSClientFromEnv()
	if err != nil {
		t.Fatalf("NewOSSClientFromEnv: %v", err)
	}
	if client == nil {
		t.Skip("OSS env is not configured")
	}

	payload := []byte("%PDF-1.4\n% financeqa oss smoke\n")
	src := filepath.Join(t.TempDir(), "financeqa-oss-smoke.pdf")
	if err := os.WriteFile(src, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	prefix := strings.Trim(strings.TrimSpace(os.Getenv("OSS_SMOKE_PREFIX")), "/")
	if prefix == "" {
		prefix = "tmp/financeqa-smoke"
	}
	key := path.Join(prefix, "financeqa-oss-smoke.pdf")
	uri, err := client.PutFile(context.Background(), src, key, "application/pdf")
	if err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "downloaded.pdf")
	if err := client.DownloadToFile(context.Background(), uri, dest); err != nil {
		t.Fatalf("DownloadToFile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("downloaded payload mismatch: got %d bytes want %d", len(got), len(payload))
	}
}
