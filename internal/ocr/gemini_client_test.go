package ocr_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/ocr"
)

func TestGeminiClientExtractsJSONAndUsage(t *testing.T) {
	var sawPDF bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/models/test-model:generateContent") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		body := mustReadBody(t, r)
		sawPDF = strings.Contains(body, `"mimeType":"application/pdf"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{"content":{"parts":[{"text":"{\"document_type\":\"contract\",\"contract\":{\"contract_title\":\"测试合同\"},\"invoice\":{},\"pages\":[{\"page_number\":1,\"plain_text\":\"第一页全文\"}],\"ocr_text_excerpt\":\"证据\",\"confidence_notes\":\"ok\",\"quality_flags\":[]}"}]}}],
			"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}
		}`))
	}))
	defer server.Close()

	pdf := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF test"), 0o600); err != nil {
		t.Fatal(err)
	}

	client := ocr.NewGeminiClient(ocr.GeminiConfig{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: server.URL,
	})
	result, meta, err := client.ExtractPDF(context.Background(), pdf)
	if err != nil {
		t.Fatalf("ExtractPDF: %v", err)
	}
	if !sawPDF {
		t.Fatal("request did not include pdf")
	}
	if result.DocumentType != ocr.DocumentTypeContract || result.Contract.ContractTitle != "测试合同" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Pages) != 1 || result.Pages[0].PlainText != "第一页全文" {
		t.Fatalf("pages = %#v", result.Pages)
	}
	if meta.Usage.InputTokens != 10 || meta.Usage.OutputTokens != 5 {
		t.Fatalf("usage = %#v", meta.Usage)
	}
	if meta.EstimatedCostUSD <= 0 {
		t.Fatalf("estimated cost should be positive: %#v", meta)
	}
}

func TestGeminiClientResolvesStorageKeyBeforeExtracting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := mustReadBody(t, r)
		if !strings.Contains(body, `"mimeType":"application/pdf"`) {
			t.Fatalf("request did not include pdf: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{"content":{"parts":[{"text":"{\"document_type\":\"contract\",\"contract\":{\"contract_title\":\"解析后合同\"},\"invoice\":{},\"pages\":[],\"ocr_text_excerpt\":\"证据\",\"confidence_notes\":\"ok\",\"quality_flags\":[]}"}]}}],
			"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}
		}`))
	}))
	defer server.Close()

	resolvedPDF := filepath.Join(t.TempDir(), "resolved.pdf")
	if err := os.WriteFile(resolvedPDF, []byte("%PDF resolved"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := &fakePDFResolver{path: resolvedPDF}
	client := ocr.NewGeminiClient(ocr.GeminiConfig{
		APIKey:       "test-key",
		Model:        "test-model",
		BaseURL:      server.URL,
		FileResolver: resolver,
	})
	result, _, err := client.ExtractPDF(context.Background(), "s3://boss-agent/ods/file.pdf")
	if err != nil {
		t.Fatalf("ExtractPDF: %v", err)
	}
	if resolver.got != "s3://boss-agent/ods/file.pdf" || !resolver.cleaned {
		t.Fatalf("resolver got=%q cleaned=%v", resolver.got, resolver.cleaned)
	}
	if result.Contract.ContractTitle != "解析后合同" {
		t.Fatalf("result = %#v", result)
	}
}

func TestGeminiClientAcceptsTrailingModelTextAfterJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = mustReadBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{"content":{"parts":[{"text":"{\"document_type\":\"contract\",\"contract\":{\"contract_title\":\"月之暗面技术服务协议\"},\"invoice\":{},\"pages\":[],\"ocr_text_excerpt\":\"证据\",\"confidence_notes\":\"ok\",\"quality_flags\":[]}}"}]}}],
			"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}
		}`))
	}))
	defer server.Close()

	pdf := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF test"), 0o600); err != nil {
		t.Fatal(err)
	}

	client := ocr.NewGeminiClient(ocr.GeminiConfig{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: server.URL,
	})
	result, _, err := client.ExtractPDF(context.Background(), pdf)
	if err != nil {
		t.Fatalf("ExtractPDF: %v", err)
	}
	if result.Contract.ContractTitle != "月之暗面技术服务协议" {
		t.Fatalf("result = %#v", result)
	}
}

type fakePDFResolver struct {
	path    string
	got     string
	cleaned bool
}

func (r *fakePDFResolver) ResolvePDF(_ context.Context, storageKey string) (string, func(), error) {
	r.got = storageKey
	return r.path, func() { r.cleaned = true }, nil
}

func mustReadBody(t *testing.T, r *http.Request) string {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
