import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawn } from "node:child_process";

const PLUGIN_ID = "openclaw-finance";

// Path to financeqa binary (auto-detect or from env)
const FINANCEQA_BIN = process.env.FINANCEQA_BIN || findFinanceQABinary();

function findFinanceQABinary() {
  // Try common paths
  const candidates = [
    path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../../bin/financeqa"),
    path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../bin/financeqa"),
    "/usr/local/bin/financeqa",
    "/usr/bin/financeqa",
    "./financeqa",
    "financeqa"
  ];
  // Return first candidate - actual existence checked at runtime
  return candidates[0];
}

const FINANCE_KEYWORDS = [
  "财务",
  "经营",
  "合同",
  "回款",
  "收款",
  "付款",
  "开票",
  "发票",
  "收入",
  "营收",
  "销售额",
  "成本",
  "费用",
  "利润",
  "应收",
  "应付",
  "到账",
  "支出",
  "流入",
  "流出",
  "现金",
  "银行",
  "余额",
  "销项",
  "进项",
  "税额",
  "供应商",
  "客户",
  "货币资金"
];

// MCP Client for communicating with financeqa serve
class MCPClient {
  constructor(binaryPath) {
    this.binaryPath = binaryPath;
    this.process = null;
    this.requestId = 0;
    this.pendingRequests = new Map();
    this.initialized = false;
  }

  async start() {
    if (this.process) return;

    return new Promise((resolve, reject) => {
      this.process = spawn(this.binaryPath, ["serve"], {
        stdio: ["pipe", "pipe", "pipe"],
        env: { ...process.env }
      });

      let buffer = "";
      this.process.stdout.on("data", (chunk) => {
        buffer += chunk.toString("utf8");
        const lines = buffer.split("\n");
        buffer = lines.pop(); // Keep incomplete line in buffer

        for (const line of lines) {
          if (line.trim()) {
            this.handleMessage(line.trim());
          }
        }
      });

      this.process.stderr.on("data", (chunk) => {
        console.error("[financeqa]", chunk.toString("utf8"));
      });

      this.process.on("error", (err) => {
        reject(new Error(`Failed to start financeqa: ${err.message}`));
      });

      this.process.on("close", (code) => {
        this.process = null;
        this.initialized = false;
        if (code !== 0 && code !== null) {
          console.error(`financeqa process exited with code ${code}`);
        }
      });

      // Wait a bit for process to start, then send initialize
      setTimeout(async () => {
        try {
          await this.sendRequest("initialize", {});
          this.initialized = true;
          resolve();
        } catch (err) {
          reject(err);
        }
      }, 500);
    });
  }

  stop() {
    if (this.process) {
      this.process.kill();
      this.process = null;
    }
    this.initialized = false;
  }

  handleMessage(line) {
    try {
      const msg = JSON.parse(line);
      if (msg.id !== undefined && this.pendingRequests.has(msg.id)) {
        const { resolve, reject } = this.pendingRequests.get(msg.id);
        this.pendingRequests.delete(msg.id);
        if (msg.error) {
          reject(new Error(msg.error.message || JSON.stringify(msg.error)));
        } else {
          resolve(msg.result);
        }
      }
    } catch (err) {
      console.error("[mcp] Failed to parse message:", line.slice(0, 200));
    }
  }

  sendRequest(method, params) {
    return new Promise((resolve, reject) => {
      if (!this.process) {
        reject(new Error("MCP client not started"));
        return;
      }

      this.requestId++;
      const id = this.requestId;
      const request = {
        jsonrpc: "2.0",
        id,
        method,
        params
      };

      this.pendingRequests.set(id, { resolve, reject });

      // Timeout after 60 seconds
      setTimeout(() => {
        if (this.pendingRequests.has(id)) {
          this.pendingRequests.delete(id);
          reject(new Error(`Request timeout: ${method}`));
        }
      }, 60000);

      this.process.stdin.write(JSON.stringify(request) + "\n");
    });
  }

  async callTool(name, args) {
    if (!this.initialized) {
      await this.start();
    }
    return this.sendRequest("tools/call", {
      name,
      arguments: args
    });
  }
}

// Global MCP client instance (lazy init)
let mcpClient = null;

async function getMCPClient() {
  if (!mcpClient) {
    mcpClient = new MCPClient(FINANCEQA_BIN);
    await mcpClient.start();
  }
  return mcpClient;
}

function textResult(payload) {
  return {
    content: [
      { type: "text", text: typeof payload === "string" ? payload : JSON.stringify(payload, null, 2) }
    ]
  };
}

function errorResult(error) {
  const message = error instanceof Error ? error.message : String(error);
  return textResult({ error: message });
}

function userVisibleText(value) {
  return String(value || "")
    .replace(/Sender \(untrusted metadata\):\s*```json[\s\S]*?```\s*/g, "")
    .replace(/\[[^\]]*GMT[^\]]*\]\s*/g, "")
    .trim();
}

function isFinanceQuestion(rawText) {
  const text = userVisibleText(rawText);
  if (!text || text.startsWith("/")) return false;
  if (/有限公司/.test(text) && /(20\d{2}年|\d{1,2}月|Q[1-4]|季度)/i.test(text)) return true;
  if (/数据(出来|有了|有没有|情况|多少)/.test(text)) return true;
  return FINANCE_KEYWORDS.some((keyword) => text.includes(keyword));
}

function mustCallFinanceQuerySystemContext() {
  return [
    "Finance QA routing policy:",
    "For any finance, business operation, contract, collection, invoice, revenue, cost, profit, cash, bank, tax, AR/AP, customer, supplier, or source-table question, you MUST call `finance-query` before answering.",
    "Do not answer from prior conversation history, memory, previous tool results, raw SQL, income statement/book values, or cached summaries, even when the latest user message repeats an earlier question.",
    "After `finance-query`, parse `content[0].text` as JSON and answer from the current tool result. If `final_answer` or `boss_reply_text` exists, use that text as the visible answer instead of summarizing from structured data.",
    "do not omit the source note. If you paraphrase for brevity, the final visible answer must still include the current result's `来源：...` sentence from `source_note`, `source_summary`, `final_answer`, or `boss_reply_text`.",
    "If the current tool result includes `contract_continuity_candidates`, use those candidates as same-project continuity evidence and make a tentative business inference with uncertainty; call it a same-project candidate/reference and do not state that the counterparty definitely changed or became associated.",
    "Keep the source note from the tool result. Do not expose internal IDs, SQL, route traces, or contract IDs unless the user explicitly asks for technical details."
  ].join("\n");
}

async function callFinanceTool(name, rawParams) {
  try {
    const client = await getMCPClient();
    const result = await client.callTool(name, rawParams || {});

    // Convert MCP tool result to OpenClaw format
    if (result.content && result.content[0] && result.content[0].type === "text") {
      try {
        // Parse the JSON result from the text
        const parsed = JSON.parse(result.content[0].text);
        return textResult(parsed);
      } catch {
        // Return as-is if not JSON
        return result;
      }
    }
    return result;
  } catch (error) {
    return errorResult(error);
  }
}

function createFinanceTool(name, description, parameters) {
  return {
    name,
    label: name,
    description,
    parameters,
    async execute(_toolCallId, rawParams) {
      return callFinanceTool(name, rawParams);
    }
  };
}

const plugin = {
  id: PLUGIN_ID,
  name: "Finance",
  description: "Finance MCP plugin (native, no Python bridge)",
  async register(api) {
    // Try to pre-start MCP client
    try {
      await getMCPClient();
    } catch (err) {
      console.error("[finance] Failed to pre-start MCP client:", err.message);
      // Continue anyway, will retry on first tool call
    }

    api.registerTool(createFinanceTool(
      "finance-query",
      "Boss finance QA. Call this first for finance questions. When the returned JSON has final_answer or boss_reply_text, preserve key amounts, uncertainty wording, and source notes. When it has contract_continuity_candidates, describe them as same-project candidates/references, not a confirmed counterparty mapping.",
      {
        type: "object",
        properties: {
          query: {
            type: "string",
            description: "The latest natural-language finance question from the user"
          }
        },
        required: ["query"]
      }
    ), { name: "finance-query" });

    api.registerTool(createFinanceTool(
      "finance-upload",
      "Upload and import Excel files (bank statements, journals, balance sheets, contract ledgers, etc.)",
      {
        type: "object",
        properties: {
          file: {
            type: "string",
            description: "Path to the Excel file to upload"
          }
        },
        required: ["file"]
      }
    ), { name: "finance-upload" });

    api.registerTool(createFinanceTool(
      "finance-sync",
      "Synchronize a directory of financial Excel files",
      {
        type: "object",
        properties: {
          directory: {
            type: "string",
            description: "Directory path containing Excel files"
          },
          incremental: {
            type: "boolean",
            description: "Incremental sync (don't clear existing data)"
          }
        },
        required: ["directory"]
      }
    ), { name: "finance-sync" });

    api.on("before_prompt_build", (event) => {
      const prompt = userVisibleText(event?.prompt || "");
      if (!isFinanceQuestion(prompt)) return undefined;
      return { prependSystemContext: mustCallFinanceQuerySystemContext() };
    });

    // Cleanup on exit
    api.on("cleanup", () => {
      if (mcpClient) {
        mcpClient.stop();
        mcpClient = null;
      }
    });
  }
};

export { plugin as default };
