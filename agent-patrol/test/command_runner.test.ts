import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { buildCommandInvocation, parseAgentEnvelope, runCommandAgent, validateAgentEnvelope } from "../src/runners/command_runner.ts";

test("buildCommandInvocation replaces placeholders without shell interpolation", () => {
  const invocation = buildCommandInvocation(
    "openclaw agent --agent {agent} --json --message {questionFile} --session-id {sessionId}",
    {
      agent: "main",
      questionFile: "/tmp/question.txt",
      sessionId: "patrol-finance-1"
    }
  );

  assert.deepEqual(invocation, {
    command: "openclaw",
    args: ["agent", "--agent", "main", "--json", "--message", "/tmp/question.txt", "--session-id", "patrol-finance-1"]
  });
});

test("parseAgentEnvelope extracts OpenClaw answer and session metadata", () => {
  const envelope = parseAgentEnvelope(JSON.stringify({
    result: {
      payloads: [{ text: "最终答案" }],
      sessionId: "patrol-finance-1",
      toolCalls: [{ name: "finance-query" }]
    }
  }));

  assert.equal(envelope.answer, "最终答案");
  assert.equal(envelope.sessionId, "patrol-finance-1");
  assert.deepEqual(envelope.toolCalls, [{ name: "finance-query" }]);
});

test("parseAgentEnvelope accepts OpenClaw pretty JSON with warning lines and nested agent metadata", () => {
  const envelope = parseAgentEnvelope(`Config warnings:\\n- duplicate plugin id detected
{
  "runId": "run-1",
  "status": "ok",
  "result": {
    "payloads": [
      {
        "text": "AGENT_PATROL_OK",
        "mediaUrl": null
      }
    ],
    "meta": {
      "agentMeta": {
        "sessionId": "patrol-smoke-1",
        "provider": "provider",
        "model": "model"
      }
    }
  }
}
`);

  assert.equal(envelope.answer, "AGENT_PATROL_OK");
  assert.equal(envelope.sessionId, "patrol-smoke-1");
});

test("validateAgentEnvelope rejects direct-MCP-only actual output", () => {
  assert.throws(() => validateAgentEnvelope({
    answer: "直接工具答案",
    source: "direct_mcp"
  }, { requireSessionIsolation: true }), /actual agent envelope/i);
});

test("runCommandAgent executes a JSON agent command with question file and isolated session", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-runner-"));
  const scriptPath = path.join(dir, "agent_stub.mjs");
  fs.writeFileSync(scriptPath, `
import fs from "node:fs";
const questionFile = process.argv[process.argv.indexOf("--question-file") + 1];
const sessionId = process.argv[process.argv.indexOf("--session-id") + 1];
const question = fs.readFileSync(questionFile, "utf8");
console.log(JSON.stringify({
  result: {
    answer: "ANSWER:" + question,
    sessionId,
    toolCalls: [{ name: "read_status" }]
  }
}));
`, "utf8");

  const envelope = await runCommandAgent({
    commandTemplate: `node ${scriptPath} --question-file {questionFile} --session-id {sessionId}`,
    question: "巡检问题",
    sessionId: "patrol-session-1",
    requireSessionIsolation: true,
    cwd: dir
  });

  assert.equal(envelope.answer, "ANSWER:巡检问题");
  assert.equal(envelope.sessionId, "patrol-session-1");
  assert.deepEqual(envelope.toolCalls, [{ name: "read_status" }]);
});
