package ocr

import (
	"context"
	"fmt"
	"sync"
)

type Extractor interface {
	ExtractPDF(ctx context.Context, filePath string) (Result, RunMetadata, error)
}

type Store interface {
	ClaimPending(ctx context.Context, limit int) ([]PendingDocument, error)
	SaveResult(ctx context.Context, doc PendingDocument, result Result, quality QualityReport, run RunMetadata) error
	MarkFailed(ctx context.Context, doc PendingDocument, run RunMetadata, errMsg string) error
}

type Worker struct {
	store     Store
	extractor Extractor
}

type ProcessOptions struct {
	Limit       int
	Concurrency int
}

type ProcessSummary struct {
	Claimed     int `json:"claimed"`
	Processed   int `json:"processed"`
	Failed      int `json:"failed"`
	NeedsReview int `json:"needs_review"`
}

func NewWorker(store Store, extractor Extractor) *Worker {
	return &Worker{store: store, extractor: extractor}
}

func (w *Worker) ProcessPending(ctx context.Context, opts ProcessOptions) (ProcessSummary, error) {
	docs, err := w.store.ClaimPending(ctx, opts.Limit)
	if err != nil {
		return ProcessSummary{}, err
	}
	summary := ProcessSummary{Claimed: len(docs)}
	if len(docs) == 0 {
		return summary, nil
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(docs) {
		concurrency = len(docs)
	}

	jobs := make(chan PendingDocument)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for doc := range jobs {
				quality, err := w.ProcessDocument(ctx, doc)
				mu.Lock()
				if err != nil {
					summary.Failed++
				} else {
					summary.Processed++
					if quality.Status == QualityNeedsReview || quality.Status == QualityFailed {
						summary.NeedsReview++
					}
				}
				mu.Unlock()
			}
		}()
	}
	for _, doc := range docs {
		select {
		case jobs <- doc:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return summary, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	return summary, nil
}

func (w *Worker) ProcessDocument(ctx context.Context, doc PendingDocument) (QualityReport, error) {
	result, run, err := w.extractor.ExtractPDF(ctx, doc.StorageKey)
	if err != nil {
		if markErr := w.store.MarkFailed(ctx, doc, run, err.Error()); markErr != nil {
			return QualityReport{}, fmt.Errorf("extract pdf: %w; mark failed: %v", err, markErr)
		}
		return QualityReport{}, err
	}
	quality := Validate(result)
	if err := w.store.SaveResult(ctx, doc, result, quality, run); err != nil {
		if markErr := w.store.MarkFailed(ctx, doc, run, err.Error()); markErr != nil {
			return quality, fmt.Errorf("save ocr result: %w; mark failed: %v", err, markErr)
		}
		return quality, err
	}
	return quality, nil
}
