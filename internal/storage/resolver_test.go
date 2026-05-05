package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"financeqa/internal/storage"
)

func TestResolverKeepsLocalPath(t *testing.T) {
	resolver := storage.NewResolver(nil)
	path := filepath.Join(t.TempDir(), "a.pdf")
	got, cleanup, err := resolver.ResolvePDF(context.Background(), path)
	if err != nil {
		t.Fatalf("ResolvePDF: %v", err)
	}
	if got != path || cleanup != nil {
		t.Fatalf("got=%q cleanup nil=%v", got, cleanup == nil)
	}
}

func TestResolverRequiresOSSForS3(t *testing.T) {
	resolver := storage.NewResolver(nil)
	if _, _, err := resolver.ResolvePDF(context.Background(), "s3://boss-agent/ods/a.pdf"); err != storage.ErrOSSNotConfigured {
		t.Fatalf("err = %v", err)
	}
}

func TestResolverDownloadsS3ToTempFile(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(source, []byte("pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &fakeDownloader{source: source}
	resolver := storage.NewResolverForTest(client)

	got, cleanup, err := resolver.ResolvePDF(context.Background(), "s3://boss-agent/ods/a.pdf")
	if err != nil {
		t.Fatalf("ResolvePDF: %v", err)
	}
	defer cleanup()
	if client.storageKey != "s3://boss-agent/ods/a.pdf" {
		t.Fatalf("storage key = %q", client.storageKey)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "pdf" {
		t.Fatalf("data = %q", data)
	}
}

func TestResolverDownloadsRelativeOSSKeyToTempFile(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(source, []byte("pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &fakeDownloader{source: source}
	resolver := storage.NewResolverForTest(client)

	got, cleanup, err := resolver.ResolvePDF(context.Background(), "tenant/uhub/contract/a.pdf")
	if err != nil {
		t.Fatalf("ResolvePDF: %v", err)
	}
	defer cleanup()
	if client.storageKey != "tenant/uhub/contract/a.pdf" {
		t.Fatalf("storage key = %q", client.storageKey)
	}
	if filepath.Base(got) != "a.pdf" {
		t.Fatalf("temp file = %q", got)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "pdf" {
		t.Fatalf("data = %q", data)
	}
}

type fakeDownloader struct {
	source     string
	storageKey string
}

func (d *fakeDownloader) DownloadToFile(_ context.Context, storageKey, destPath string) error {
	d.storageKey = storageKey
	data, err := os.ReadFile(d.source)
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, data, 0o600)
}
