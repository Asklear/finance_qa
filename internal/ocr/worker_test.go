package ocr_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"financeqa/internal/ocr"
)

func TestWorkerProcessesPendingDocuments(t *testing.T) {
	store := &fakeOCRStore{
		docs: []ocr.PendingDocument{
			{ID: 1, FileName: "a.pdf", StorageKey: "/tmp/a.pdf"},
			{ID: 2, FileName: "b.pdf", StorageKey: "/tmp/b.pdf"},
		},
	}
	extractor := &fakeOCRExtractor{
		result: ocr.Result{
			DocumentType:   ocr.DocumentTypeContract,
			OCRTextExcerpt: "合同证据",
			Contract: ocr.ContractResult{
				ContractTitle: "数据服务合同",
				PartyA:        "甲方公司",
				PartyB:        "乙方公司",
			},
		},
		run: testRunMeta(),
	}

	summary, err := ocr.NewWorker(store, extractor).ProcessPending(context.Background(), ocr.ProcessOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}
	if summary.Claimed != 2 || summary.Processed != 2 || summary.Failed != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(store.saved) != 2 {
		t.Fatalf("saved = %#v", store.saved)
	}
	if extractor.calls != 2 {
		t.Fatalf("extractor calls = %d", extractor.calls)
	}
}

func TestWorkerMarksFailedOnExtractorError(t *testing.T) {
	store := &fakeOCRStore{
		docs: []ocr.PendingDocument{{ID: 9, FileName: "bad.pdf", StorageKey: "/tmp/bad.pdf"}},
	}
	extractor := &fakeOCRExtractor{err: errors.New("gemini unavailable")}

	summary, err := ocr.NewWorker(store, extractor).ProcessPending(context.Background(), ocr.ProcessOptions{Limit: 1})
	if err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}
	if summary.Claimed != 1 || summary.Processed != 0 || summary.Failed != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(store.failed) != 1 || store.failed[0].id != 9 {
		t.Fatalf("failed = %#v", store.failed)
	}
}

func TestWorkerHonorsConcurrencyLimit(t *testing.T) {
	store := &fakeOCRStore{
		docs: []ocr.PendingDocument{
			{ID: 1, FileName: "a.pdf", StorageKey: "/tmp/a.pdf"},
			{ID: 2, FileName: "b.pdf", StorageKey: "/tmp/b.pdf"},
			{ID: 3, FileName: "c.pdf", StorageKey: "/tmp/c.pdf"},
			{ID: 4, FileName: "d.pdf", StorageKey: "/tmp/d.pdf"},
		},
	}
	extractor := &fakeOCRExtractor{
		result: ocr.Result{
			DocumentType:   ocr.DocumentTypeContract,
			OCRTextExcerpt: "合同证据",
			Contract: ocr.ContractResult{
				ContractTitle: "数据服务合同",
				PartyA:        "甲方公司",
				PartyB:        "乙方公司",
			},
		},
		run:   testRunMeta(),
		delay: 30 * time.Millisecond,
	}

	summary, err := ocr.NewWorker(store, extractor).ProcessPending(context.Background(), ocr.ProcessOptions{Limit: 10, Concurrency: 2})
	if err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}
	if summary.Processed != 4 || summary.Failed != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	if extractor.maxActive != 2 {
		t.Fatalf("max active extractors = %d, want 2", extractor.maxActive)
	}
}

type fakeOCRStore struct {
	mu     sync.Mutex
	docs   []ocr.PendingDocument
	saved  []savedOCRResult
	failed []failedOCRResult
}

func (s *fakeOCRStore) ClaimPending(_ context.Context, limit int) ([]ocr.PendingDocument, error) {
	if limit > 0 && limit < len(s.docs) {
		return s.docs[:limit], nil
	}
	return s.docs, nil
}

func (s *fakeOCRStore) SaveResult(_ context.Context, doc ocr.PendingDocument, result ocr.Result, quality ocr.QualityReport, run ocr.RunMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, savedOCRResult{doc: doc, result: result, quality: quality, run: run})
	return nil
}

func (s *fakeOCRStore) MarkFailed(_ context.Context, doc ocr.PendingDocument, run ocr.RunMetadata, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = append(s.failed, failedOCRResult{id: doc.ID, run: run, errMsg: errMsg})
	return nil
}

type fakeOCRExtractor struct {
	mu        sync.Mutex
	result    ocr.Result
	run       ocr.RunMetadata
	err       error
	delay     time.Duration
	calls     int
	active    int
	maxActive int
}

func (e *fakeOCRExtractor) ExtractPDF(ctx context.Context, _ string) (ocr.Result, ocr.RunMetadata, error) {
	e.mu.Lock()
	e.calls++
	e.active++
	if e.active > e.maxActive {
		e.maxActive = e.active
	}
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.active--
		e.mu.Unlock()
	}()
	if e.delay > 0 {
		select {
		case <-time.After(e.delay):
		case <-ctx.Done():
			return ocr.Result{}, ocr.RunMetadata{Model: "fake", ProcessedAt: time.Now().UTC()}, ctx.Err()
		}
	}
	if e.err != nil {
		return ocr.Result{}, ocr.RunMetadata{Model: "fake", ProcessedAt: time.Now().UTC()}, e.err
	}
	return e.result, e.run, nil
}

type savedOCRResult struct {
	doc     ocr.PendingDocument
	result  ocr.Result
	quality ocr.QualityReport
	run     ocr.RunMetadata
}

type failedOCRResult struct {
	id     int64
	run    ocr.RunMetadata
	errMsg string
}
