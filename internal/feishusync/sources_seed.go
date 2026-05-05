package feishusync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	feishuSyncSourcesJSONEnv = "FEISHU_SYNC_SOURCES_JSON"
	feishuSyncSourcesFileEnv = "FEISHU_SYNC_SOURCES_FILE"
)

// DefaultSources loads Feishu sources from deployment configuration.
//
// Keep source tokens out of code: folder/workbook tokens are tenant-specific
// operational config, even though they are not API secrets.
func DefaultSources() ([]SyncSource, error) {
	raw, source, err := configuredSourcesPayload()
	if err != nil {
		return nil, err
	}
	sources, err := parseConfiguredSources(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", source, err)
	}
	return sources, nil
}

func configuredSourcesPayload() (string, string, error) {
	if filePath := strings.TrimSpace(os.Getenv(feishuSyncSourcesFileEnv)); filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", feishuSyncSourcesFileEnv, err
		}
		return string(data), feishuSyncSourcesFileEnv, nil
	}
	if value := strings.TrimSpace(os.Getenv(feishuSyncSourcesJSONEnv)); value != "" {
		return value, feishuSyncSourcesJSONEnv, nil
	}
	return "", "", fmt.Errorf("%s or %s is required to seed Feishu sources", feishuSyncSourcesJSONEnv, feishuSyncSourcesFileEnv)
}

type configuredSyncSource struct {
	ID              int64           `json:"id"`
	SourceType      string          `json:"source_type"`
	SourceToken     string          `json:"source_token"`
	SourceURL       string          `json:"source_url"`
	DisplayName     string          `json:"display_name"`
	ParentToken     string          `json:"parent_token"`
	SyncMode        string          `json:"sync_mode"`
	SyncStatus      string          `json:"sync_status"`
	LastRevision    string          `json:"last_revision"`
	LastContentHash string          `json:"last_content_hash"`
	ErrorMessage    string          `json:"error_message"`
	MetadataJSON    json.RawMessage `json:"metadata_json"`
}

func parseConfiguredSources(raw string) ([]SyncSource, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty source configuration")
	}
	if !strings.HasPrefix(raw, "[") {
		return nil, errors.New("source configuration must be a JSON array")
	}
	var configured []configuredSyncSource
	if err := json.Unmarshal([]byte(raw), &configured); err != nil {
		return nil, err
	}
	if len(configured) == 0 {
		return nil, errors.New("source configuration must contain at least one source")
	}
	sources := make([]SyncSource, 0, len(configured))
	seen := map[string]bool{}
	for idx, item := range configured {
		src, err := normalizeConfiguredSource(item)
		if err != nil {
			return nil, fmt.Errorf("source[%d]: %w", idx, err)
		}
		identity := src.SourceType + "\x00" + src.SourceToken
		if seen[identity] {
			return nil, fmt.Errorf("source[%d]: duplicate source_type/source_token", idx)
		}
		seen[identity] = true
		sources = append(sources, src)
	}
	return sources, nil
}

func normalizeConfiguredSource(item configuredSyncSource) (SyncSource, error) {
	src := SyncSource{
		ID:              item.ID,
		SourceType:      strings.TrimSpace(item.SourceType),
		SourceToken:     strings.TrimSpace(item.SourceToken),
		SourceURL:       strings.TrimSpace(item.SourceURL),
		DisplayName:     strings.TrimSpace(item.DisplayName),
		ParentToken:     strings.TrimSpace(item.ParentToken),
		SyncMode:        strings.TrimSpace(item.SyncMode),
		SyncStatus:      strings.TrimSpace(item.SyncStatus),
		LastRevision:    strings.TrimSpace(item.LastRevision),
		LastContentHash: strings.TrimSpace(item.LastContentHash),
		ErrorMessage:    strings.TrimSpace(item.ErrorMessage),
		MetadataJSON:    normalizeMetadataJSON(item.MetadataJSON),
	}
	if src.SourceType == "" {
		return SyncSource{}, errors.New("source_type is required")
	}
	switch src.SourceType {
	case SourceTypePDFFolder, SourceTypeFinanceWorkbook:
	default:
		return SyncSource{}, fmt.Errorf("unsupported source_type: %s", src.SourceType)
	}
	if src.SourceToken == "" {
		return SyncSource{}, errors.New("source_token is required")
	}
	if src.SyncMode == "" {
		src.SyncMode = "active_scan"
	}
	if src.SyncStatus == "" {
		src.SyncStatus = SyncStatusActive
	}
	return src, nil
}

func normalizeMetadataJSON(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		normalized, err := json.Marshal(value)
		if err != nil {
			return strings.TrimSpace(string(raw))
		}
		return string(normalized)
	}
}
