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

test("parseAgentEnvelope extracts OpenClaw nested session key", () => {
  const envelope = parseAgentEnvelope(JSON.stringify({
    result: {
      payloads: [{ text: "AGENT_PATROL_OK" }],
      meta: {
        agentMeta: {
          sessionId: "7d7a39ca-d320-4a12-9ca8-970262cbf73e"
        },
        systemPromptReport: {
          sessionKey: "agent:main:main"
        }
      }
    }
  }));

  assert.equal(envelope.sessionId, "7d7a39ca-d320-4a12-9ca8-970262cbf73e");
  assert.equal(envelope.sessionKey, "agent:main:main");
});

test("validateAgentEnvelope rejects direct-MCP-only actual output", () => {
  assert.throws(() => validateAgentEnvelope({
    answer: "直接工具答案",
    source: "direct_mcp"
  }, { requireSessionIsolation: true }), /actual agent envelope/i);
});

test("validateAgentEnvelope rejects mismatched OpenClaw sessions", () => {
  assert.throws(() => validateAgentEnvelope({
    answer: "AGENT_PATROL_OK",
    source: "agent",
    sessionId: "7d7a39ca-d320-4a12-9ca8-970262cbf73e",
    sessionKey: "agent:main:main"
  }, {
    requireSessionIsolation: true,
    expectedSessionId: "patrol-session-1"
  }), /session isolation/i);
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

test("runCommandAgent attaches OpenClaw session tool evidence when session transcript exists", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-session-evidence-"));
  const sessionDir = path.join(dir, "sessions");
  fs.mkdirSync(sessionDir);
  const scriptPath = path.join(dir, "agent_stub.mjs");
  fs.writeFileSync(scriptPath, `
const sessionId = process.argv[process.argv.indexOf("--session-id") + 1];
console.log(JSON.stringify({
  status: "ok",
  result: {
    payloads: [{ text: "最终答案" }],
    meta: { agentMeta: { sessionId } }
  }
}));
`, "utf8");
  fs.writeFileSync(path.join(sessionDir, "patrol-session-2.jsonl"), [
    JSON.stringify({ type: "session", id: "patrol-session-2" }),
    JSON.stringify({
      type: "message",
      message: {
        role: "user",
        content: [{
          type: "text",
          text: "<relevant-memories>历史财务数据</relevant-memories>\n[Sat 2026-06-27 01:00 UTC] 巡检问题"
        }]
      }
    }),
    JSON.stringify({
      type: "message",
      message: {
        role: "assistant",
        content: [{
          type: "toolCall",
          id: "call_1",
          name: "finance-query",
          arguments: { query: "从2025年10月起到上一个完整自然月月底，所有项目的应收未收是多少？" }
        }]
      }
    }),
    JSON.stringify({
      type: "message",
      message: {
        role: "toolResult",
        toolCallId: "call_1",
        toolName: "finance-query",
        content: [{
          type: "text",
          text: JSON.stringify({
            success: true,
            data: {
              query_spec: {
                period_from: "2026-06",
                period_to: "2026-06",
                requested_period: "2002-06"
              },
              total: 525200
            }
          })
        }]
      }
    })
  ].join("\n") + "\n", "utf8");

  const previousSessionDir = process.env.AGENT_PATROL_OPENCLAW_SESSION_DIR;
  process.env.AGENT_PATROL_OPENCLAW_SESSION_DIR = sessionDir;
  try {
    const envelope = await runCommandAgent({
      commandTemplate: `node ${scriptPath} --question-file {questionFile} --session-id {sessionId}`,
      question: "巡检问题",
      sessionId: "patrol-session-2",
      requireSessionIsolation: true,
      cwd: dir
    });

    assert.deepEqual(envelope.toolCalls, [{
      id: "call_1",
      name: "finance-query",
      arguments: { query: "从2025年10月起到上一个完整自然月月底，所有项目的应收未收是多少？" }
    }]);
    assert.equal(envelope.sessionEvidence?.sessionFile, path.join(sessionDir, "patrol-session-2.jsonl"));
    assert.equal(envelope.sessionEvidence?.userMessages?.[0]?.text, "<relevant-memories>历史财务数据</relevant-memories>\n[Sat 2026-06-27 01:00 UTC] 巡检问题");
    assert.equal(envelope.sessionEvidence?.toolResults?.[0]?.toolName, "finance-query");
    const toolJson = envelope.sessionEvidence?.toolResults?.[0]?.json as {
      data?: { query_spec?: { requested_period?: string }; total?: number };
    };
    assert.equal(toolJson.data?.query_spec?.requested_period, "2002-06");
    assert.equal(toolJson.data?.total, 525200);
  } finally {
    if (previousSessionDir === undefined) {
      delete process.env.AGENT_PATROL_OPENCLAW_SESSION_DIR;
    } else {
      process.env.AGENT_PATROL_OPENCLAW_SESSION_DIR = previousSessionDir;
    }
  }
});
