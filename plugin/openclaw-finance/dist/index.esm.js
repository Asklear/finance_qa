const PLUGIN_ID = "openclaw-finance";
const BRIDGE_PATH = "/root/.openclaw/extensions/openclaw-finance/server/finance_bridge.py";

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
    "After `finance-query`, parse `content[0].text` as JSON and answer from the current tool result. Prefer deterministic fields such as `final_answer`, `boss_reply_text`, `boss_reply`, `message`, and structured data, but do not blindly repeat a fallback or missing-subject message if the payload contains facts that let you answer the user's question.",
    "When `final_answer` or `boss_reply_text` exists, preserve its key amounts, uncertainty wording, and source note; do not omit the source note.",
    "If the current tool result includes `contract_continuity_candidates`, use those candidates as same-project continuity evidence and make a tentative business inference with uncertainty; call it a same-project candidate/reference and do not state that the counterparty definitely changed or became associated.",
    "Keep the source note from the tool result. Do not expose internal IDs, SQL, route traces, or contract IDs unless the user explicitly asks for technical details."
  ].join("\n");
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
      "Boss finance QA. For finance questions, call this first and answer from the current returned facts and source notes.",
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

    api.on("before_prompt_build", (event) => {
      const prompt = userVisibleText(event?.prompt || "");
      if (!isFinanceQuestion(prompt)) return undefined;
      return { prependSystemContext: mustCallFinanceQuerySystemContext() };
    });
  }
};

export { plugin as default };
