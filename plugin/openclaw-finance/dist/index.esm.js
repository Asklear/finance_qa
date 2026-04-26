const PLUGIN_ID = "openclaw-finance";
const BRIDGE_PATH = "/root/.openclaw/extensions/openclaw-finance/server/finance_bridge.py";
const forcedAnswersBySessionKey = new Map();

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
    "After `finance-query`, parse `content[0].text` as JSON. If `final_answer` exists, return `final_answer` unchanged. Otherwise return `boss_reply_text` unchanged, then `boss_reply`, then `message`.",
    "Keep the source note from the tool result. Do not expose internal IDs, SQL, route traces, or contract IDs unless the returned final answer explicitly contains them."
  ].join("\n");
}

function forcedFinalAnswerSystemContext() {
  return [
    mustCallFinanceQuerySystemContext(),
    "For this turn, finance-query has already been executed by the openclaw-finance hook.",
    "When a FINANCE_QUERY_FINAL_ANSWER block is present, do not call tools again and do not use conversation history.",
    "Your entire visible response must equal the text inside FINANCE_QUERY_FINAL_ANSWER_START/END exactly."
  ].join("\n");
}

function finalAnswerPromptContext(finalAnswer) {
  return [
    "FINANCE_QUERY_FINAL_ANSWER_START",
    finalAnswer,
    "FINANCE_QUERY_FINAL_ANSWER_END"
  ].join("\n");
}

function financePayloadPromptContext(payload) {
  return [
    "FINANCE_QUERY_PAYLOAD_START",
    JSON.stringify(payload, null, 2),
    "FINANCE_QUERY_PAYLOAD_END"
  ].join("\n");
}

function isBridgeFallbackPayload(payload) {
  if (!payload || typeof payload !== "object") return false;
  if (payload.success === false) return true;
  if (payload.answer_method === "llm_payload") return true;
  const data = payload.data && typeof payload.data === "object" ? payload.data : {};
  return data.answer_method === "llm_payload" || Boolean(data.llm_payload);
}

function fallbackPayloadSystemContext() {
  return [
    mustCallFinanceQuerySystemContext(),
    "For this turn, finance-query has already returned a FINANCE_QUERY_PAYLOAD block instead of a direct final answer.",
    "Do not answer from prior conversation history or memory.",
    "Reason from the current payload and its llm_payload/source_note only. If the payload contains enough facts, answer the user's question directly in boss-facing language.",
    "If the payload is still insufficient, say exactly what is missing. Keep source notes, and do not expose SQL, internal IDs, route traces, bridge_meta, or raw field names."
  ].join("\n");
}

function hookSessionKey(ctx) {
  return String(ctx?.sessionKey || ctx?.sessionId || "__default__");
}

function assistantMessageWithText(message, text) {
  return {
    ...message,
    content: [
      { type: "text", text }
    ]
  };
}

function parseBridgePayload(result) {
  if (!result || typeof result !== "object") return {};
  const content = Array.isArray(result.content) ? result.content : [];
  const textBlock = content.find((item) => item && item.type === "text" && typeof item.text === "string");
  if (!textBlock) return result;
  try {
    return JSON.parse(textBlock.text);
  } catch {
    return { message: textBlock.text };
  }
}

function bossReplyText(reply) {
  if (!reply || typeof reply !== "object") return "";
  return ["结论", "原因", "建议"]
    .map((key) => String(reply[key] || "").trim())
    .filter(Boolean)
    .join("\n\n");
}

function finalAnswerFromPayload(payload) {
  if (!payload || typeof payload !== "object") return "";
  if (typeof payload.final_answer === "string" && payload.final_answer.trim()) return payload.final_answer.trim();
  if (typeof payload.boss_reply_text === "string" && payload.boss_reply_text.trim()) return payload.boss_reply_text.trim();
  const reply = bossReplyText(payload.boss_reply);
  if (reply) return reply;
  if (typeof payload.message === "string" && payload.message.trim()) return payload.message.trim();
  return "";
}

async function callBridge(name, rawParams) {
  const { spawn } = await import("node:child_process");
  const payload = {
    action: "call",
    name,
    arguments: rawParams || {}
  };
  const child = spawn("python3", [BRIDGE_PATH], {
    stdio: ["pipe", "pipe", "pipe"]
  });
  const stdout = [];
  const stderr = [];
  child.stdout.on("data", (chunk) => stdout.push(chunk));
  child.stderr.on("data", (chunk) => stderr.push(chunk));
  child.stdin.write(JSON.stringify(payload));
  child.stdin.end();
  const code = await new Promise((resolve, reject) => {
    child.on("error", reject);
    child.on("close", resolve);
  });
  if (code !== 0) {
    const message = Buffer.concat(stderr).toString("utf8") || `bridge exited with code ${code}`;
    throw new Error(message);
  }
  const raw = Buffer.concat(stdout).toString("utf8").trim();
  return raw ? JSON.parse(raw) : errorResult("empty bridge response");
}

function createFinanceTool(name, description, parameters) {
  return {
    name,
    label: name,
    description,
    parameters,
    async execute(_toolCallId, rawParams) {
      try {
        return await callBridge(name, rawParams);
      } catch (error) {
        return errorResult(error);
      }
    }
  };
}

const plugin = {
  id: PLUGIN_ID,
  name: "Finance",
  description: "Finance bridge plugin",
  register(api) {
    api.registerTool(createFinanceTool(
      "finance-query",
      "Boss finance QA. For finance questions, call this first and return final_answer unchanged when present.",
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
          filePath: {
            type: "string",
            description: "Path to the Excel file to upload"
          }
        },
        required: ["filePath"]
      }
    ), { name: "finance-upload" });

    api.on("before_prompt_build", async (event, ctx) => {
      const prompt = userVisibleText(event?.prompt || "");
      if (!isFinanceQuestion(prompt)) return undefined;
      try {
        const result = await callBridge("finance-query", { query: prompt });
        const payload = parseBridgePayload(result);
        if (isBridgeFallbackPayload(payload)) {
          return {
            prependSystemContext: fallbackPayloadSystemContext(),
            prependContext: financePayloadPromptContext(payload)
          };
        }
        const finalAnswer = finalAnswerFromPayload(payload);
        if (finalAnswer) {
          forcedAnswersBySessionKey.set(hookSessionKey(ctx), finalAnswer);
          return {
            prependSystemContext: forcedFinalAnswerSystemContext(),
            prependContext: finalAnswerPromptContext(finalAnswer)
          };
        }
      } catch {
        return { prependSystemContext: mustCallFinanceQuerySystemContext() };
      }
      return { prependSystemContext: mustCallFinanceQuerySystemContext() };
    });

    api.on("before_message_write", (event, ctx) => {
      const key = hookSessionKey(ctx);
      const finalAnswer = forcedAnswersBySessionKey.get(key);
      if (!finalAnswer) return undefined;
      const message = event?.message;
      if (!message || message.role !== "assistant") return undefined;
      forcedAnswersBySessionKey.delete(key);
      return { message: assistantMessageWithText(message, finalAnswer) };
    });

    api.on("before_dispatch", async (event) => {
      const question = userVisibleText(event?.body || event?.content || "");
      if (!isFinanceQuestion(question)) return undefined;
      const result = await callBridge("finance-query", { query: question });
      const payload = parseBridgePayload(result);
      const finalAnswer = finalAnswerFromPayload(payload);
      if (finalAnswer && !isBridgeFallbackPayload(payload)) return { handled: true, text: finalAnswer };
      return undefined;
    });
  }
};

export { plugin as default };
