import test from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { executeReadonlyReference } from "../src/reference.ts";
import type { PatrolCase, TargetConfig } from "../src/types.ts";

test("executeReadonlyReference calls FinanceQA MCP as read-only finance-query reference", async () => {
  let requestBody = "";
  let authorization = "";
  const server = http.createServer((req, res) => {
    authorization = req.headers.authorization ?? "";
    req.setEncoding("utf8");
    req.on("data", (chunk) => {
      requestBody += chunk;
    });
    req.on("end", () => {
      res.setHeader("content-type", "application/json");
      res.end(JSON.stringify({
        jsonrpc: "2.0",
        id: "case-1",
        result: {
          content: [{
            type: "text",
            text: JSON.stringify({
              final_answer: "FinanceQA MCP：项目应收口径，应收未收为 2,185,200.00 元。"
            })
          }]
        }
      }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  assert.ok(address && typeof address === "object");
  process.env.AGENT_PATROL_TEST_TOKEN = "test-token";
  try {
    const reference = await executeReadonlyReference({
      patrolCase: patrolCase("case-1", "所有项目的应收未收是多少？"),
      target: target(`http://127.0.0.1:${address.port}/mcp`)
    });

    assert.equal(reference?.source, "financeqa_mcp");
    assert.equal(reference?.tool, "finance-query");
    assert.match(reference?.answer ?? "", /2,185,200\.00/);
    assert.equal(authorization, "Bearer test-token");
    const payload = JSON.parse(requestBody);
    assert.equal(payload.method, "tools/call");
    assert.equal(payload.params.name, "finance-query");
    assert.equal(payload.params.arguments.query, "所有项目的应收未收是多少？");
  } finally {
    delete process.env.AGENT_PATROL_TEST_TOKEN;
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
});

test("executeReadonlyReference records configuration errors instead of failing the suite", async () => {
  const reference = await executeReadonlyReference({
    patrolCase: patrolCase("case-2", "收入表中最新月份的营收是多少？"),
    target: target("${FINANCEQA_MCP_URL}")
  });

  assert.equal(reference?.source, "financeqa_mcp");
  assert.equal(reference?.tool, "finance-query");
  assert.match(reference?.error ?? "", /mcpUrl is not configured/);
});

function patrolCase(id: string, question: string): PatrolCase {
  return {
    id,
    target: "finance_qa",
    template: "finance",
    question,
    actualRunner: "command_agent",
    oracle: "financeqa_readonly",
    scoring: {}
  };
}

function target(mcpUrl: string): TargetConfig {
  return {
    runner: {
      type: "command_agent",
      command: "unused"
    },
    oracle: {
      type: "financeqa_readonly",
      mcpUrl,
      bearerTokenEnv: "AGENT_PATROL_TEST_TOKEN",
      allowedTools: ["finance-query", "finance-host-data"]
    }
  };
}
