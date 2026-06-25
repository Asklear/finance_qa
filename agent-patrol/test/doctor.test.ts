import test from "node:test";
import assert from "node:assert/strict";
import { runDoctor } from "../src/doctor.ts";

test("runDoctor reports safe runner and read-only oracle config", () => {
  const report = runDoctor({
    targets: {
      finance: {
        runner: { type: "openclaw_agent_cli", command: "openclaw agent --json --message {question}" },
        oracle: { type: "financeqa_readonly", mcpUrl: "http://127.0.0.1/mcp", allowedTools: ["finance-query"] }
      }
    }
  });

  assert.equal(report.ok, true);
  assert.deepEqual(report.targets[0]?.blockedWriteTools, []);
  assert.equal(report.targets[0]?.runner.available, true);
});

test("runDoctor fails closed on write tools and missing actual runner command", () => {
  const report = runDoctor({
    targets: {
      bad: {
        runner: { type: "claude_agent_sdk" },
        oracle: {
          type: "bossa_readonly_mcp",
          mcpUrl: "http://127.0.0.1/mcp",
          allowedTools: ["get_all_deals", "create_scheduled_job"]
        }
      }
    }
  });

  assert.equal(report.ok, false);
  assert.deepEqual(report.targets[0]?.blockedWriteTools, ["create_scheduled_job"]);
  assert.equal(report.targets[0]?.runner.available, false);
});
