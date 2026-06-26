import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { main } from "../src/index.ts";
import { loadConfig } from "../src/config.ts";
import { generateCases } from "../src/cases.ts";

test("generic command-agent stub example runs through the agent-patrol CLI", async () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-command-stub-"));

  const code = await main([
    "run",
    "--config",
    "examples/stub.command-agent.yaml",
    "--suite",
    "smoke",
    "--seed",
    "fixed",
    "--out",
    outDir
  ]);

  assert.equal(code, 0);
  assert.match(fs.readFileSync(path.join(outDir, "summary.md"), "utf8"), /Accuracy: 100\.00%/);
});

test("live OpenClaw SSH runner fails closed unless explicitly enabled", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-live-runner-"));
  const questionFile = path.join(dir, "question.txt");
  fs.writeFileSync(questionFile, "请只回答：AGENT_PATROL_OK", "utf8");

  const result = spawnSync("node", [
    "examples/runners/openclaw_ssh_runner.mjs",
    "--host",
    "clawdbot",
    "--question-file",
    questionFile,
    "--session-id",
    "patrol-test"
  ], {
    cwd: process.cwd(),
    encoding: "utf8",
    env: { ...process.env, AGENT_PATROL_LIVE: "" }
  });

  assert.equal(result.status, 2);
  assert.match(result.stderr, /AGENT_PATROL_LIVE=1/);
});

test("live OpenClaw local runner fails closed unless explicitly enabled", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-live-local-runner-"));
  const questionFile = path.join(dir, "question.txt");
  fs.writeFileSync(questionFile, "请只回答：AGENT_PATROL_OK", "utf8");

  const result = spawnSync("node", [
    "examples/runners/openclaw_local_runner.mjs",
    "--question-file",
    questionFile,
    "--session-id",
    "patrol-test"
  ], {
    cwd: process.cwd(),
    encoding: "utf8",
    env: { ...process.env, AGENT_PATROL_LIVE: "" }
  });

  assert.equal(result.status, 2);
  assert.match(result.stderr, /AGENT_PATROL_LIVE=1/);
});

test("financeqa preset generates varied daily sample pool", () => {
  const config = loadConfig("presets/financeqa.yaml", {
    OPENCLAW_AGENT_CMD: "node examples/runners/openclaw_ssh_runner.mjs --host clawdbot --question-file {questionFile} --session-id {sessionId}",
    FINANCEQA_MCP_URL: "http://127.0.0.1/stub"
  });

  const cases = generateCases(config, { suite: "daily", seed: "2026-06-25" });
  const questions = cases.map((item) => item.question);

  assert.equal(cases.length, 8);
  assert.equal(new Set(questions).size, 8);
  assert.equal(questions.some((item) => item.includes("最新月份") || item.includes("最新可见月份")), true);
  assert.equal(questions.some((item) => item.includes("上一个完整自然月月底")), true);
  assert.equal(questions.some((item) => item.includes("应收未收")), true);
  assert.equal(questions.some((item) => item.includes("应付未付") || item.includes("未付款")), true);
  assert.equal(questions.some((item) => item.includes("已开票未回款") || item.includes("已开票未收款")), true);
  assert.equal(questions.some((item) => item.includes("已收票未付款")), true);
});

test("financeqa preset scores finance answers against FinanceQA MCP references", () => {
  const config = loadConfig("presets/financeqa.yaml", {
    OPENCLAW_AGENT_CMD: "node examples/runners/openclaw_ssh_runner.mjs --host clawdbot --question-file {questionFile} --session-id {sessionId}",
    FINANCEQA_MCP_URL: "http://127.0.0.1/stub"
  });

  for (const [name, template] of Object.entries(config.templates ?? {})) {
    const referenceChecks = template.scoring?.referenceChecks as Record<string, unknown> | undefined;
    assert.ok(referenceChecks, `${name} missing referenceChecks`);
    assert.equal(referenceChecks.periods, true, `${name} should compare reference periods`);
    assert.equal(referenceChecks.sources, true, `${name} should compare reference sources`);
    assert.notEqual(referenceChecks.perspectives, true, `${name} should not hard-fail reference boilerplate wording`);
  }
});

test("financeqa low-frequency dry-run schedule examples only write local reports", () => {
  const files = [
    "examples/cleanup/claude-session-cleanup.sh",
    "examples/cleanup/hermes-json-cleanup.sh",
    "examples/cleanup/openclaw-jsonl-cleanup.sh",
    "examples/cleanup/run-agent-cleanups.sh",
    "examples/schedules/README.md",
    "examples/schedules/financeqa-daily.env.example",
    "examples/schedules/financeqa-daily.cron.example",
    "examples/schedules/financeqa-daily.service",
    "examples/schedules/financeqa-daily.timer",
    "examples/schedules/run-financeqa-dry-run.sh"
  ];
  const contents = files.map((file) => fs.readFileSync(file, "utf8")).join("\n");
  const scriptMode = fs.statSync("examples/schedules/run-financeqa-dry-run.sh").mode;

  assert.match(contents, /presets\/financeqa\.yaml/);
  assert.match(contents, /--suite "\$\{AGENT_PATROL_SUITE:-smoke\}"/);
  assert.match(contents, /tmp\/financeqa-dry-run/);
  assert.match(contents, /AGENT_PATROL_LIVE=1/);
  assert.match(contents, /uuidgen|\/proc\/sys\/kernel\/random\/uuid/);
  assert.doesNotMatch(contents, /date \+%F-%H/);
  assert.match(contents, /OPENCLAW_AGENT_CMD="/);
  assert.match(contents, /FINANCEQA_MCP_READ_TOKEN_FILE/);
  assert.match(contents, /flock/);
  assert.match(contents, /AGENT_PATROL_CLEANUP_CMD/);
  assert.match(contents, /AGENT_PATROL_CLEANUP_KINDS/);
  assert.match(contents, /AGENT_PATROL_SESSION_RETENTION_DAYS/);
  assert.match(contents, /openclaw-jsonl-cleanup\.sh/);
  assert.match(contents, /hermes-json-cleanup\.sh/);
  assert.match(contents, /claude-session-cleanup\.sh/);
  assert.match(contents, /patrol-\*\.jsonl/);
  assert.match(contents, /patrol-\*\.json/);
  assert.match(contents, /09,17/);
  assert.doesNotMatch(contents, /09,13,18/);
  assert.match(contents, /openclaw-finance/);
  assert.match(contents, /non-production|非生产|dry-run/i);
  assert.doesNotMatch(contents, /--deliver/);
  assert.doesNotMatch(contents, /\blzh\b/);
  assert.notEqual(scriptMode & 0o111, 0);
});

test("agent cleanup examples prune only expired patrol session files", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-cleanup-"));
  const oldStamp = "202001010000";

  const cases = [
    {
      script: "examples/cleanup/openclaw-jsonl-cleanup.sh",
      envKey: "AGENT_PATROL_OPENCLAW_SESSION_DIR",
      oldPatrol: "patrol-finance-old.jsonl",
      freshPatrol: "patrol-finance-fresh.jsonl",
      regular: "regular-session.jsonl"
    },
    {
      script: "examples/cleanup/hermes-json-cleanup.sh",
      envKey: "AGENT_PATROL_HERMES_SESSION_DIR",
      oldPatrol: "patrol-hermes-old.json",
      freshPatrol: "patrol-hermes-fresh.json",
      regular: "session_cron_keep.json"
    },
    {
      script: "examples/cleanup/claude-session-cleanup.sh",
      envKey: "AGENT_PATROL_CLAUDE_SESSION_DIR",
      oldPatrol: "patrol-claude-old.jsonl",
      freshPatrol: "patrol-claude-fresh.jsonl",
      regular: "conversation-keep.jsonl"
    }
  ];

  for (const item of cases) {
    const sessionDir = path.join(dir, item.envKey);
    fs.mkdirSync(sessionDir, { recursive: true });
    for (const name of [item.oldPatrol, item.freshPatrol, item.regular]) {
      fs.writeFileSync(path.join(sessionDir, name), "{}", "utf8");
    }
    spawnSync("touch", ["-t", oldStamp, path.join(sessionDir, item.oldPatrol)], { encoding: "utf8" });
    spawnSync("touch", ["-t", oldStamp, path.join(sessionDir, item.regular)], { encoding: "utf8" });

    const result = spawnSync("bash", [item.script], {
      cwd: process.cwd(),
      encoding: "utf8",
      env: {
        ...process.env,
        [item.envKey]: sessionDir,
        AGENT_PATROL_SESSION_RETENTION_DAYS: "1"
      }
    });

    assert.equal(result.status, 0, `${item.script} failed: ${result.stderr}`);
    assert.equal(fs.existsSync(path.join(sessionDir, item.oldPatrol)), false, `${item.oldPatrol} should be removed`);
    assert.equal(fs.existsSync(path.join(sessionDir, item.freshPatrol)), true, `${item.freshPatrol} should stay`);
    assert.equal(fs.existsSync(path.join(sessionDir, item.regular)), true, `${item.regular} should stay`);
  }
});

test("agent cleanup dispatcher requires explicitly configured kinds", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-cleanup-dispatcher-"));
  const oldStamp = "202001010000";
  const files = [
    {
      envKey: "AGENT_PATROL_OPENCLAW_SESSION_DIR",
      name: "patrol-openclaw-old.jsonl"
    },
    {
      envKey: "AGENT_PATROL_HERMES_SESSION_DIR",
      name: "patrol-hermes-old.json"
    },
    {
      envKey: "AGENT_PATROL_CLAUDE_SESSION_DIR",
      name: "patrol-claude-old.jsonl"
    }
  ];
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    AGENT_PATROL_SESSION_RETENTION_DAYS: "1"
  };

  for (const item of files) {
    const sessionDir = path.join(dir, item.envKey);
    fs.mkdirSync(sessionDir, { recursive: true });
    const file = path.join(sessionDir, item.name);
    fs.writeFileSync(file, "{}", "utf8");
    spawnSync("touch", ["-t", oldStamp, file], { encoding: "utf8" });
    env[item.envKey] = sessionDir;
  }
  delete env.AGENT_PATROL_CLEANUP_KINDS;

  const result = spawnSync("bash", ["examples/cleanup/run-agent-cleanups.sh"], {
    cwd: process.cwd(),
    encoding: "utf8",
    env
  });

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /skip agent cleanup: no AGENT_PATROL_CLEANUP_KINDS/);
  for (const item of files) {
    assert.equal(
      fs.existsSync(path.join(dir, item.envKey, item.name)),
      true,
      `${item.name} should stay when cleanup kinds are not configured`
    );
  }
});
