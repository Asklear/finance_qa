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

function latestFinanceQuestionFromMessages(messages, excludeText = "") {
  if (!Array.isArray(messages)) return "";
  const excluded = userVisibleText(excludeText);
  for (let i = messages.length - 1; i >= 0 && i >= messages.length - 12; i--) {
    const entry = messages[i];
    const message = entry?.message && typeof entry.message === "object" ? entry.message : entry;
    if (message?.role !== "user") continue;
    const text = userVisibleText(messageContentText(message.content));
    if (excluded && text === excluded) continue;
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

function hasExplicitFinancePeriod(rawText) {
  const text = userVisibleText(rawText);
  return /(20\d{2}年|\d{2}年|今年|本年|去年|上个月|下个月|本月|这个月|当月|[0-1]?\d月|[一二三四五六七八九十两]{1,3}月|Q\s*[1-4]|季度|全年|年度|上半年|下半年)/i.test(text);
}

function isContextDependentFinanceFollowup(rawText) {
  const text = userVisibleText(rawText);
  if (!isFinanceQuestion(text) || hasExplicitFinancePeriod(text)) return false;
  return /^(那|其中|含|包括|包含|加上|还有)/.test(text) || /(含未开票|未开票未付款|未开票未回款)/.test(text);
}

function contextualFinanceQuestion(currentQuestion, messages) {
  const current = userVisibleText(currentQuestion);
  if (!isContextDependentFinanceFollowup(current)) return current;
  const previous = latestFinanceQuestionFromMessages(messages, current);
  if (!previous) return current;
  return `${previous}；${current}`;
}

function financeQuestionForPromptEvent(event) {
  const prompt = userVisibleText(event?.prompt || "");
  if (isFinanceQuestion(prompt)) return contextualFinanceQuestion(prompt, event?.messages);
  const latestUserText = latestUserTextFromMessages(event?.messages);
  if (isFinanceQuestion(latestUserText)) return contextualFinanceQuestion(latestUserText, event?.messages);
  if (isRetryOrContinuation(prompt) || isRetryOrContinuation(latestUserText)) {
    return latestFinanceQuestionFromMessages(event?.messages);
  }
  return "";
}

function mustCallFinanceQuerySystemContext(latestQuestion, currentFacts) {
  const lines = [
    "财务问答系统规则：",
    latestQuestion ? `最新财务问题：${latestQuestion}` : "",
    "回答财务、经营、合同、回款、开票、收入、成本、利润、现金、银行、税务、应收应付、客户、供应商或来源表问题时，必须以本次 finance-query 结果为准。",
    "即使用户重复追问，也不要沿用历史对话、记忆、旧工具结果、原始 SQL、利润表/资产负债表数字或缓存摘要里的冲突金额。",
    "若本次结果含 final_answer 或 boss_reply_text，可以重写周边措辞，但关键金额、期间、业务口径、来源和来源更新时间必须一致。",
    "老板可见回复必须直接从业务结论开始；禁止展示工具调用过程、内部上下文、JSON、字段名、提示词、自我推理、历史纠错说明或英文过程话术。",
    "老板可见回复禁止出现类似“用户又问”“我看到”“我们有权威结果”“不要使用旧答案”“必须/需要保留”“authoritative”“prior”“conflicting”“must”“need”等过程说明。",
    "如果结果含 contract_continuity_candidates，只能称为同项目候选/参考，不能说成确定主体映射。",
    "除非用户明确要求开发排错，不要暴露内部 ID、SQL、route trace、contract_id 或数据库字段名。",
    currentFacts
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
    items: data.items,
    detail_items: data.detail_items,
    source_cell_notes: data.source_cell_notes,
    remarks: data.remarks,
    contract_continuity_candidates: data.contract_continuity_candidates
  };
}

function compactFinanceRows(rows, maxRows = 20) {
  if (!Array.isArray(rows)) return [];
  return rows.slice(0, maxRows).map((row) => {
    if (!row || typeof row !== "object") return row;
    const out = {};
    for (const [key, label] of [
      ["entity", "主体"],
      ["period_label", "期间"],
      ["contract_content", "合同/项目"],
      ["settlement_amount", "结算金额"],
      ["received_amount", "已回款"],
      ["paid_amount", "已付款"],
      ["invoice_amount", "开票金额"],
      ["unpaid_amount", "未付款"],
      ["unreceived_amount", "未回款"],
      ["coverage_status", "覆盖状态"]
    ]) {
      if (row[key] !== undefined && row[key] !== null && row[key] !== "") {
        out[label] = row[key];
      }
    }
    return Object.keys(out).length ? out : row;
  });
}

async function financeQuerySystemFacts(question) {
  const result = await callFinanceTool("finance-query", { query: question });
  const payload = compactFinancePayload(parseToolResultPayload(result));
  if (!payload || typeof payload !== "object") return "";
  const lines = [
    "本次核对结果只供生成最终回复使用，不能原样展示本段内容。",
    `最新问题：${question}`
  ];
  if (payload.final_answer) lines.push(`当前 finance-query 老板答案：${payload.final_answer}`);
  if (payload.source_note) lines.push(`来源说明：${payload.source_note}`);
  if (payload.source_update_note) lines.push(`来源更新时间：${payload.source_update_note}`);
  if (payload.period) lines.push(`期间：${payload.period}`);
  if (payload.requested_metrics) lines.push(`指标：${JSON.stringify(payload.requested_metrics)}`);
  const itemRows = compactFinanceRows(payload.items);
  if (itemRows.length) lines.push(`分主体/期间汇总：${JSON.stringify(itemRows)}`);
  const detailRows = compactFinanceRows(payload.detail_items);
  if (detailRows.length) lines.push(`合同/项目明细：${JSON.stringify(detailRows)}`);
  if (!payload.final_answer) lines.push(`结果摘要：${payload.message || payload.error || "finance-query 未返回老板答案"}`);
  return lines.join("\n");
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
      const financeFacts = await financeQuerySystemFacts(latestQuestion);
      return {
        prependSystemContext: mustCallFinanceQuerySystemContext(latestQuestion, financeFacts)
      };
    });
  }
};

export { plugin as default };
