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

test("financeqa daily schedule examples only write local reports", () => {
  const files = [
    "examples/schedules/financeqa-daily.env.example",
    "examples/schedules/financeqa-daily.cron.example",
    "examples/schedules/financeqa-daily.service",
    "examples/schedules/financeqa-daily.timer"
  ];
  const contents = files.map((file) => fs.readFileSync(file, "utf8")).join("\n");

  assert.match(contents, /presets\/financeqa\.yaml/);
  assert.match(contents, /--suite daily/);
  assert.match(contents, /tmp\/financeqa-daily/);
  assert.match(contents, /AGENT_PATROL_LIVE=1/);
  assert.match(contents, /OPENCLAW_AGENT_CMD="/);
  assert.doesNotMatch(contents, /--deliver/);
  assert.doesNotMatch(contents, /\blzh\b/);
});
