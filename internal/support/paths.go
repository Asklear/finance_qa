package support

import (
	"os"
	"path/filepath"
	"strings"
)

func DefaultCompanyName() string {
	return strings.TrimSpace(os.Getenv("FINANCEQA_DEFAULT_COMPANY"))
}

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

func FindProjectRoot() string {
	curr, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(curr, "go.mod")); err == nil {
			return curr
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	// Fallback to CWD if no go.mod found
	wd, _ := os.Getwd()
	return wd
}

func DefaultDBPath(root string) string {
	_ = root

	if explicit := strings.TrimSpace(os.Getenv("FINANCEQA_DB")); explicit != "" {
		return explicit
	}
	if dsn := strings.TrimSpace(os.Getenv("FINANCEQA_PG_DSN")); dsn != "" {
		return dsn
	}
	host := strings.TrimSpace(os.Getenv("PGHOST"))
	port := strings.TrimSpace(os.Getenv("PGPORT"))
	user := strings.TrimSpace(os.Getenv("PGUSER"))
	pass := strings.TrimSpace(os.Getenv("PGPASSWORD"))
	dbname := strings.TrimSpace(os.Getenv("PGDATABASE"))
	if host != "" && user != "" && dbname != "" {
		if port == "" {
			port = "5432"
		}
		schema := strings.TrimSpace(os.Getenv("FINANCEQA_PG_SCHEMA"))
		dsn := "host=" + host + " port=" + port + " user=" + user + " password=" + pass + " dbname=" + dbname
		if schema != "" {
			dsn += " search_path=" + schema + ",public"
		}
		return dsn
	}
	return ""
}

func DefaultUserConfigPath(root string) string {
	if root == "" {
		root = FindProjectRoot()
	}
	return filepath.Join(root, "config", "user_preferences.yaml")
}

func DefaultKeywordsPath(root string) string {
	if root == "" {
		root = FindProjectRoot()
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
