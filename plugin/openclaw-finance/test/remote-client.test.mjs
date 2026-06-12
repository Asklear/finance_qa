import test from "node:test";
import assert from "node:assert/strict";
import http from "node:http";

import { RemoteMCPClient } from "../dist/index.esm.js";

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
