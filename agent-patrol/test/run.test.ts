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
