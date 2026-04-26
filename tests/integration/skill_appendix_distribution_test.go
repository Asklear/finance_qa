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
	appendixPath := filepath.Join("..", "..", "docs", "SKILL_APPENDIX_FULL.md")

	rootSkill, err := os.ReadFile(rootSkillPath)
	if err != nil {
		t.Fatalf("read root skill: %v", err)
	}
	if !strings.Contains(string(rootSkill), "`docs/SKILL_APPENDIX_FULL.md`") {
		t.Fatalf("root skill should reference appendix via docs/SKILL_APPENDIX_FULL.md")
	}
	if !strings.Contains(string(rootSkill), "MUST call `finance-query`") {
		t.Fatalf("root skill description should force finance-query for finance questions before the model reads SKILL.md")
	}
	if !strings.Contains(string(rootSkill), "`final_answer` unchanged") {
		t.Fatalf("root skill description should force hosts to return final_answer unchanged")
	}
	if !strings.Contains(string(rootSkill), `"openclaw": { "always": true }`) {
		t.Fatalf("root skill should set metadata.openclaw.always=true so OpenClaw injects finance rules for generic finance questions")
	}
	if _, err := os.Stat(appendixPath); err != nil {
		t.Fatalf("appendix file should exist: %v", err)
	}
}

func TestClaudeInstructionsForceBridgeFinalAnswer(t *testing.T) {
	t.Parallel()

	claudePath := filepath.Join("..", "..", "CLAUDE.md")
	claude, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	text := string(claude)
	for _, want := range []string{
		"必须先调用 bridge",
		"finance_bridge.py",
		"`final_answer` 原样返回",
		"不能摘要、改写、换算或省略来源",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("CLAUDE.md should force bridge final_answer usage; missing %q", want)
		}
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
	if !strings.Contains(scriptText, "cp docs/SKILL_APPENDIX_FULL.md \"$OUTPUT_DIR/docs/\"") {
		t.Fatalf("build script should package skill appendix under docs/")
	}
	if !strings.Contains(scriptText, "mkdir -p \"$OUTPUT_DIR/skills/finance/docs\"") {
		t.Fatalf("build script should create OpenClaw extension skill docs directory")
	}
	if !strings.Contains(scriptText, "cp SKILL.md \"$OUTPUT_DIR/skills/finance/SKILL.md\"") {
		t.Fatalf("build script should package SKILL.md under skills/finance/")
	}
	if !strings.Contains(scriptText, "cp docs/SKILL_APPENDIX_FULL.md \"$OUTPUT_DIR/skills/finance/docs/\"") {
		t.Fatalf("build script should package appendix under skills/finance/docs/")
	}
	if !strings.Contains(scriptText, "mkdir -p \"$OUTPUT_DIR/plugin/openclaw-finance/dist\"") {
		t.Fatalf("build script should create OpenClaw finance plugin runtime directory")
	}
	if !strings.Contains(scriptText, "cp plugin/openclaw-finance/dist/index.esm.js \"$OUTPUT_DIR/plugin/openclaw-finance/dist/\"") {
		t.Fatalf("build script should package OpenClaw finance plugin runtime")
	}
	if !strings.Contains(scriptText, "cp plugin/openclaw-finance/index.ts \"$OUTPUT_DIR/plugin/openclaw-finance/\"") {
		t.Fatalf("build script should package OpenClaw finance plugin entrypoint")
	}
	if !strings.Contains(scriptText, "cp plugin/openclaw-finance/openclaw.plugin.json \"$OUTPUT_DIR/plugin/openclaw-finance/\"") {
		t.Fatalf("build script should package OpenClaw finance plugin manifest")
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
	if !strings.Contains(scriptText, "LOCAL_APPENDIX=\"$ROOT_DIR/docs/SKILL_APPENDIX_FULL.md\"") {
		t.Fatalf("sync script should define local appendix path")
	}
	if !strings.Contains(scriptText, "REMOTE_OPENCLAW_EXT_SKILL_DIR=\"${REMOTE_OPENCLAW_EXT_SKILL_DIR:-/root/.openclaw/extensions/openclaw-finance/skills/finance}\"") {
		t.Fatalf("sync script should target OpenClaw extension skill directory")
	}
	if !strings.Contains(scriptText, "$SERVER:$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md") {
		t.Fatalf("sync script should upload appendix into remote repo docs path")
	}
	if !strings.Contains(scriptText, "mkdir -p '$REMOTE_OPENCLAW_SKILL_DIR/docs';") {
		t.Fatalf("sync script should provision docs directory inside OpenClaw skill dir")
	}
	if !strings.Contains(scriptText, "mkdir -p '$REMOTE_OPENCLAW_EXT_SKILL_DIR/docs';") {
		t.Fatalf("sync script should provision docs directory inside OpenClaw extension skill dir")
	}
	if !strings.Contains(scriptText, "rm -rf '$REMOTE_OPENCLAW_SKILL_DIR' '$REMOTE_OPENCLAW_EXT_SKILL_DIR';") {
		t.Fatalf("sync script should remove stale skill symlinks before writing real OpenClaw skill directories")
	}
	if !strings.Contains(scriptText, "cp '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md';") {
		t.Fatalf("sync script should copy root skill into OpenClaw skill path")
	}
	if !strings.Contains(scriptText, "cp '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_OPENCLAW_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md';") {
		t.Fatalf("sync script should copy appendix into OpenClaw skill docs path")
	}
	if !strings.Contains(scriptText, "cp '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_OPENCLAW_EXT_SKILL_DIR/SKILL.md';") {
		t.Fatalf("sync script should copy root skill into OpenClaw extension skill path")
	}
	if !strings.Contains(scriptText, "cp '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_OPENCLAW_EXT_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md';") {
		t.Fatalf("sync script should copy appendix into OpenClaw extension skill docs path")
	}
	if !strings.Contains(scriptText, "skills.load.extraDirs") {
		t.Fatalf("sync script should register finance skill under skills.load.extraDirs so runtime prompt ordering includes it")
	}
	if !strings.Contains(scriptText, "const financeSkillDir = process.argv[2];") {
		t.Fatalf("sync script should pass the published finance skill dir into the OpenClaw config patch")
	}
	if !strings.Contains(scriptText, "cfg.skills.load.extraDirs = [financeSkillDir, ...existing.filter((dir) => dir !== financeSkillDir)];") {
		t.Fatalf("sync script should keep finance skill dir first in skills.load.extraDirs")
	}
	if !strings.Contains(scriptText, "delete entry.skillsSnapshot;") {
		t.Fatalf("sync script should clear stale OpenClaw session skillsSnapshot caches after publishing finance skill")
	}
	if strings.Contains(scriptText, "ln -sfn '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md';") ||
		strings.Contains(scriptText, "ln -sfn '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_OPENCLAW_EXT_SKILL_DIR/SKILL.md';") {
		t.Fatalf("sync script should not publish OpenClaw skill files as symlinks outside configured roots")
	}
	if strings.Contains(scriptText, "DEFAULT_SKILL_CANDIDATES") {
		t.Fatalf("sync script should not rely on removed DEFAULT_SKILL_CANDIDATES marker")
	}
}
