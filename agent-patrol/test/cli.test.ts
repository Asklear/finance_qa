import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { main, parseCliArgs } from "../src/index.ts";

test("parseCliArgs parses command flags", () => {
  const parsed = parseCliArgs(["generate", "--config", "patrol.yaml", "--suite", "smoke", "--out", "tmp/out"]);
  assert.equal(parsed.command, "generate");
  assert.equal(parsed.flags.config, "patrol.yaml");
  assert.equal(parsed.flags.suite, "smoke");
  assert.equal(parsed.flags.out, "tmp/out");
});

test("help command lists only implemented commands", async () => {
  const originalLog = console.log;
  const messages: string[] = [];
  console.log = (message?: unknown) => {
    messages.push(String(message ?? ""));
  };
  try {
    const code = await main(["help"]);
    assert.equal(code, 0);
  } finally {
    console.log = originalLog;
  }

  const help = messages.join("\n");
  assert.match(help, /generate/);
  assert.match(help, /doctor/);
  assert.match(help, /run/);
  assert.doesNotMatch(help, /score/);
});

test("generate command writes cases.json to output directory", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-cli-"));
  const configPath = path.join(dir, "patrol.yaml");
  const outDir = path.join(dir, "out");
  fs.writeFileSync(configPath, `
templates:
  latest_revenue:
    questions: [收入表中最新月份的营收是多少？]
targets:
  finance:
    runner:
      type: openclaw_agent_cli
      command: openclaw agent --json --message {question}
    oracle:
      type: financeqa_readonly
      mcpUrl: http://127.0.0.1:3009/mcp
      allowedTools: [finance-query]
    suites:
      smoke:
        caseCount: 1
        templates: [latest_revenue]
`, "utf8");

  const code = await main(["generate", "--config", configPath, "--suite", "smoke", "--seed", "fixed", "--out", outDir]);

  assert.equal(code, 0);
  const cases = JSON.parse(fs.readFileSync(path.join(outDir, "cases.json"), "utf8"));
  assert.equal(cases.length, 1);
  assert.equal(cases[0].actualRunner, "openclaw_agent_cli");
});

test("run command executes configured command agent and writes summary", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-cli-run-"));
  const scriptPath = path.join(dir, "agent_stub.mjs");
  const configPath = path.join(dir, "patrol.yaml");
  const outDir = path.join(dir, "out");
  fs.writeFileSync(scriptPath, `
import fs from "node:fs";
const questionFile = process.argv[process.argv.indexOf("--question-file") + 1];
const sessionId = process.argv[process.argv.indexOf("--session-id") + 1];
const question = fs.readFileSync(questionFile, "utf8");
console.log(JSON.stringify({ result: { answer: "状态正常 " + question, sessionId, toolCalls: [{ name: "read_status" }] } }));
`, "utf8");
  fs.writeFileSync(configPath, `
report:
  minAccuracy: 0.9
templates:
  status:
    questions: [看一下当前状态。]
    scoring:
      mustContain: [状态正常]
targets:
  demo:
    runner:
      type: command_agent
      command: "node ${scriptPath} --question-file {questionFile} --session-id {sessionId}"
      isolatedSessionPrefix: patrol-demo
      requireSessionIsolation: true
    oracle:
      type: readonly_mcp
      mcpUrl: http://127.0.0.1/mcp
      allowedTools: [read_status]
    suites:
      smoke:
        caseCount: 1
        templates: [status]
`, "utf8");

  const code = await main(["run", "--config", configPath, "--suite", "smoke", "--seed", "fixed", "--out", outDir]);

  assert.equal(code, 0);
  assert.match(fs.readFileSync(path.join(outDir, "summary.md"), "utf8"), /Accuracy: 100\.00%/);
  assert.equal(fs.existsSync(path.join(outDir, "raw_results.jsonl")), true);
});
