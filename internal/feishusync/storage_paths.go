package feishusync

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"financeqa/internal/feishu"
)

const (
	defaultContractOSSPrefix = "tenant/uhub/contract"
	defaultFinanceOSSPrefix  = "tenant/uhub/finance"
)

type sourceStorageConfig struct {
	OSSPrefix  string `json:"oss_prefix"`
	StorageKey string `json:"storage_key"`
}

func pdfObjectKey(src SyncSource, file feishu.DriveFile, relativePath string) string {
	prefix := sourceOSSPrefix(src, "OSS_CONTRACT_PREFIX", defaultContractOSSPrefix)
	if rel := cleanObjectPath(relativePath); rel != "" {
		if !strings.EqualFold(filepath.Ext(rel), ".pdf") {
			rel += ".pdf"
		}
		return path.Join(prefix, rel)
	}
	name := safeObjectKeyPart(strings.TrimSpace(file.Name))
	if name == "" {
		name = safeObjectKeyPart(strings.TrimSpace(file.Token)) + ".pdf"
	}
	if !strings.EqualFold(filepath.Ext(name), ".pdf") {
		name += ".pdf"
	}
	return path.Join(prefix, name)
}

func workbookObjectKey(src SyncSource, file feishu.DriveFile) string {
	prefix := financeOSSPrefix(src, file)
	name := safeObjectKeyPart(strings.TrimSpace(file.Name))
	if name == "" {
		name = safeObjectKeyPart(strings.TrimSpace(file.Token)) + ".xlsx"
	}
	if !strings.EqualFold(filepath.Ext(name), ".xlsx") {
		name += ".xlsx"
	}
	return path.Join(prefix, name)
}

func sourceOSSPrefix(src SyncSource, envName, fallback string) string {
	if cfg := parseSourceStorageConfig(src.MetadataJSON); strings.TrimSpace(cfg.OSSPrefix) != "" {
		return cleanObjectPrefix(cfg.OSSPrefix)
	}
	if envName != "" {
		if prefix := strings.TrimSpace(os.Getenv(envName)); prefix != "" {
			return cleanObjectPrefix(prefix)
		}
	}
	return cleanObjectPrefix(fallback)
}

func financeOSSPrefix(src SyncSource, file feishu.DriveFile) string {
	prefix := sourceOSSPrefix(src, "OSS_FINANCE_PREFIX", defaultFinanceOSSPrefix)
	if isFinanceRootPrefix(prefix) {
		if year := inferFinanceYear(file.Name); year != "" {
			return path.Join(prefix, year)
		}
	}
	return prefix
}

func sourceStorageKey(metadata string) string {
	return strings.TrimSpace(parseSourceStorageConfig(metadata).StorageKey)
}

func parseSourceStorageConfig(metadata string) sourceStorageConfig {
	var cfg sourceStorageConfig
	if strings.TrimSpace(metadata) == "" {
		return cfg
	}
	_ = json.Unmarshal([]byte(metadata), &cfg)
	return cfg
}

func cleanObjectPrefix(prefix string) string {
	prefix = strings.TrimSpace(strings.ReplaceAll(prefix, "\\", "/"))
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return ""
	}
	prefix = strings.TrimPrefix(path.Clean("/"+prefix), "/")
	if prefix == "." {
		return ""
	}
	return prefix
}

func cleanObjectPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.Trim(value, "/")
	if value == "" {
		return ""
	}
	segments := strings.Split(value, "/")
	cleaned := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = safeObjectKeyPart(segment)
		if segment != "" && segment != "." {
			cleaned = append(cleaned, segment)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	return path.Join(cleaned...)
}

func isFinanceRootPrefix(prefix string) bool {
	prefix = strings.Trim(cleanObjectPrefix(prefix), "/")
	return prefix == defaultFinanceOSSPrefix
}

func inferFinanceYear(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if match := regexp.MustCompile(`20\d{2}`).FindString(name); match != "" {
		return match
	}
	matches := regexp.MustCompile(`(?:^|[^0-9])(\d{2})年`).FindStringSubmatch(name)
	if len(matches) == 2 {
		return "20" + matches[1]
	}
	return ""
}

func objectKeyWithHashSuffix(key, hash string) string {
	key = cleanObjectPrefix(key)
	hash = strings.TrimSpace(strings.ToLower(hash))
	if len(hash) > 12 {
		hash = hash[:12]
	}
	if hash == "" {
		hash = "unknown"
	}
	ext := path.Ext(key)
	base := strings.TrimSuffix(key, ext)
	return base + ".sha256-" + hash + ext
}

func objectKeyFromStorageKey(storageKey string) string {
	storageKey = strings.TrimSpace(storageKey)
	if strings.HasPrefix(storageKey, "s3://") {
		rest := strings.TrimPrefix(storageKey, "s3://")
		if _, key, ok := strings.Cut(rest, "/"); ok {
			return cleanObjectPrefix(key)
		}
		return ""
	}
	return cleanObjectPrefix(storageKey)
}
