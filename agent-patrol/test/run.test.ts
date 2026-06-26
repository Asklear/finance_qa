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

test("runSuite captures direct-tool baseline and writes per-case evidence", async () => {
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

test("runSuite detects agent mismatch through active reference scoring", async () => {
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

test("runSuite scores against golden reference and stores direct tool baseline separately", async () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-golden-reference-"));
  const config = {
    report: { minAccuracy: 0.9 },
    templates: {
      payable: {
        questions: ["2025年10月至上一个完整自然月月底，所有项目的应付未付是多少？"],
        scoring: {
          referenceChecks: {
            amounts: { labels: ["项目应付"] },
            periods: true,
            sources: true
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
        goldenReference: {
          type: "command",
          command: "unused"
        },
        suites: { smoke: { templates: ["payable"], caseCount: 1 } }
      }
    }
  };

  const result = await runSuite(config, {
    suite: "smoke",
    seed: "fixed",
    outDir,
    executeAgent: async (item: { sessionId: string }): Promise<AgentEnvelope> => ({
      source: "agent",
      answer: "2025-10~2026-05 项目应付 636000.00 元。来源：《fin-revcost-0601.xlsx》",
      sessionId: item.sessionId
    }),
    executeReference: async () => ({
      source: "financeqa_mcp",
      tool: "finance-query",
      answer: "2025-10~2026-04 项目应付 1029611.43 元。来源：《fin-revcost-0506.xlsx》"
    }),
    executeGoldenReference: async () => ({
      source: "golden_reference",
      tool: "financeqa-structured-golden",
      answer: "2025-10~2026-05 项目应付 636000.00 元。来源：《fin-revcost-0601.xlsx》"
    })
  });

  assert.equal(result.aggregate.passed, 1);
  assert.equal(result.evidence[0]?.goldenReference?.tool, "financeqa-structured-golden");
  assert.equal(result.evidence[0]?.directToolBaseline?.tool, "finance-query");
  assert.match(result.evidence[0]?.reference?.answer ?? "", /636000\.00/);

  const evidenceRows = fs.readFileSync(path.join(outDir, "case_evidence.jsonl"), "utf8").trim().split("\n").map((line) => JSON.parse(line));
  assert.match(evidenceRows[0].goldenReference.answer, /636000\.00/);
  assert.match(evidenceRows[0].directToolBaseline.answer, /1029611\.43/);
});

test("runSuite fails closed when configured golden reference returns no result", async () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-missing-golden-reference-"));
  const config = {
    report: { minAccuracy: 0.9 },
    templates: {
      revenue: {
        questions: ["收入表中最新月份的营收是多少？"],
        scoring: {
          referenceChecks: {
            amounts: { labels: ["项目结算"] },
            periods: true,
            sources: true
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
        goldenReference: {
          type: "command",
          command: "unused"
        },
        suites: { smoke: { templates: ["revenue"], caseCount: 1 } }
      }
    }
  };

  const result = await runSuite(config, {
    suite: "smoke",
    seed: "fixed",
    outDir,
    executeAgent: async (item: { sessionId: string }): Promise<AgentEnvelope> => ({
      source: "agent",
      answer: "最新月份项目结算 800000.00 元。来源：《fin-revenue-0601.xlsx》",
      sessionId: item.sessionId
    }),
    executeReference: async () => ({
      source: "financeqa_mcp",
      tool: "finance-query",
      answer: "最新月份项目结算 800000.00 元。来源：《fin-revenue-0601.xlsx》"
    }),
    executeGoldenReference: async () => undefined
  });

  assert.equal(result.aggregate.passed, 0);
  assert.equal(result.evidence[0]?.directToolBaseline?.tool, "finance-query");
  assert.equal(result.evidence[0]?.goldenReference?.source, "golden_reference");
  assert.equal(result.evidence[0]?.reference?.source, "golden_reference");
  assert.deepEqual(result.scores[0]?.failureDetails.map((failure) => failure.type), ["missing_reference"]);

  const evidenceRows = fs.readFileSync(path.join(outDir, "case_evidence.jsonl"), "utf8").trim().split("\n").map((line) => JSON.parse(line));
  assert.match(evidenceRows[0].goldenReference.error, /configured.*did not return/);
  assert.match(evidenceRows[0].directToolBaseline.answer, /800000\.00/);
  assert.equal(evidenceRows[0].score.pass, false);
});

test("runSuite does not execute golden reference when target does not configure it", async () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-no-golden-reference-"));
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
          command: "unused"
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

  await runSuite(config, {
    suite: "smoke",
    seed: "fixed",
    outDir,
    executeAgent: async (): Promise<AgentEnvelope> => ({
      source: "agent",
      answer: "状态正常"
    }),
    executeReference: async () => undefined,
    executeGoldenReference: async () => {
      throw new Error("golden reference should not run without target.goldenReference");
    }
  });
});

test("runSuite keeps golden scoring when direct baseline exceeds oracle timeout", async () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-direct-baseline-timeout-"));
  const config = {
    report: { minAccuracy: 0.9 },
    templates: {
      revenue: {
        questions: ["收入表中最新月份的营收是多少？"],
        scoring: {
          referenceChecks: {
            amounts: { labels: ["项目结算"] },
            sources: true
          }
        }
      }
    },
    targets: {
      finance_qa: {
        runner: {
          type: "command_agent",
          command: "unused"
        },
        oracle: {
          type: "financeqa_readonly",
          mcpUrl: "http://127.0.0.1/mcp",
          allowedTools: ["finance-query"],
          timeoutMs: 10
        },
        goldenReference: {
          type: "command",
          command: "unused"
        },
        suites: { smoke: { templates: ["revenue"], caseCount: 1 } }
      }
    }
  };

  const result = await runSuite(config, {
    suite: "smoke",
    seed: "fixed",
    outDir,
    executeAgent: async (): Promise<AgentEnvelope> => ({
      source: "agent",
      answer: "项目结算 800000.00 元。来源：《fin-revenue-0601.xlsx》"
    }),
    executeReference: async () => {
      await new Promise((resolve) => setTimeout(resolve, 80));
      return {
        source: "financeqa_mcp",
        tool: "finance-query",
        answer: "迟到的 direct baseline"
      };
    },
    executeGoldenReference: async () => ({
      source: "golden_reference",
      tool: "financeqa-structured-golden",
      answer: "项目结算 800000.00 元。来源：《fin-revenue-0601.xlsx》"
    })
  });

  assert.equal(result.aggregate.passed, 1);
  assert.match(result.evidence[0]?.directToolBaseline?.error ?? "", /timed out after 10ms/);
  assert.match(result.evidence[0]?.reference?.answer ?? "", /800000\.00/);
});

test("runSuite rejects non-golden envelopes from configured golden executor", async () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-wrong-golden-source-"));
  const config = {
    report: { minAccuracy: 0.9 },
    templates: {
      payable: {
        questions: ["项目应付是多少？"],
        scoring: {
          referenceChecks: {
            amounts: { labels: ["项目应付"] }
          }
        }
      }
    },
    targets: {
      finance_qa: {
        runner: {
          type: "command_agent",
          command: "unused"
        },
        oracle: {
          type: "financeqa_readonly",
          mcpUrl: "http://127.0.0.1/mcp",
          allowedTools: ["finance-query"]
        },
        goldenReference: {
          type: "command",
          command: "unused"
        },
        suites: { smoke: { templates: ["payable"], caseCount: 1 } }
      }
    }
  };

  const result = await runSuite(config, {
    suite: "smoke",
    seed: "fixed",
    outDir,
    executeAgent: async (): Promise<AgentEnvelope> => ({
      source: "agent",
      answer: "项目应付 600.00 元。"
    }),
    executeReference: async () => undefined,
    executeGoldenReference: async () => ({
      source: "financeqa_mcp",
      tool: "finance-query",
      answer: "项目应付 600.00 元。"
    })
  });

  assert.equal(result.aggregate.passed, 0);
  assert.equal(result.evidence[0]?.goldenReference?.source, "golden_reference");
  assert.match(result.evidence[0]?.goldenReference?.error ?? "", /expected source golden_reference/);
  assert.deepEqual(result.scores[0]?.failureDetails.map((failure) => failure.type), ["missing_reference"]);
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
