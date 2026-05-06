package support

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv reads KEY=VALUE pairs from file and sets env vars only when unset.
// Missing file is treated as no-op.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		val := strings.TrimSpace(v)
		val = strings.Trim(val, `"'`)
		_ = os.Setenv(key, val)
	}
	return scanner.Err()
}

// LoadAppDotEnv loads a repository-local .env plus an optional FINANCEQA_ENV_FILE.
// The explicit FINANCEQA_ENV_FILE is loaded first so it can override the local file.
func LoadAppDotEnv(root string) error {
	explicit := strings.TrimSpace(os.Getenv("FINANCEQA_ENV_FILE"))
	if explicit != "" {
		_ = LoadDotEnv(explicit)
	}
	if strings.TrimSpace(root) != "" {
		_ = LoadDotEnv(filepath.Join(root, ".env"))
	}
	return nil
}
