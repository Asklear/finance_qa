package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateBearerTokenRejectsEmptyConfiguredToken(t *testing.T) {
	t.Parallel()

	if ValidateBearerToken("Bearer anything", "") {
		t.Fatalf("ValidateBearerToken should reject empty configured token")
	}
}

func TestValidateBearerTokenRejectsMissingHeader(t *testing.T) {
	t.Parallel()

	if ValidateBearerToken("", "configured-token") {
		t.Fatalf("ValidateBearerToken should reject missing bearer header")
	}
}

func TestValidateBearerTokenAcceptsMatchingToken(t *testing.T) {
	t.Parallel()

	if !ValidateBearerToken("Bearer configured-token", "configured-token") {
		t.Fatalf("ValidateBearerToken should accept matching bearer token")
	}
}

func TestValidateBearerTokenRejectsWrongToken(t *testing.T) {
	t.Parallel()

	if ValidateBearerToken("Bearer wrong-token", "configured-token") {
		t.Fatalf("ValidateBearerToken should reject wrong bearer token")
	}
}

func TestExtractBearerTokenTrimsBearerValue(t *testing.T) {
	t.Parallel()

	if got := ExtractBearerToken("Bearer  configured-token  "); got != "configured-token" {
		t.Fatalf("ExtractBearerToken = %q, want configured-token", got)
	}
	if got := ExtractBearerToken("Token configured-token"); got != "" {
		t.Fatalf("ExtractBearerToken should reject non-bearer header, got %q", got)
	}
}

func TestLoadTokenFileTrimsWhitespaceAndRejectsEmpty(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte(" configured-token \n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	token, err := LoadTokenFile(tokenPath)
	if err != nil {
		t.Fatalf("LoadTokenFile returned error: %v", err)
	}
	if token != "configured-token" {
		t.Fatalf("LoadTokenFile = %q, want configured-token", token)
	}

	emptyPath := filepath.Join(t.TempDir(), "empty-token")
	if err := os.WriteFile(emptyPath, []byte("\n"), 0o600); err != nil {
		t.Fatalf("write empty token: %v", err)
	}
	if _, err := LoadTokenFile(emptyPath); err == nil {
		t.Fatalf("LoadTokenFile should reject empty token files")
	}
}

func TestRedactAuthorizationHidesBearerValue(t *testing.T) {
	t.Parallel()

	redacted := RedactAuthorization("Bearer very-secret-token")
	if strings.Contains(redacted, "very-secret-token") {
		t.Fatalf("RedactAuthorization leaked token: %q", redacted)
	}
	if redacted != "Bearer <redacted>" {
		t.Fatalf("RedactAuthorization = %q, want Bearer <redacted>", redacted)
	}
}

func TestScopeAllowsReadOnlyTools(t *testing.T) {
	t.Parallel()

	for _, tool := range []string{"finance-query", "finance-host-data"} {
		if !ScopeAllowsTool(ScopeRead, tool, map[string]any{}) {
			t.Fatalf("read scope should allow %s", tool)
		}
	}
	if !ScopeAllowsTool(ScopeRead, "finance-dimensions", map[string]any{"action": "list"}) {
		t.Fatalf("read scope should allow finance-dimensions list")
	}
}

func TestScopeRejectsWriteToolForReadToken(t *testing.T) {
	t.Parallel()

	for _, tool := range []string{"finance-upload", "finance-sync"} {
		if ScopeAllowsTool(ScopeRead, tool, map[string]any{}) {
			t.Fatalf("read scope should reject %s", tool)
		}
	}
	if ScopeAllowsTool(ScopeRead, "finance-dimensions", map[string]any{"action": "seed-standard"}) {
		t.Fatalf("read scope should reject write-like finance-dimensions actions")
	}
}

func TestScopeAdminAllowsAllFinanceTools(t *testing.T) {
	t.Parallel()

	for _, tool := range []string{"finance-query", "finance-host-data", "finance-upload", "finance-sync", "finance-dimensions"} {
		if !ScopeAllowsTool(ScopeAdmin, tool, map[string]any{"action": "seed-standard"}) {
			t.Fatalf("admin scope should allow %s", tool)
		}
	}
}

func TestFinanceToolsForScopeFiltersWriteToolsFromReadScope(t *testing.T) {
	t.Parallel()

	tools := financeToolsForScope(ScopeRead)
	byName := map[string]bool{}
	for _, tool := range tools {
		byName[tool.Name] = true
	}
	for _, allowed := range []string{"finance-query", "finance-host-data", "finance-dimensions"} {
		if !byName[allowed] {
			t.Fatalf("read scope tools missing %s: %#v", allowed, tools)
		}
	}
	for _, denied := range []string{"finance-upload", "finance-sync"} {
		if byName[denied] {
			t.Fatalf("read scope tools should exclude %s: %#v", denied, tools)
		}
	}
}
