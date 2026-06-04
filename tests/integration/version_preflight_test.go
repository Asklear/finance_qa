package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionPreflightFailsWhenRuntimeChangesWithoutVersionBump(t *testing.T) {
	t.Parallel()

	scriptPath := filepath.Join("..", "..", "tests", "scripts", "check_version_preflight.sh")
	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"VERSION_PRECHECK_CHANGED_FILES=internal/query/example.go",
		"VERSION_PRECHECK_SKIP_GIT=1",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("version preflight should fail when runtime files changed without a version bump\n%s", out)
	}
	if !strings.Contains(string(out), "runtime changed but version was not bumped") {
		t.Fatalf("version preflight failure should explain missing bump, got:\n%s", out)
	}
}

func TestVersionPreflightPassesWhenRuntimeAndVersionBothChange(t *testing.T) {
	t.Parallel()

	scriptPath := filepath.Join("..", "..", "tests", "scripts", "check_version_preflight.sh")
	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"VERSION_PRECHECK_CHANGED_FILES=internal/query/example.go\ninternal/buildinfo/version.go\nplugin/openclaw-finance/package.json\nplugin/openclaw-finance/openclaw.plugin.json",
		"VERSION_PRECHECK_SKIP_GIT=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version preflight should pass when runtime files and canonical version files changed: %v\n%s", err, out)
	}
}

func TestVersionPreflightIgnoresGoTestOnlyChanges(t *testing.T) {
	t.Parallel()

	scriptPath := filepath.Join("..", "..", "tests", "scripts", "check_version_preflight.sh")
	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"VERSION_PRECHECK_CHANGED_FILES=internal/query/example_test.go",
		"VERSION_PRECHECK_SKIP_GIT=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version preflight should not require a bump for Go test-only changes: %v\n%s", err, out)
	}
}

func TestBumpVersionScriptUpdatesAllVersionSurfaces(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	tmp := t.TempDir()
	for _, rel := range []string{
		"internal/buildinfo/version.go",
		"internal/mcp/server_test.go",
		"plugin/openclaw-finance/package.json",
		"plugin/openclaw-finance/openclaw.plugin.json",
		"plugin/openclaw-finance/server/README.md",
		"docs/architecture/03-deployment-runtime.md",
		"tests/integration/openclaw_finance_plugin_test.go",
		"tests/scripts/check_version_preflight.sh",
		"tests/scripts/bump_version.sh",
	} {
		copyTestFile(t, filepath.Join(root, rel), filepath.Join(tmp, rel))
	}

	bumpPath := filepath.Join(tmp, "tests", "scripts", "bump_version.sh")
	cmd := exec.Command("bash", bumpPath, "9.8.7")
	cmd.Env = append(os.Environ(), "ROOT_DIR="+tmp, "VERSION_PRECHECK_SKIP_DIFF=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bump version script should update copied version surfaces: %v\n%s", err, out)
	}

	for _, rel := range []string{
		"internal/buildinfo/version.go",
		"internal/mcp/server_test.go",
		"plugin/openclaw-finance/package.json",
		"plugin/openclaw-finance/openclaw.plugin.json",
		"plugin/openclaw-finance/server/README.md",
		"docs/architecture/03-deployment-runtime.md",
		"tests/integration/openclaw_finance_plugin_test.go",
	} {
		raw, err := os.ReadFile(filepath.Join(tmp, rel))
		if err != nil {
			t.Fatalf("read bumped file %s: %v", rel, err)
		}
		if !strings.Contains(string(raw), "9.8.7") {
			t.Fatalf("bumped file %s does not contain new version", rel)
		}
	}
	docsRaw, err := os.ReadFile(filepath.Join(tmp, "docs", "architecture", "03-deployment-runtime.md"))
	if err != nil {
		t.Fatalf("read bumped deployment doc: %v", err)
	}
	if !strings.Contains(string(docsRaw), "http://127.0.0.1:8787/feishu/oauth/callback") {
		t.Fatalf("bump script should not rewrite loopback URLs in deployment docs")
	}

	checkPath := filepath.Join(tmp, "tests", "scripts", "check_version_preflight.sh")
	check := exec.Command("bash", checkPath)
	check.Env = append(os.Environ(), "ROOT_DIR="+tmp, "VERSION_PRECHECK_SKIP_DIFF=1")
	out, err = check.CombinedOutput()
	if err != nil {
		t.Fatalf("bumped temp repo should pass version preflight: %v\n%s", err, out)
	}
}

func TestSyncScriptRunsVersionPreflightBeforeDeploying(t *testing.T) {
	t.Parallel()

	syncScriptPath := filepath.Join("..", "..", "tests", "scripts", "sync_openclaw_bridge_and_skill.sh")
	raw, err := os.ReadFile(syncScriptPath)
	if err != nil {
		t.Fatalf("read sync script: %v", err)
	}
	script := string(raw)
	for _, want := range []string{
		`LOCAL_VERSION_PREFLIGHT="$ROOT_DIR/tests/scripts/check_version_preflight.sh"`,
		`VERSION_PREFLIGHT_ENABLED="${VERSION_PREFLIGHT_ENABLED:-1}"`,
		`"$LOCAL_VERSION_PREFLIGHT"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("sync script should run version preflight before deployment; missing %q", want)
		}
	}
}

func copyTestFile(t *testing.T, src, dst string) {
	t.Helper()

	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read source file %s: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("create dir for %s: %v", dst, err)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatalf("stat source file %s: %v", src, err)
	}
	if err := os.WriteFile(dst, raw, info.Mode().Perm()); err != nil {
		t.Fatalf("write copied file %s: %v", dst, err)
	}
}
