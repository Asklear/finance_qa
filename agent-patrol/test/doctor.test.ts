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

test("runDoctor can require a structured golden reference for production patrols", () => {
  const report = runDoctor({
    targets: {
      finance: {
        runner: { type: "openclaw_agent_cli", command: "openclaw agent --json --message {question}" },
        oracle: { type: "financeqa_readonly", mcpUrl: "http://127.0.0.1/mcp", allowedTools: ["finance-query"] }
      }
    }
  }, { requireGoldenReference: true });

  assert.equal(report.ok, false);
  assert.deepEqual(report.targets[0]?.goldenReference?.problems, ["missing_golden_reference"]);
  assert.match(report.targets[0]?.problems.join(","), /missing_golden_reference/);
});

test("runDoctor rejects unresolved placeholders in production patrols", () => {
  const report = runDoctor({
    targets: {
      finance: {
        runner: { type: "openclaw_agent_cli", command: "${OPENCLAW_AGENT_CMD}" },
        oracle: { type: "financeqa_readonly", mcpUrl: "${FINANCEQA_MCP_URL}", allowedTools: ["finance-query"] },
        goldenReference: { type: "command", command: "${FINANCEQA_GOLDEN_CMD}" }
      }
    }
  }, { requireGoldenReference: true, requireResolvedEnv: true });

  assert.equal(report.ok, false);
  assert.deepEqual(report.targets[0]?.runner.problems, ["unresolved_runner_command"]);
  assert.deepEqual(report.targets[0]?.oracle.problems, ["unresolved_oracle_mcp_url"]);
  assert.deepEqual(report.targets[0]?.goldenReference?.problems, ["unresolved_golden_reference_command"]);
});
