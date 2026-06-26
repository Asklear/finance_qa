import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { executeGoldenReference, executeReadonlyReference } from "../src/reference.ts";
import type { PatrolCase, TargetConfig } from "../src/types.ts";

test("executeReadonlyReference calls FinanceQA MCP as read-only direct-tool baseline", async () => {
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

test("executeReadonlyReference times out slow FinanceQA MCP calls", async () => {
  const server = http.createServer((req, res) => {
    req.resume();
    setTimeout(() => {
      res.setHeader("content-type", "application/json");
      res.end(JSON.stringify({
        result: {
          content: [{ type: "text", text: JSON.stringify({ final_answer: "迟到的 direct baseline" }) }]
        }
      }));
    }, 80);
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  assert.ok(address && typeof address === "object");

  try {
    const reference = await executeReadonlyReference({
      patrolCase: patrolCase("case-timeout", "收入表中最新月份的营收是多少？"),
      target: {
        ...target(`http://127.0.0.1:${address.port}/mcp`),
        oracle: {
          ...target(`http://127.0.0.1:${address.port}/mcp`).oracle,
          timeoutMs: 10
        }
      }
    });

    assert.equal(reference?.source, "financeqa_mcp");
    assert.equal(reference?.tool, "finance-query");
    assert.match(reference?.error ?? "", /timed out after 10ms/);
  } finally {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
});

test("executeGoldenReference runs configured command and extracts structured answer", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-golden-reference-"));
  const scriptPath = path.join(dir, "golden_reference.mjs");
  fs.writeFileSync(scriptPath, `
import fs from "node:fs";

const questionFile = process.argv[process.argv.indexOf("--question-file") + 1];
const caseId = process.argv[process.argv.indexOf("--case-id") + 1];
const question = fs.readFileSync(questionFile, "utf8");
console.log("diagnostic: building structured golden reference");
console.log(JSON.stringify({
  result: {
    final_answer: \`case=\${caseId}; question=\${question}; 项目应收 1,234.00 元。\`
  }
}));
`, "utf8");

  const reference = await executeGoldenReference({
    patrolCase: patrolCase("case-golden", "从2025年10月起到上一个完整自然月月底，所有项目的应收未收是多少？"),
    target: {
      ...target("http://127.0.0.1/mcp"),
      goldenReference: {
        type: "command",
        command: `node ${scriptPath} --case-id {caseId} --question-file {questionFile}`,
        timeoutMs: 5_000
      }
    }
  });

  assert.equal(reference?.source, "golden_reference");
  assert.equal(reference?.tool, "command");
  assert.match(reference?.answer ?? "", /case=case-golden/);
  assert.match(reference?.answer ?? "", /项目应收 1,234\.00 元/);
});

test("executeGoldenReference records an error when command JSON has no extractable answer", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-empty-golden-reference-"));
  const scriptPath = path.join(dir, "empty_golden_reference.mjs");
  fs.writeFileSync(scriptPath, "console.log(JSON.stringify({ result: {} }));\n", "utf8");

  const reference = await executeGoldenReference({
    patrolCase: patrolCase("case-empty-golden", "收入表中最新月份的营收是多少？"),
    target: {
      ...target("http://127.0.0.1/mcp"),
      goldenReference: {
        type: "command",
        command: `node ${scriptPath}`
      }
    }
  });

  assert.equal(reference?.source, "golden_reference");
  assert.equal(reference?.tool, "command");
  assert.equal(reference?.answer, undefined);
  assert.match(reference?.error ?? "", /no extractable answer/);
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
