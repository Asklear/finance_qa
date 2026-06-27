import test from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import os from "node:os";

import { RemoteMCPClient, normalizePluginConfig } from "../dist/index.esm.js";

test("normalizePluginConfig reads remote bearer token from mcp_token_file", async () => {
  const dir = await mkdtemp(path.join(os.tmpdir(), "finance-token-file-"));
  const tokenFile = path.join(dir, "mcp_read_token");
  try {
    await writeFile(tokenFile, " file-token-value \n", { mode: 0o600 });

    const config = normalizePluginConfig({
      transport: "remote",
      mcp_url: "http://127.0.0.1:3009/mcp",
      mcp_token_file: tokenFile
    });

    assert.equal(config.transport, "remote");
    assert.equal(config.mcp_url, "http://127.0.0.1:3009/mcp");
    assert.equal(config.mcp_token, "file-token-value");
    assert.equal(config.mcp_token_file, tokenFile);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("RemoteMCPClient sends bearer auth, accept header, and reuses MCP session id", async () => {
  const seen = [];
  await withServer(async (req, res, body) => {
    seen.push({ headers: req.headers, body: JSON.parse(body || "{}") });
    assert.equal(req.headers.authorization, "Bearer test-token");
    assert.match(req.headers.accept || "", /application\/json/);
    assert.match(req.headers.accept || "", /text\/event-stream/);

    if (seen.length === 1) {
      assert.equal(seen[0].body.method, "initialize");
      res.setHeader("Mcp-Session-Id", "session-1");
      writeJSON(res, {
        jsonrpc: "2.0",
        id: seen[0].body.id,
        result: { serverInfo: { name: "financeqa-mcp" }, capabilities: {} }
      });
      return;
    }

    assert.equal(req.headers["mcp-session-id"], "session-1");
    assert.equal(seen[1].body.method, "tools/call");
    assert.equal(seen[1].body.params.name, "finance-query");
    writeJSON(res, {
      jsonrpc: "2.0",
      id: seen[1].body.id,
      result: { content: [{ type: "text", text: "{\"ok\":true}" }] }
    });
  }, async (url) => {
    const client = new RemoteMCPClient({ url, token: "test-token", timeoutMs: 5000 });
    const result = await client.callTool("finance-query", { query: "2026年3月营收" });
    assert.equal(result.content[0].text, "{\"ok\":true}");
  });
});

test("RemoteMCPClient parses SSE JSON-RPC responses", async () => {
  await withServer(async (req, res, body) => {
    const message = JSON.parse(body || "{}");
    res.setHeader("Content-Type", "text/event-stream");
    if (message.method === "initialize") {
      res.end(`event: message\ndata: ${JSON.stringify({
        jsonrpc: "2.0",
        id: message.id,
        result: { serverInfo: { name: "financeqa-mcp" }, capabilities: {} }
      })}\n\n`);
      return;
    }
    res.end(`event: message\ndata: ${JSON.stringify({
      jsonrpc: "2.0",
      id: message.id,
      result: { content: [{ type: "text", text: "{\"sse\":true}" }] }
    })}\n\n`);
  }, async (url) => {
    const client = new RemoteMCPClient({ url, token: "test-token", timeoutMs: 5000 });
    const result = await client.callTool("finance-query", { query: "test" });
    assert.equal(result.content[0].text, "{\"sse\":true}");
  });
});

test("RemoteMCPClient reports auth failures without leaking token", async () => {
  await withServer(async (_req, res) => {
    res.statusCode = 401;
    res.end("unauthorized");
  }, async (url) => {
    const client = new RemoteMCPClient({ url, token: "super-secret-token", timeoutMs: 5000 });
    await assert.rejects(
      () => client.callTool("finance-query", { query: "test" }),
      (error) => {
        assert.match(error.message, /auth|401|unauthorized/i);
        assert.doesNotMatch(error.message, /super-secret-token/);
        return true;
      }
    );
  });
});

test("finance prompt hook strips relevant memories before prefetching facts", async () => {
  const toolCalls = [];
  await withFinancePluginHarness(toolCalls, async ({ hooks }) => {
    const wrappedQuestion = `<relevant-memories>
The following are stored memories for user "mem0-tqt". Use them to personalize your response:
- As of 2026-06-25, 项目口径从2025年10月到2026年5月的应收未收总额为146,688.40 元。
</relevant-memories>

[Sat 2026-06-27 07:01 UTC] 从2025年10月起到上一个完整自然月月底，所有项目的应收未收是多少？`;

    await hooks.get("before_prompt_build")({
      prompt: wrappedQuestion,
      messages: [{ role: "user", content: [{ type: "text", text: wrappedQuestion }] }]
    });

    assert.equal(toolCalls[0].arguments.query, "从2025年10月起到上一个完整自然月月底，所有项目的应收未收是多少？");
  });
});

test("finance-query execute keeps clean tool query after polluted prompt hook", async () => {
  const toolCalls = [];
  await withFinancePluginHarness(toolCalls, async ({ hooks, tools }) => {
    const lzhWrappedPrompt = `Conversation info (untrusted metadata):
\`\`\`json
{
  "message_id": "openclaw-weixin:test",
  "timestamp": "Fri 2026-06-26 13:51 GMT+8"
}
\`\`\`

帮我做一个 润泽科技公司深度分析。包含公司概况 核心业务 财务数据 竞争格局 能力优势等`;

    await hooks.get("before_prompt_build")({
      prompt: lzhWrappedPrompt,
      messages: [{ role: "user", content: [{ type: "text", text: lzhWrappedPrompt }] }]
    });

    await tools.get("finance-query").execute("call-clean-query", {
      query: "润泽科技 客户 合同 收入 回款"
    });

    assert.equal(toolCalls.at(-1).arguments.query, "润泽科技 客户 合同 收入 回款");
  });
});

async function withServer(handler, run) {
  const server = http.createServer(async (req, res) => {
    let body = "";
    req.setEncoding("utf8");
    req.on("data", (chunk) => {
      body += chunk;
    });
    req.on("end", async () => {
      try {
        await handler(req, res, body);
      } catch (error) {
        res.statusCode = 500;
        res.end(error.stack || String(error));
      }
    });
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  try {
    await run(`http://127.0.0.1:${address.port}/mcp`);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

function writeJSON(res, payload) {
  res.setHeader("Content-Type", "application/json");
  res.end(JSON.stringify(payload));
}

async function withFinancePluginHarness(toolCalls, run) {
  await withServer(async (req, res, body) => {
    const message = JSON.parse(body || "{}");
    assert.equal(req.headers.authorization, "Bearer test-token");
    if (message.method === "initialize") {
      res.setHeader("Mcp-Session-Id", "finance-test-session");
      writeJSON(res, {
        jsonrpc: "2.0",
        id: message.id,
        result: { serverInfo: { name: "financeqa-mcp" }, capabilities: {} }
      });
      return;
    }

    assert.equal(req.headers["mcp-session-id"], "finance-test-session");
    assert.equal(message.method, "tools/call");
    assert.equal(message.params.name, "finance-query");
    toolCalls.push(message.params);
    writeJSON(res, {
      jsonrpc: "2.0",
      id: message.id,
      result: {
        content: [
          {
            type: "text",
            text: JSON.stringify({ success: true, final_answer: "ok" })
          }
        ]
      }
    });
  }, async (url) => {
    const moduleUrl = `../dist/index.esm.js?test=${Date.now()}-${Math.random()}`;
    const { default: plugin } = await import(moduleUrl);
    const tools = new Map();
    const hooks = new Map();
    plugin.register({
      getPluginConfig() {
        return {
          transport: "remote",
          mcp_url: url,
          mcp_token: "test-token",
          timeout_ms: 5000
        };
      },
      registerTool(tool, options) {
        tools.set(options?.name || tool.name, tool);
      },
      on(name, handler) {
        hooks.set(name, handler);
      }
    });
    await run({ hooks, tools });
  });
}
