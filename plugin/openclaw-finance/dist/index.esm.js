import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawn } from "node:child_process";
import { existsSync } from "node:fs";

const PLUGIN_ID = "openclaw-finance";

// Path to financeqa binary (auto-detect or from env)
const FINANCEQA_BIN = process.env.FINANCEQA_BIN || findFinanceQABinary();

function findFinanceQABinary() {
  const pluginDir = path.dirname(fileURLToPath(import.meta.url));
  const repoRoot = path.resolve(pluginDir, "../../..");
  const fixedServerBin = path.join(process.env.HOME || "", "finance_qa/bin/financeqa");
  const candidates = [
    fixedServerBin,
    path.resolve(repoRoot, "bin/financeqa"),
    path.resolve(process.cwd(), "bin/financeqa")
  ];
  for (const candidate of candidates) {
    if (existsSync(candidate)) {
      return candidate;
    }
  }
  return fixedServerBin;
}

function findFinanceQACwd(binaryPath) {
  const binDir = path.dirname(binaryPath);
  if (path.basename(binDir) === "bin") {
    return path.dirname(binDir);
  }
  return process.cwd();
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
        cwd: findFinanceQACwd(this.binaryPath),
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

function messageContentText(content) {
  if (typeof content === "string") return content;
  if (Array.isArray(content)) {
    return content
      .map((item) => {
        if (typeof item === "string") return item;
        if (item && typeof item === "object") return item.text || item.content || "";
        return "";
      })
      .filter(Boolean)
      .join("\n");
  }
  if (content && typeof content === "object") return content.text || content.content || "";
  return "";
}

function isFinanceQuestion(rawText) {
  const text = userVisibleText(rawText);
  if (!text || text.startsWith("/")) return false;
  if (/有限公司/.test(text) && /(20\d{2}年|\d{1,2}月|Q[1-4]|季度)/i.test(text)) return true;
  if (/数据(出来|有了|有没有|情况|多少)/.test(text)) return true;
  return FINANCE_KEYWORDS.some((keyword) => text.includes(keyword));
}

function latestFinanceQuestionFromMessages(messages) {
  if (!Array.isArray(messages)) return "";
  for (let i = messages.length - 1; i >= 0 && i >= messages.length - 12; i--) {
    const entry = messages[i];
    const message = entry?.message && typeof entry.message === "object" ? entry.message : entry;
    if (message?.role !== "user") continue;
    const text = userVisibleText(messageContentText(message.content));
    if (isFinanceQuestion(text)) return text;
  }
  return "";
}

function latestUserTextFromMessages(messages) {
  if (!Array.isArray(messages)) return "";
  for (let i = messages.length - 1; i >= 0 && i >= messages.length - 6; i--) {
    const entry = messages[i];
    const message = entry?.message && typeof entry.message === "object" ? entry.message : entry;
    if (message?.role !== "user") continue;
    return userVisibleText(messageContentText(message.content));
  }
  return "";
}

function isRetryOrContinuation(rawText) {
  return /(continue where you left off|previous model attempt failed|timed out|继续|接着)/i.test(String(rawText || ""));
}

function financeQuestionForPromptEvent(event) {
  const prompt = userVisibleText(event?.prompt || "");
  if (isFinanceQuestion(prompt)) return prompt;
  const latestUserText = latestUserTextFromMessages(event?.messages);
  if (isFinanceQuestion(latestUserText)) return latestUserText;
  if (isRetryOrContinuation(prompt) || isRetryOrContinuation(latestUserText)) {
    return latestFinanceQuestionFromMessages(event?.messages);
  }
  return "";
}

function mustCallFinanceQuerySystemContext(latestQuestion) {
  const lines = [
    "Finance QA routing policy:",
    latestQuestion ? `Latest finance question that MUST be sent to finance-query: ${latestQuestion}` : "",
    "The current `finance-query` result may be injected into the prompt as authoritative context. Use that current result for key amounts, periods, business basis, and source notes before considering prior conversation history.",
    "For any finance, business operation, contract, collection, invoice, revenue, cost, profit, cash, bank, tax, AR/AP, customer, supplier, or source-table question, you MUST call `finance-query` before answering.",
    "Do not answer from prior conversation history, memory, previous tool results, raw SQL, income statement/book values, or cached summaries, even when the latest user message repeats an earlier question.",
    "After `finance-query`, use the tool result as the authoritative source for the current answer. If the result has `final_answer` or `boss_reply_text`, preserve its key amounts, period, business basis, and source notes, but you may rephrase the surrounding wording.",
    "do not omit the source note. If you paraphrase for brevity, the final visible answer must still include the current result's `来源：...` sentence from `source_note`, `source_summary`, `final_answer`, or `boss_reply_text`.",
    "If the current tool result includes `contract_continuity_candidates`, use those candidates as same-project continuity evidence and make a tentative business inference with uncertainty; call it a same-project candidate/reference and do not state that the counterparty definitely changed or became associated.",
    "Keep the source note from the tool result. Do not expose internal IDs, SQL, route traces, or contract IDs unless the user explicitly asks for technical details."
  ];
  return lines.filter(Boolean).join("\n");
}

function parseToolResultPayload(result) {
  const text = result?.content?.find((item) => item?.type === "text")?.text || "";
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    return { message: text };
  }
}

function compactFinancePayload(payload) {
  if (!payload || typeof payload !== "object") return payload;
  const data = payload.data && typeof payload.data === "object" ? payload.data : {};
  return {
    error: payload.error,
    success: payload.success,
    final_answer: payload.final_answer || payload.boss_reply_text || payload.message,
    source_note: data.source_note || payload.source_note,
    source_update_note: data.source_update_note || payload.source_update_note,
    period: data.period,
    source_priority: data.source_priority,
    requested_metrics: data.requested_metrics,
    role: data.role,
    account_view: data.account_view,
    cash_view: data.cash_view,
    contract_summary: data.contract_summary,
    customer_summary: data.customer_summary,
    supplier_summary: data.supplier_summary,
    tax_summary: data.tax_summary,
    source_documents: data.source_documents,
    source_cell_notes: data.source_cell_notes,
    remarks: data.remarks,
    contract_continuity_candidates: data.contract_continuity_candidates
  };
}

async function financeQueryPromptContext(question) {
  const result = await callFinanceTool("finance-query", { query: question });
  const payload = compactFinancePayload(parseToolResultPayload(result));
  return [
    "Current authoritative finance-query result for the latest user finance question.",
    "Use these current facts for the visible answer. Preserve the key amounts, period, business basis, and source note; do not reuse conflicting amounts or sources from prior conversation history.",
    "You may rephrase the final wording, but the numbers and source must match this current result.",
    "```json",
    JSON.stringify({ query: question, result: payload }, null, 2),
    "```"
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
  register(api) {
    api.registerTool(createFinanceTool(
      "finance-query",
      "Boss finance QA. Call this first for finance questions. When the returned JSON has final_answer or boss_reply_text, preserve key amounts, period, business basis, uncertainty, and source notes; surrounding wording may be rephrased. When it has contract_continuity_candidates, describe them as same-project candidates/references, not a confirmed counterparty mapping.",
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

    api.on("before_prompt_build", async (event) => {
      const latestQuestion = financeQuestionForPromptEvent(event);
      if (!latestQuestion) return undefined;
      const prependContext = await financeQueryPromptContext(latestQuestion);
      return {
        prependSystemContext: mustCallFinanceQuerySystemContext(latestQuestion),
        prependContext
      };
    });
  }
};

export { plugin as default };
