package feishusync_test

import (
	"os"
	"path/filepath"
	"testing"

	"financeqa/internal/feishusync"
)

func TestSlotKeyNormalizesFilename(t *testing.T) {
	t.Parallel()

	got := feishusync.SlotKey("folder-token", " 合同A .PDF ")
	want := "folder-token:合同a.pdf"
	if got != want {
		t.Fatalf("slot key = %q, want %q", got, want)
	}
}

func TestNormalizeFileNameCollapsesWhitespace(t *testing.T) {
	t.Parallel()

	got := feishusync.NormalizeFileName("  采购   合同\tA.Pdf  ")
	want := "采购 合同 a.pdf"
	if got != want {
		t.Fatalf("normalized name = %q, want %q", got, want)
	}
}

func TestFileSHA256(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := feishusync.FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("hash = %s", got)
	}
}
