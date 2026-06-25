import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { runSuite } from "../src/run.ts";
import type { AgentEnvelope, PatrolCase } from "../src/types.ts";

test("runSuite executes actual agent path, scores, and writes report files", async () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-run-"));
  const config = {
    report: { minAccuracy: 0.9 },
    templates: {
      status: {
        questions: ["看一下当前状态。"],
        scoring: { mustContain: ["状态正常"] }
      }
    },
    targets: {
      demo: {
        runner: {
          type: "command_agent",
          command: "unused",
          isolatedSessionPrefix: "patrol-demo",
          requireSessionIsolation: true
        },
        oracle: {
          type: "readonly_mcp",
          mcpUrl: "http://127.0.0.1/mcp",
          allowedTools: ["read_status"]
        },
        suites: { smoke: { templates: ["status"], caseCount: 1 } }
      }
    }
  };

  const result = await runSuite(config, {
    suite: "smoke",
    seed: "fixed",
    outDir,
    executeAgent: async (item: { patrolCase: PatrolCase; sessionId: string }): Promise<AgentEnvelope> => ({
      source: "agent",
      answer: `状态正常：${item.patrolCase.question}`,
      sessionId: item.sessionId,
      toolCalls: [{ name: "read_status" }]
    })
  });

  assert.equal(result.aggregate.total, 1);
  assert.equal(result.aggregate.passed, 1);
  assert.equal(result.aggregate.accuracy, 1);
  assert.equal(typeof result.aggregate.durationMs, "number");
  assert.ok(result.aggregate.durationMs >= 0);
  assert.equal(result.results[0]?.runner, "command_agent");
  assert.equal(typeof result.results[0]?.durationMs, "number");
  assert.ok(result.results[0]!.durationMs >= 0);
  assert.equal(fs.existsSync(path.join(outDir, "summary.md")), true);
  assert.equal(fs.existsSync(path.join(outDir, "summary.json")), true);
  assert.equal(fs.existsSync(path.join(outDir, "raw_results.jsonl")), true);
  assert.equal(fs.existsSync(path.join(outDir, "scores.json")), true);
});

test("runSuite captures FinanceQA reference and writes per-case evidence", async () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-evidence-"));
  const config = {
    report: { minAccuracy: 0.9 },
    templates: {
      receivable: {
        questions: ["从2025年10月起到上一个完整自然月月底，所有项目的应收未收是多少？"],
        scoring: {
          amounts: [{ label: "应收未收", value: 2185200 }],
          sources: ["项目应收"],
          periods: ["2025年10月", "2026年5月"],
          perspectives: ["项目口径"]
        }
      }
    },
    targets: {
      finance_qa: {
        runner: {
          type: "command_agent",
          command: "unused",
          isolatedSessionPrefix: "patrol-finance"
        },
        oracle: {
          type: "financeqa_readonly",
          mcpUrl: "http://127.0.0.1/mcp",
          allowedTools: ["finance-query"]
        },
        suites: { smoke: { templates: ["receivable"], caseCount: 1 } }
      }
    }
  };

  const result = await runSuite(config, {
    suite: "smoke",
    seed: "fixed",
    outDir,
    executeAgent: async (item: { sessionId: string }): Promise<AgentEnvelope> => ({
      source: "agent",
      answer: "项目口径看，2025年10月至2026年5月，项目应收的应收未收为 2,185,200.00 元。",
      sessionId: item.sessionId
    }),
    executeReference: async () => ({
      source: "financeqa_mcp",
      tool: "finance-query",
      answer: "FinanceQA MCP：项目应收口径，2025年10月至2026年5月，应收未收为 2,185,200.00 元。"
    })
  });

  assert.equal(result.evidence.length, 1);
  assert.equal(result.evidence[0]?.question, "从2025年10月起到上一个完整自然月月底，所有项目的应收未收是多少？");
  assert.deepEqual(result.evidence[0]?.expected, config.templates.receivable.scoring);
  assert.match(result.evidence[0]?.actual.answer ?? "", /2,185,200\.00/);
  assert.match(result.evidence[0]?.reference?.answer ?? "", /FinanceQA MCP/);
  assert.equal(result.evidence[0]?.score.pass, true);

  const evidencePath = path.join(outDir, "case_evidence.jsonl");
  assert.equal(fs.existsSync(evidencePath), true);
  const evidenceRows = fs.readFileSync(evidencePath, "utf8").trim().split("\n").map((line) => JSON.parse(line));
  assert.equal(evidenceRows.length, 1);
  assert.equal(evidenceRows[0].reference.tool, "finance-query");
  assert.equal(evidenceRows[0].score.pass, true);
});

test("runSuite detects agent/reference mismatch through reference-driven scoring", async () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-reference-driven-"));
  const config = {
    report: { minAccuracy: 0.9 },
    templates: {
      receivable: {
        questions: ["从2025年10月起到上一个完整自然月月底，所有项目的应收未收是多少？"],
        scoring: {
          referenceChecks: {
            amounts: { labels: ["项目应收", "应收未收"] },
            periods: true,
            sources: true,
            perspectives: true
          }
        }
      }
    },
    targets: {
      finance_qa: {
        runner: {
          type: "command_agent",
          command: "unused",
          isolatedSessionPrefix: "patrol-finance"
        },
        oracle: {
          type: "financeqa_readonly",
          mcpUrl: "http://127.0.0.1/mcp",
          allowedTools: ["finance-query"]
        },
        suites: { smoke: { templates: ["receivable"], caseCount: 1 } }
      }
    }
  };

  const result = await runSuite(config, {
    suite: "smoke",
    seed: "fixed",
    outDir,
    executeAgent: async (item: { sessionId: string }): Promise<AgentEnvelope> => ({
      source: "agent",
      answer: "从2025年10月起到2026年5月底，项目应收 146,688.40 元。来源：《fin-revcost-0601.xlsx》",
      sessionId: item.sessionId
    }),
    executeReference: async () => ({
      source: "financeqa_mcp",
      tool: "finance-query",
      answer: "2025-10~2026-04 老板口径先看项目汇总：项目应收 10943576.36 元。来源：《fin-revenue-0422.xlsx》《fin-revcost-0601.xlsx》"
    })
  });

  assert.equal(result.aggregate.passed, 0);
  assert.deepEqual(result.scores[0]?.failureDetails.map((failure) => failure.type), [
    "agent_changed_amount",
    "missing_source",
    "period_mismatch",
    "perspective_mismatch",
    "scorer_term_miss"
  ]);
  const evidencePath = path.join(outDir, "failed_cases", "finance_qa_receivable_001.json");
  assert.equal(fs.existsSync(evidencePath), true);
  const evidence = JSON.parse(fs.readFileSync(evidencePath, "utf8"));
  assert.equal(evidence.score.failureDetails[0].type, "agent_changed_amount");
  assert.match(evidence.reference.answer, /10943576\.36/);
});

test("runSuite applies runner timeout to command agents", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-timeout-"));
  const scriptPath = path.join(dir, "slow_agent.mjs");
  const outDir = path.join(dir, "out");
  fs.writeFileSync(scriptPath, `
setTimeout(() => {
  console.log(JSON.stringify({ result: { answer: "状态正常", sessionId: process.argv[process.argv.indexOf("--session-id") + 1] } }));
}, 200);
`, "utf8");

  const config = {
    report: { minAccuracy: 0.9 },
    templates: {
      status: {
        questions: ["看一下当前状态。"],
        scoring: { mustContain: ["状态正常"] }
      }
    },
    targets: {
      demo: {
        runner: {
          type: "command_agent",
          command: `node ${scriptPath} --session-id {sessionId}`,
          timeoutMs: 50
        },
        oracle: {
          type: "readonly_mcp",
          mcpUrl: "http://127.0.0.1/mcp",
          allowedTools: ["read_status"]
        },
        suites: { smoke: { templates: ["status"], caseCount: 1 } }
      }
    }
  };

  await assert.rejects(
    () => runSuite(config, { suite: "smoke", seed: "fixed", outDir }),
    /timed out after 50ms/
  );
});
