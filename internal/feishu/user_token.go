package feishu

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func loadUserToken(path string) (UserToken, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return UserToken{}, fmt.Errorf("feishu user token file is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return UserToken{}, fmt.Errorf("read feishu user token file: %w", err)
	}
	var token UserToken
	if err := json.Unmarshal(data, &token); err != nil {
		return UserToken{}, fmt.Errorf("decode feishu user token file: %w", err)
	}
	return token, nil
}

func SaveUserToken(path string, token UserToken) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("feishu user token file is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create feishu user token dir: %w", err)
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("encode feishu user token file: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write feishu user token file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace feishu user token file: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	return nil
}
