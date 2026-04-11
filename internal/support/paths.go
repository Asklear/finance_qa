package support

import (
	"os"
	"path/filepath"
)

func CurrentWorkingDirectory() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func EnsureParentDir(filePath string) error {
	dir := filepath.Dir(filePath)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func DefaultDBPath(root string) string {
	if root == "" {
		root = CurrentWorkingDirectory()
	}
	return filepath.Join(root, "finance.db")
}

func DefaultUserConfigPath(root string) string {
	if root == "" {
		root = CurrentWorkingDirectory()
	}
	return filepath.Join(root, "config", "user_preferences.yaml")
}

func DefaultKeywordsPath(root string) string {
	if root == "" {
		root = CurrentWorkingDirectory()
	}

	candidates := []string{
		filepath.Join(root, "config", "query_keywords.json"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return candidates[len(candidates)-1]
}
