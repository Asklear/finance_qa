package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type Resolver struct {
	oss downloader
}

type downloader interface {
	DownloadToFile(ctx context.Context, storageKey, destPath string) error
}

func NewResolver(oss *OSSClient) *Resolver {
	if oss == nil {
		return &Resolver{}
	}
	return &Resolver{oss: oss}
}

func NewResolverForTest(oss downloader) *Resolver {
	return &Resolver{oss: oss}
}

func NewResolverFromEnv() (*Resolver, error) {
	oss, err := NewOSSClientFromEnv()
	if err != nil {
		return nil, err
	}
	return NewResolver(oss), nil
}

func (r *Resolver) ResolvePDF(ctx context.Context, storageKey string) (string, func(), error) {
	storageKey = strings.TrimSpace(storageKey)
	if storageKey == "" {
		return storageKey, nil, nil
	}
	if !strings.HasPrefix(storageKey, "s3://") {
		if info, err := os.Stat(storageKey); err == nil && !info.IsDir() {
			return storageKey, nil, nil
		}
		if filepath.IsAbs(storageKey) || strings.HasPrefix(storageKey, ".") {
			return storageKey, nil, nil
		}
	}
	if r == nil || r.oss == nil {
		return "", nil, ErrOSSNotConfigured
	}
	tmpDir, err := os.MkdirTemp("", "financeqa-ocr-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	dest := filepath.Join(tmpDir, filepath.Base(objectKeyBase(storageKey)))
	if err := r.oss.DownloadToFile(ctx, storageKey, dest); err != nil {
		cleanup()
		return "", nil, err
	}
	return dest, cleanup, nil
}

func objectKeyBase(storageKey string) string {
	storageKey = strings.TrimSpace(storageKey)
	if strings.HasPrefix(storageKey, "s3://") {
		if ref, err := ParseS3URI(storageKey); err == nil {
			return ref.Key
		}
	}
	return storageKey
}
