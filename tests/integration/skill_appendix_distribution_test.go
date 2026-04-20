package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootSkillReferencesAppendixViaStableRelativePath(t *testing.T) {
	t.Parallel()

	rootSkillPath := filepath.Join("..", "..", "SKILL.md")
	appendixPath := filepath.Join("..", "..", "docs", "SKILL_APPENDIX_FULL_2026-04-15.md")

	rootSkill, err := os.ReadFile(rootSkillPath)
	if err != nil {
		t.Fatalf("read root skill: %v", err)
	}
	if !strings.Contains(string(rootSkill), "`docs/SKILL_APPENDIX_FULL_2026-04-15.md`") {
		t.Fatalf("root skill should reference appendix via docs/SKILL_APPENDIX_FULL_2026-04-15.md")
	}
	if _, err := os.Stat(appendixPath); err != nil {
		t.Fatalf("appendix file should exist: %v", err)
	}
}

func TestBuildScriptPackagesAppendixAlongsideSkill(t *testing.T) {
	t.Parallel()

	buildScriptPath := filepath.Join("..", "..", "tests", "scripts", "build_openclaw_package.sh")
	buildScript, err := os.ReadFile(buildScriptPath)
	if err != nil {
		t.Fatalf("read build script: %v", err)
	}
	scriptText := string(buildScript)
	if !strings.Contains(scriptText, "mkdir -p \"$OUTPUT_DIR/docs\"") {
		t.Fatalf("build script should create docs directory for packaged appendix")
	}
	if !strings.Contains(scriptText, "cp docs/SKILL_APPENDIX_FULL_2026-04-15.md \"$OUTPUT_DIR/docs/\"") {
		t.Fatalf("build script should package skill appendix under docs/")
	}
}

func TestSyncScriptPublishesAppendixForOpenClawSkillDir(t *testing.T) {
	t.Parallel()

	syncScriptPath := filepath.Join("..", "..", "tests", "scripts", "sync_openclaw_bridge_and_skill.sh")
	syncScript, err := os.ReadFile(syncScriptPath)
	if err != nil {
		t.Fatalf("read sync script: %v", err)
	}
	scriptText := string(syncScript)
	if !strings.Contains(scriptText, "LOCAL_APPENDIX=\"$ROOT_DIR/docs/SKILL_APPENDIX_FULL_2026-04-15.md\"") {
		t.Fatalf("sync script should define local appendix path")
	}
	if !strings.Contains(scriptText, "$SERVER:$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL_2026-04-15.md") {
		t.Fatalf("sync script should upload appendix into remote repo docs path")
	}
	if !strings.Contains(scriptText, "mkdir -p '$REMOTE_OPENCLAW_SKILL_DIR/docs';") {
		t.Fatalf("sync script should provision docs directory inside OpenClaw skill dir")
	}
	if !strings.Contains(scriptText, "ln -sfn '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL_2026-04-15.md' '$REMOTE_OPENCLAW_SKILL_DIR/docs/SKILL_APPENDIX_FULL_2026-04-15.md';") {
		t.Fatalf("sync script should symlink appendix into OpenClaw skill docs path")
	}
	if strings.Contains(scriptText, "DEFAULT_SKILL_CANDIDATES") {
		t.Fatalf("sync script should not rely on removed DEFAULT_SKILL_CANDIDATES marker")
	}
}
