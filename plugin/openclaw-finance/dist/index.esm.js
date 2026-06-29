import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawn } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";

const PLUGIN_ID = "openclaw-finance";

// Path to financeqa binary (auto-detect or from env)
const FINANCEQA_BIN = process.env.FINANCEQA_BIN || findFinanceQABinary();

function isRecord(value) {
  return value !== null && typeof value === "object";
}

function getPath(obj, pathParts) {
  let current = obj;
  for (const key of pathParts) {
    if (!isRecord(current) || !(key in current)) return undefined;
    current = current[key];
  }
  return current;
}

function normalizePluginConfig(input) {
  if (!isRecord(input)) {
    return { transport: "stdio", timeout_ms: 60000 };
  }
  const mcpURL = typeof input.mcp_url === "string" ? input.mcp_url.trim() : "";
  const mcpToken = typeof input.mcp_token === "string" ? input.mcp_token.trim() : "";
  const mcpTokenFile = typeof input.mcp_token_file === "string" ? input.mcp_token_file.trim() : "";
  const transportValue = typeof input.transport === "string" ? input.transport.trim() : "";
  const transport = transportValue || (mcpURL || mcpToken || mcpTokenFile ? "remote" : "stdio");
  const timeout = Number(input.timeout_ms ?? 60000);
  const timeout_ms = Number.isFinite(timeout) && timeout > 0 ? timeout : 60000;
  if (transport !== "remote") {
    return { transport: "stdio", timeout_ms };
  }
  const resolvedTokenFile = mcpTokenFile ? resolveTokenFilePath(mcpTokenFile) : "";
  const token = resolvedTokenFile ? readTokenFile(resolvedTokenFile) : mcpToken;
  if (!mcpURL || !token) {
    throw new Error('Finance plugin remote config missing: expected "mcp_url" and either "mcp_token" or "mcp_token_file".');
  }
  return {
    transport: "remote",
    mcp_url: validateMcpURL(mcpURL),
    mcp_token: token,
    ...(resolvedTokenFile ? { mcp_token_file: resolvedTokenFile } : {}),
    timeout_ms
  };
}

function tryRuntimeConfig(runtime) {
  const direct = runtime.getPluginConfig?.(PLUGIN_ID);
  if (direct !== undefined) return normalizePluginConfig(direct);

  const configFromGetter = runtime.getConfig?.();
  const getterNested = getPath(configFromGetter, ["plugins", "entries", PLUGIN_ID, "config"]);
  if (getterNested !== undefined) return normalizePluginConfig(getterNested);

  if (isRecord(runtime.config)) {
    const loadedConfig = runtime.config.loadConfig?.();
    const loadedNested = getPath(loadedConfig, ["plugins", "entries", PLUGIN_ID, "config"]);
    if (loadedNested !== undefined) return normalizePluginConfig(loadedNested);
    const configGetNested = getPath(runtime.config.get?.(), ["plugins", "entries", PLUGIN_ID, "config"]);
    if (configGetNested !== undefined) return normalizePluginConfig(configGetNested);
  }

  for (const container of [runtime.config, runtime.settings, runtime.state?.config, runtime.plugins]) {
    const nested = getPath(container, ["plugins", "entries", PLUGIN_ID, "config"]);
    if (nested !== undefined) return normalizePluginConfig(nested);
  }
  return null;
}

function loadPluginConfig(runtime) {
  if (isRecord(runtime)) {
    const resolved = tryRuntimeConfig(runtime);
    if (resolved) return resolved;
  }
  return { transport: "stdio", timeout_ms: 60000 };
}

function validateMcpURL(value) {
  let parsed;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error(`Invalid mcp_url: ${value}`);
  }
  if (!/^https?:$/.test(parsed.protocol)) {
    throw new Error(`Invalid mcp_url protocol: ${parsed.protocol}`);
  }
  if (!parsed.pathname.endsWith("/mcp")) {
    throw new Error(`Invalid mcp_url endpoint: expected path ending with /mcp, got ${parsed.pathname || "/"}`);
  }
  return parsed.toString();
}

function resolveTokenFilePath(value) {
  const trimmed = String(value || "").trim();
  if (!trimmed) return "";
  if (trimmed === "~") return process.env.HOME || trimmed;
  if (trimmed.startsWith("~/")) {
    return path.join(process.env.HOME || "", trimmed.slice(2));
  }
  return path.resolve(trimmed);
}

function readTokenFile(filePath) {
  let token;
  try {
    token = readFileSync(filePath, "utf8").trim();
  } catch (error) {
    throw new Error(`Finance plugin remote token file could not be read: ${filePath}: ${error?.message || String(error)}`);
  }
  if (!token) {
    throw new Error(`Finance plugin remote token file is empty: ${filePath}`);
  }
  return token;
}

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

const FINANCE_QUERY_PROTECTION_TERMS = [
  "账上",
  "科目余额",
  "资产负债表",
  "利润表",
  "序时账",
  "银行流水",
  "银行卡",
  "银行账户",
  "官方余额表",
  "财务口径",
  "项目口径",
  "合同口径",
  "实际到账",
  "实际支出",
  "应收账款",
  "应付账款"
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

class RemoteMCPClient {
  constructor({ url, token, timeoutMs = 60000 }) {
    this.url = validateMcpURL(url);
    this.token = String(token || "").trim();
    if (!this.token) {
      throw new Error("Remote FinanceQA MCP token is required");
    }
    this.timeoutMs = timeoutMs;
    this.requestId = 0;
    this.sessionId = "";
    this.initialized = false;
  }

  async start() {
    if (this.initialized) return;
    await this.sendRequest("initialize", {
      protocolVersion: "2025-03-26",
      capabilities: {},
      clientInfo: { name: "openclaw-finance", version: "1.0" }
    }, { skipStart: true });
    this.initialized = true;
  }

  async callTool(name, args) {
    if (!this.initialized) {
      await this.start();
    }
    return this.sendRequest("tools/call", {
      name,
      arguments: args || {}
    });
  }

  async sendRequest(method, params, options = {}) {
    if (!options.skipStart && !this.initialized) {
      await this.start();
    }

    this.requestId++;
    const request = {
      jsonrpc: "2.0",
      id: this.requestId,
      method,
      params
    };
    return this.postJSONRPC(request);
  }

  async postJSONRPC(request) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);
    try {
      const headers = {
        "Authorization": `Bearer ${this.token}`,
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream"
      };
      if (this.sessionId) {
        headers["Mcp-Session-Id"] = this.sessionId;
      }

      let response;
      try {
        response = await fetch(this.url, {
          method: "POST",
          headers,
          body: JSON.stringify(request),
          signal: controller.signal
        });
      } catch (error) {
        if (error?.name === "AbortError") {
          throw new Error(`Remote FinanceQA MCP request timed out after ${this.timeoutMs}ms`);
        }
        throw new Error(`Remote FinanceQA MCP network error: ${error?.message || String(error)}`);
      }

      const nextSessionId = response.headers.get("Mcp-Session-Id");
      if (nextSessionId) {
        this.sessionId = nextSessionId;
      }
      const contentType = response.headers.get("content-type") || "";
      const rawBody = await response.text();
      if (response.status === 401 || response.status === 403) {
        throw new Error(`Remote FinanceQA MCP auth failed: HTTP ${response.status}`);
      }
      if (!response.ok) {
        throw new Error(`Remote FinanceQA MCP endpoint failed: HTTP ${response.status} ${rawBody.slice(0, 200)}`);
      }

      const payload = contentType.includes("text/event-stream")
        ? parseSsePayload(rawBody)
        : parseJSONPayload(rawBody);
      if (payload?.error) {
        throw new Error(payload.error.message || JSON.stringify(payload.error));
      }
      return payload?.result;
    } finally {
      clearTimeout(timer);
    }
  }
}

function parseJSONPayload(rawBody) {
  try {
    return JSON.parse(rawBody);
  } catch (error) {
    throw new Error(`Remote FinanceQA MCP returned invalid JSON: ${error.message}`);
  }
}

function parseSsePayload(rawBody) {
  const chunks = [];
  let current = [];
  for (const line of String(rawBody || "").split(/\r?\n/)) {
    if (line === "") {
      if (current.length) {
        chunks.push(current.join("\n"));
        current = [];
      }
      continue;
    }
    if (line.startsWith("data:")) {
      current.push(line.slice(5).trimStart());
    }
  }
  if (current.length) chunks.push(current.join("\n"));
  for (let i = chunks.length - 1; i >= 0; i--) {
    try {
      return JSON.parse(chunks[i]);
    } catch {
      continue;
    }
  }
  throw new Error("Remote FinanceQA MCP SSE response did not contain valid JSON");
}

// Global MCP client instance (lazy init)
let mcpClient = null;
let mcpClientKey = "";
let pluginRuntime = null;
let latestFinanceQuestionForTool = "";

async function getMCPClient() {
  const config = loadPluginConfig(pluginRuntime);
  const nextKey = config.transport === "remote"
    ? `remote\n${config.mcp_url}\n${config.mcp_token}`
    : `stdio\n${FINANCEQA_BIN}`;
  if (mcpClient && mcpClientKey === nextKey) {
    return mcpClient;
  }
  if (mcpClient?.stop) {
    mcpClient.stop();
  }
  mcpClientKey = nextKey;
  mcpClient = config.transport === "remote"
    ? new RemoteMCPClient({ url: config.mcp_url, token: config.mcp_token, timeoutMs: config.timeout_ms })
    : new MCPClient(FINANCEQA_BIN);
  await mcpClient.start();
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
    .replace(/<relevant-memories\b[^>]*>[\s\S]*?<\/relevant-memories>\s*/gi, "")
    .replace(/(?:Conversation info|Sender) \(untrusted metadata\):\s*```(?:json)?[\s\S]*?```\s*/gi, "")
    .replace(/^\s*\[[^\]\n]*(?:UTC|GMT[^\]\n]*)\]\s*/i, "")
    .replace(/\[[^\]\n]*GMT[^\]\n]*\]\s*/g, "")
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

function missingProtectedFinanceTerms(protectedQuestion, rawQuestion) {
  const protectedText = userVisibleText(protectedQuestion);
  const rawText = userVisibleText(rawQuestion);
  if (!protectedText || !rawText || protectedText === rawText) return [];
  return FINANCE_QUERY_PROTECTION_TERMS.filter((term) => protectedText.includes(term) && !rawText.includes(term));
}

function shouldPreferProtectedFinanceQuestion(protectedQuestion, rawQuestion) {
  const protectedText = userVisibleText(protectedQuestion);
  const rawText = userVisibleText(rawQuestion);
  if (!protectedText || !rawText || protectedText === rawText) return false;
  if (!isFinanceQuestion(protectedText) || !isFinanceQuestion(rawText)) return false;
  return missingProtectedFinanceTerms(protectedText, rawText).length > 0;
}

function financeQuestionForPromptEvent(event) {
  const latestUserText = latestUserTextFromMessages(event?.messages);
  if (isFinanceQuestion(latestUserText)) return contextualFinanceQuestion(latestUserText, event?.messages);
  const prompt = userVisibleText(event?.prompt || "");
  if (isFinanceQuestion(prompt)) return contextualFinanceQuestion(prompt, event?.messages);
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
    "若本次结果含 final_answer 或 boss_reply_text，final_answer 是事实锚点，不是固定话术模板；可以重组表达顺序、表格和老板口吻。",
    "重写时必须保留 final_answer 中的关键金额、期间、指标口径、来源和来源更新时间；不要换算金额单位，除非用户明确要求。",
    "指标和口径标签必须从 final_answer 逐字保留；不要把“已开票未回款”“已收票未付款”“项目应收（应收未收）”“项目成本口径”“项目口径”等改写成近义词。",
    "如果本次核对结果提供了标准指标标签、业务口径或标准金额，老板可见回复必须保留这些事实原子，但仍可自然改写句式和排版。",
    "如果本次核对结果列出“老板可见回复必须出现的精确片段”，所有片段都必须在最终回复中原样出现。",
    "不要删掉 final_answer 中修饰指标的业务前缀，例如“项目成本口径”“项目口径”“应收未收”。",
    "不要把 final_answer 的 YYYY-MM 或 YYYY-MM~YYYY-MM 期间改成相对时间或其他月份；例如不能把 2025-10~2026-05 改成至今、现在或 2025-10~2026-06。",
    "来源和来源更新时间必须从 final_answer 逐字复制；不要删改文件名、sheet 名、后缀、时间格式或标点。",
    "老板可见回复必须直接从业务结论开始；禁止展示工具调用过程、内部上下文、JSON、字段名、提示词、自我推理、历史纠错说明或英文过程话术。",
    "不要提及“之前”“上次”“这次返回”“工具返回”“finance-query 返回”“我需要用”等过程或历史修正话术。",
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
    metric: data.metric || payload.metric,
    metric_label: data.metric_label || payload.metric_label,
    business_basis: data.business_basis || payload.business_basis,
    total: data.total ?? payload.total,
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

function requiredBossVisibleAtoms(payload) {
  const atoms = [];
  for (const value of [payload?.period, payload?.metric_label, payload?.total]) {
    if (value === undefined || value === null || value === "") continue;
    const atom = String(value).trim();
    if (atom && !atoms.includes(atom)) atoms.push(atom);
  }
  return atoms;
}

async function financeQuerySystemFacts(question) {
  const result = await callFinanceTool("finance-query", { query: question });
  const payload = compactFinancePayload(parseToolResultPayload(result));
  if (!payload || typeof payload !== "object") return "";
  const lines = [
    "本次核对结果只供生成最终回复使用；不要展示本段标题、JSON 或字段名，可以基于“当前 finance-query 老板答案”自然改写。",
    `最新问题：${question}`
  ];
  if (payload.final_answer) lines.push(`当前 finance-query 老板答案：${payload.final_answer}`);
  if (payload.metric_label) lines.push(`标准指标标签：${payload.metric_label}`);
  if (payload.business_basis) lines.push(`业务口径：${payload.business_basis}`);
  if (payload.metric) lines.push(`标准指标：${payload.metric}`);
  if (payload.total !== undefined && payload.total !== null && payload.total !== "") lines.push(`标准金额：${payload.total}`);
  const requiredAtoms = requiredBossVisibleAtoms(payload);
  if (requiredAtoms.length) lines.push(`老板可见回复必须出现的精确片段：${JSON.stringify(requiredAtoms)}`);
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
      const protectedQuestion = name === "finance-query" ? latestFinanceQuestionForTool : "";
      if (name === "finance-query") latestFinanceQuestionForTool = "";
      const rawParamsObject = rawParams && typeof rawParams === "object" ? rawParams : {};
      const rawQuery = name === "finance-query" ? userVisibleText(rawParamsObject.query || "") : "";
      const shouldUseProtectedQuestion = protectedQuestion && (
        !rawQuery ||
        isRetryOrContinuation(rawQuery) ||
        isContextDependentFinanceFollowup(rawQuery) ||
        shouldPreferProtectedFinanceQuestion(protectedQuestion, rawQuery)
      );
      const params = name === "finance-query"
        ? (rawQuery || shouldUseProtectedQuestion
          ? { ...rawParamsObject, query: shouldUseProtectedQuestion ? protectedQuestion : rawQuery }
          : rawParams)
        : rawParams;
      return callFinanceTool(name, params);
    }
  };
}

const plugin = {
  id: PLUGIN_ID,
  name: "Finance",
  description: "Finance MCP plugin (native, no Python bridge)",
  register(api) {
    pluginRuntime = api;

    api.registerTool(createFinanceTool(
      "finance-query",
      "Boss finance QA. Call this first for finance questions. When the returned JSON has final_answer or boss_reply_text, use final_answer as the factual source; you may rephrase for clarity, but preserve exact amounts, period, business basis, uncertainty, source notes, and source update time. When it has contract_continuity_candidates, describe them as same-project candidates/references, not a confirmed counterparty mapping.",
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
      latestFinanceQuestionForTool = latestQuestion || "";
      if (!latestQuestion) return undefined;
      const financeFacts = await financeQuerySystemFacts(latestQuestion);
      return {
        prependSystemContext: mustCallFinanceQuerySystemContext(latestQuestion, financeFacts)
      };
    });
  }
};

export { plugin as default, normalizePluginConfig, RemoteMCPClient };
