package storage_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/storage"
)

func TestOSSClientPutAndDownloadFile(t *testing.T) {
	var putPath, putAuth, putContentType, putDate string
	var putSHA256 string
	var putBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			putPath = r.URL.Path
			putAuth = r.Header.Get("Authorization")
			putContentType = r.Header.Get("Content-Type")
			putDate = r.Header.Get("Date")
			putSHA256 = r.Header.Get("x-oss-meta-sha256")
			putBody = mustReadRequestBody(t, r)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if r.URL.Path != "/boss-agent/tenant/uhub/contract/file.pdf" {
				t.Fatalf("GET path = %s", r.URL.Path)
			}
			_, _ = w.Write([]byte("pdf-bytes"))
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
	defer server.Close()

	src := filepath.Join(t.TempDir(), "file.pdf")
	if err := os.WriteFile(src, []byte("upload-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := storage.NewOSSClient(storage.OSSConfig{
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-secret",
		Bucket:          "boss-agent",
		Endpoint:        server.URL,
	})

	key, err := client.PutFile(context.Background(), src, "tenant/uhub/contract/file.pdf", "application/pdf")
	if err != nil {
		t.Fatalf("PutFile: %v", err)
	}
	if key != "tenant/uhub/contract/file.pdf" {
		t.Fatalf("key = %q", key)
	}
	if putPath != "/boss-agent/tenant/uhub/contract/file.pdf" {
		t.Fatalf("PUT path = %s", putPath)
	}
	if !strings.HasPrefix(putAuth, "OSS test-ak:") {
		t.Fatalf("authorization = %q", putAuth)
	}
	if putContentType != "application/pdf" {
		t.Fatalf("content type = %q", putContentType)
	}
	wantHashBytes := sha256.Sum256([]byte("upload-bytes"))
	if putSHA256 != hex.EncodeToString(wantHashBytes[:]) {
		t.Fatalf("x-oss-meta-sha256 = %q", putSHA256)
	}
	if putDate == "" {
		t.Fatal("missing Date header")
	}
	stringToSign := http.MethodPut + "\n\n" + putContentType + "\n" + putDate + "\n" +
		"x-oss-meta-sha256:" + putSHA256 + "\n" + putPath
	if wantAuth := "OSS test-ak:" + hmacSHA1Base64("test-secret", stringToSign); putAuth != wantAuth {
		t.Fatalf("authorization = %q, want %q", putAuth, wantAuth)
	}
	if string(putBody) != "upload-bytes" {
		t.Fatalf("put body = %q", putBody)
	}

	dest := filepath.Join(t.TempDir(), "download.pdf")
	if err := client.DownloadToFile(context.Background(), key, dest); err != nil {
		t.Fatalf("DownloadToFile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pdf-bytes" {
		t.Fatalf("downloaded = %q", got)
	}
}

func TestParseS3URI(t *testing.T) {
	ref, err := storage.ParseS3URI("s3://boss-agent/ods/a.pdf")
	if err != nil {
		t.Fatalf("ParseS3URI: %v", err)
	}
	if ref.Bucket != "boss-agent" || ref.Key != "ods/a.pdf" {
		t.Fatalf("ref = %#v", ref)
	}
}

func TestOSSClientObjectSHA256UsesObjectMetadata(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		if r.URL.Path != "/boss-agent/tenant/uhub/contract/a.pdf" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("x-oss-meta-sha256", "abc123")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := storage.NewOSSClient(storage.OSSConfig{
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-secret",
		Bucket:          "boss-agent",
		Endpoint:        server.URL,
	})

	hash, exists, err := client.ObjectSHA256(context.Background(), "tenant/uhub/contract/a.pdf")
	if err != nil {
		t.Fatalf("ObjectSHA256: %v", err)
	}
	if !exists || hash != "abc123" || gotMethod != http.MethodHead {
		t.Fatalf("hash=%q exists=%v method=%s", hash, exists, gotMethod)
	}
}

func TestOSSClientObjectSHA256StreamsObjectWhenMetadataMissing(t *testing.T) {
	body := []byte("existing-object")
	wantBytes := sha256.Sum256(body)
	want := hex.EncodeToString(wantBytes[:])
	var sawGet bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/boss-agent/tenant/uhub/finance/2026/a.xlsx" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			sawGet = true
			_, _ = w.Write(body)
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
	defer server.Close()

	client := storage.NewOSSClient(storage.OSSConfig{
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-secret",
		Bucket:          "boss-agent",
		Endpoint:        server.URL,
	})

	hash, exists, err := client.ObjectSHA256(context.Background(), "tenant/uhub/finance/2026/a.xlsx")
	if err != nil {
		t.Fatalf("ObjectSHA256: %v", err)
	}
	if !exists || hash != want || !sawGet {
		t.Fatalf("hash=%q exists=%v sawGet=%v want=%q", hash, exists, sawGet, want)
	}
}

func TestOSSClientObjectSHA256ReportsMissingObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("method = %s", r.Method)
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := storage.NewOSSClient(storage.OSSConfig{
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-secret",
		Bucket:          "boss-agent",
		Endpoint:        server.URL,
	})

	hash, exists, err := client.ObjectSHA256(context.Background(), "tenant/uhub/contract/missing.pdf")
	if err != nil {
		t.Fatalf("ObjectSHA256: %v", err)
	}
	if exists || hash != "" {
		t.Fatalf("hash=%q exists=%v", hash, exists)
	}
}

func TestOSSClientFindObjectBySHA256ScansPrefix(t *testing.T) {
	body := []byte("existing-object")
	wantBytes := sha256.Sum256(body)
	want := hex.EncodeToString(wantBytes[:])
	var listed bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/boss-agent/" && r.URL.Query().Get("prefix") == "tenant/uhub/contract":
			listed = true
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<ListBucketResult>
<IsTruncated>false</IsTruncated>
<Contents><Key>tenant/uhub/contract/a.pdf</Key></Contents>
<Contents><Key>tenant/uhub/contract/b.pdf</Key></Contents>
</ListBucketResult>`))
		case r.Method == http.MethodHead && r.URL.Path == "/boss-agent/tenant/uhub/contract/a.pdf":
			w.Header().Set("x-oss-meta-sha256", "not-it")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodHead && r.URL.Path == "/boss-agent/tenant/uhub/contract/b.pdf":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/boss-agent/tenant/uhub/contract/b.pdf":
			_, _ = w.Write(body)
		default:
			t.Fatalf("unexpected request %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()

	client := storage.NewOSSClient(storage.OSSConfig{
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-secret",
		Bucket:          "boss-agent",
		Endpoint:        server.URL,
	})

	key, exists, err := client.FindObjectBySHA256(context.Background(), "tenant/uhub/contract", want)
	if err != nil {
		t.Fatalf("FindObjectBySHA256: %v", err)
	}
	if !exists || key != "tenant/uhub/contract/b.pdf" || !listed {
		t.Fatalf("key=%q exists=%v listed=%v", key, exists, listed)
	}
}

func hmacSHA1Base64(secret, text string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write([]byte(text))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func mustReadRequestBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
