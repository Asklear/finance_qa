package support

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvParsesAssignmentsAndPreservesExistingEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	payload := `
# comment
export FINANCEQA_TEST_DOTENV_A = "from-file"
FINANCEQA_TEST_DOTENV_B='quoted'
FINANCEQA_TEST_DOTENV_EXISTING=from-file
NO_EQUALS
=bad
`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write dotenv: %v", err)
	}

	t.Setenv("FINANCEQA_TEST_DOTENV_EXISTING", "already-set")
	t.Cleanup(func() {
		_ = os.Unsetenv("FINANCEQA_TEST_DOTENV_A")
		_ = os.Unsetenv("FINANCEQA_TEST_DOTENV_B")
	})

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("load dotenv: %v", err)
	}
	if got := os.Getenv("FINANCEQA_TEST_DOTENV_A"); got != "from-file" {
		t.Fatalf("dotenv A = %q, want from-file", got)
	}
	if got := os.Getenv("FINANCEQA_TEST_DOTENV_B"); got != "quoted" {
		t.Fatalf("dotenv B = %q, want quoted", got)
	}
	if got := os.Getenv("FINANCEQA_TEST_DOTENV_EXISTING"); got != "already-set" {
		t.Fatalf("existing env = %q, want already-set", got)
	}
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatalf("missing dotenv should be ignored: %v", err)
	}
}
