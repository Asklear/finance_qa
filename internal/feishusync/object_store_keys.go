package feishusync

import (
	"context"
	"fmt"
	"path"
	"strings"
)

func sameContentHash(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func unusedHashSuffixedObjectKey(ctx context.Context, probe ObjectStoreProbe, key, hash string) (string, bool, error) {
	candidates := []string{objectKeyWithHashSuffix(key, hash)}
	hash = strings.TrimSpace(strings.ToLower(hash))
	if len(hash) > 12 {
		ext := path.Ext(key)
		base := strings.TrimSuffix(key, ext)
		candidates = append(candidates, base+".sha256-"+hash+ext)
	}
	for _, candidate := range candidates {
		remoteHash, exists, err := probe.ObjectSHA256(ctx, candidate)
		if err != nil {
			return "", false, err
		}
		if !exists {
			return candidate, false, nil
		}
		if sameContentHash(remoteHash, hash) {
			return candidate, true, nil
		}
	}
	return "", false, fmt.Errorf("oss object key already exists with different content: %s", candidates[len(candidates)-1])
}
