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
	if !strings.Contains(string(rootSkill), "必须先调用 `finance-query`") {
		t.Fatalf("root skill description should force finance-query for finance questions before the model reads SKILL.md")
	}
	if !strings.Contains(string(rootSkill), "关键金额、期间、业务口径和来源说明") {
		t.Fatalf("root skill description should force hosts to preserve current Go MCP business facts without exact-output passthrough")
	}
	if strings.Contains(string(rootSkill), "`final_answer` unchanged") {
		t.Fatalf("root skill description should not force exact final_answer passthrough")
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
		"必须先调用 Go MCP",
		"financeqa serve",
		"必须保留 Go MCP 结果里的关键数值、期间、业务口径、来源和来源更新时间",
		"可以按老板汇报风格重写措辞",
		"不能用历史对话、记忆、旧答案",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("CLAUDE.md should force Go MCP fact preservation without exact-output passthrough; missing %q", want)
		}
	}
	for _, reject := range []string{
		"`final_answer` 原样返回",
		"不能摘要、改写、换算或省略来源",
	} {
		if strings.Contains(text, reject) {
			t.Fatalf("CLAUDE.md should not force exact final_answer passthrough; found %q", reject)
		}
	}
	if strings.Contains(text, "finance_bridge.py") {
		t.Fatalf("CLAUDE.md should not point users at the retired Python bridge")
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
	if !strings.Contains(scriptText, "mkdir -p \"$OUTPUT_DIR/plugin/openclaw-finance/dist\"") {
		t.Fatalf("build script should create OpenClaw finance plugin runtime directory")
	}
	if !strings.Contains(scriptText, "cp plugin/openclaw-finance/dist/index.esm.js \"$OUTPUT_DIR/plugin/openclaw-finance/dist/\"") {
		t.Fatalf("build script should package OpenClaw finance plugin runtime")
	}
	if !strings.Contains(scriptText, "cp plugin/openclaw-finance/openclaw.plugin.json \"$OUTPUT_DIR/plugin/openclaw-finance/\"") {
		t.Fatalf("build script should package OpenClaw finance plugin manifest")
	}
	if strings.Contains(scriptText, "cp plugin/openclaw-finance/index.ts") {
		t.Fatalf("build script should not package plugin source entrypoint")
	}
	if strings.Contains(scriptText, "skills/finance") {
		t.Fatalf("build script should not package a redundant skill inside the OpenClaw extension")
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
	if !strings.Contains(scriptText, "REMOTE_OPENCLAW_EXT_SKILL_DIR=\"${REMOTE_OPENCLAW_EXT_SKILL_DIR:-$REMOTE_HOME/.openclaw/extensions/openclaw-finance/skills/finance}\"") {
		t.Fatalf("sync script should track stale OpenClaw extension skill directory for cleanup")
	}
	if !strings.Contains(scriptText, "REMOTE_CLAUDE_SKILL_DIR=\"${REMOTE_CLAUDE_SKILL_DIR:-$REMOTE_HOME/.claude/skills/finance}\"") {
		t.Fatalf("sync script should target Claude Code skill directory")
	}
	if !strings.Contains(scriptText, "$SERVER:$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md") {
		t.Fatalf("sync script should upload appendix into remote repo docs path")
	}
	if !strings.Contains(scriptText, `REMOTE_OPENCLAW_SKILL_PARENT="$(dirname "$REMOTE_OPENCLAW_SKILL_DIR")"`) {
		t.Fatalf("sync script should derive the OpenClaw skill parent for directory-level symlink publishing")
	}
	if !strings.Contains(scriptText, `REMOTE_CLAUDE_SKILL_PARENT="$(dirname "$REMOTE_CLAUDE_SKILL_DIR")"`) {
		t.Fatalf("sync script should derive the Claude skill parent for directory-level symlink publishing")
	}
	if !strings.Contains(scriptText, "mkdir -p '$REMOTE_OPENCLAW_SKILL_PARENT' '$REMOTE_CLAUDE_SKILL_PARENT';") {
		t.Fatalf("sync script should provision skill parent directories before directory symlinks")
	}
	if !strings.Contains(scriptText, "rm -rf '$REMOTE_OPENCLAW_SKILL_DIR' '$REMOTE_CLAUDE_SKILL_DIR' '$REMOTE_OPENCLAW_EXT_SKILL_DIR';") {
		t.Fatalf("sync script should remove stale file-level or copied skill dirs before relinking")
	}
	if !strings.Contains(scriptText, "ln -sfn '$REMOTE_REPO_DIR' '$REMOTE_OPENCLAW_SKILL_DIR';") {
		t.Fatalf("sync script should symlink the OpenClaw skill directory to the repo root")
	}
	if !strings.Contains(scriptText, "ln -sfn '$REMOTE_REPO_DIR' '$REMOTE_CLAUDE_SKILL_DIR';") {
		t.Fatalf("sync script should symlink the Claude skill directory to the repo root")
	}
	if !strings.Contains(scriptText, "skills.load.extraDirs") {
		t.Fatalf("sync script should verify finance skill is present under skills.load.extraDirs")
	}
	if !strings.Contains(scriptText, "const financeSkillDir = process.argv[2];") {
		t.Fatalf("sync script should pass the published finance skill dir into the OpenClaw config check")
	}
	if !strings.Contains(scriptText, "extraDirs.includes(financeSkillDir)") {
		t.Fatalf("sync script should verify existing OpenClaw config includes the finance skill dir")
	}
	if strings.Contains(scriptText, "cfg.skills.load.extraDirs = [financeSkillDir, ...existing.filter((dir) => dir !== financeSkillDir)];") {
		t.Fatalf("sync script should not rewrite skills.load.extraDirs when existing OpenClaw config is usable")
	}
	if !strings.Contains(scriptText, "delete entry.skillsSnapshot;") {
		t.Fatalf("sync script should clear stale OpenClaw session skillsSnapshot caches after publishing finance skill")
	}
	if !strings.Contains(scriptText, "test -L '$REMOTE_OPENCLAW_SKILL_DIR';") ||
		!strings.Contains(scriptText, "test -L '$REMOTE_CLAUDE_SKILL_DIR';") ||
		!strings.Contains(scriptText, `test \"\$(readlink -f '$REMOTE_OPENCLAW_SKILL_DIR')\" = '$REMOTE_REPO_DIR';`) ||
		!strings.Contains(scriptText, `test \"\$(readlink -f '$REMOTE_CLAUDE_SKILL_DIR')\" = '$REMOTE_REPO_DIR';`) {
		t.Fatalf("sync script should verify directory-level OpenClaw and Claude skill symlinks")
	}
	if strings.Contains(scriptText, "cp '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md';") ||
		strings.Contains(scriptText, "cp '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_OPENCLAW_EXT_SKILL_DIR/SKILL.md';") ||
		strings.Contains(scriptText, "cp '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_CLAUDE_SKILL_DIR/SKILL.md';") {
		t.Fatalf("sync script should not copy published skill files; use repo-backed symlinks")
	}
	if strings.Contains(scriptText, "ln -sfn '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_OPENCLAW_EXT_SKILL_DIR/SKILL.md';") ||
		strings.Contains(scriptText, "ln -sfn '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_OPENCLAW_EXT_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md';") ||
		strings.Contains(scriptText, "mkdir -p '$REMOTE_OPENCLAW_EXT_SKILL_DIR/docs';") ||
		strings.Contains(scriptText, "ln -sfn '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_OPENCLAW_SKILL_DIR/SKILL.md';") ||
		strings.Contains(scriptText, "ln -sfn '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_OPENCLAW_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md';") ||
		strings.Contains(scriptText, "ln -sfn '$REMOTE_REPO_DIR/SKILL.md' '$REMOTE_CLAUDE_SKILL_DIR/SKILL.md';") ||
		strings.Contains(scriptText, "ln -sfn '$REMOTE_REPO_DIR/docs/SKILL_APPENDIX_FULL.md' '$REMOTE_CLAUDE_SKILL_DIR/docs/SKILL_APPENDIX_FULL.md';") {
		t.Fatalf("sync script should not publish a redundant skill inside the OpenClaw extension")
	}
	if strings.Contains(scriptText, "DEFAULT_SKILL_CANDIDATES") {
		t.Fatalf("sync script should not rely on removed DEFAULT_SKILL_CANDIDATES marker")
	}
}
