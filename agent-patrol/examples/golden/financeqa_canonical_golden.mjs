#!/usr/bin/env node
import fs from "node:fs";

const PERIOD_START = { year: 2025, month: 10 };

const TEMPLATE_DEFINITIONS = {
  finance_latest_month_revenue: {
    metric: "项目结算",
    query: () => "收入表中最新月份的营收是多少？",
    amountPaths: [
      ["data", "metrics", "项目结算"],
      ["data", "metrics", "营收"],
      ["data", "metrics", "收入"],
      ["data", "revenue"],
      ["data", "revenue_amount"]
    ],
    amountLabels: ["项目结算", "营收", "收入"]
  },
  finance_project_receivable_unpaid: {
    metric: "项目应收",
    query: ({ end }) => `${formatZhMonth(PERIOD_START)}至${formatZhMonth(end)}，所有项目的应收未收是多少？`,
    amountPaths: [
      ["data", "contract_summary", "receivable_amount"],
      ["data", "project_summary", "receivable_amount"],
      ["data", "metrics", "项目应收"],
      ["data", "metrics", "应收未收"],
      ["data", "metrics", "应收"]
    ],
    amountLabels: ["项目应收", "应收未收", "应收"]
  },
  finance_project_invoiced_receivable_unpaid: {
    metric: "已开票未回款",
    query: ({ end }) => `${formatZhMonth(PERIOD_START)}至${formatZhMonth(end)}，所有项目已开票未回款是多少？`,
    amountPaths: [
      ["data", "contract_summary", "invoiced_receivable_unpaid_amount"],
      ["data", "project_summary", "invoiced_receivable_unpaid_amount"],
      ["data", "metrics", "已开票未回款"],
      ["data", "metrics", "已开票未收款"]
    ],
    amountLabels: ["已开票未回款", "已开票未收款"]
  },
  finance_project_payable_unpaid: {
    metric: "项目应付",
    query: ({ end }) => `${formatZhMonth(PERIOD_START)}至${formatZhMonth(end)}，所有项目的应付未付是多少？`,
    amountPaths: [
      ["data", "contract_summary", "payable_amount"],
      ["data", "project_summary", "payable_amount"],
      ["data", "metrics", "项目应付"],
      ["data", "metrics", "应付未付"],
      ["data", "metrics", "项目成本"]
    ],
    amountLabels: ["项目应付", "应付未付", "项目成本"]
  },
  finance_project_invoiced_payable_unpaid: {
    metric: "已收票未付款",
    query: ({ end }) => `${formatZhMonth(PERIOD_START)}至${formatZhMonth(end)}，所有项目已收票未付款是多少？`,
    amountPaths: [
      ["data", "contract_summary", "invoiced_payable_unpaid_amount"],
      ["data", "project_summary", "invoiced_payable_unpaid_amount"],
      ["data", "metrics", "已收票未付款"],
      ["data", "metrics", "已收票但未付款"]
    ],
    amountLabels: ["已收票未付款", "已收票但未付款"]
  },
  finance_unpaid_projects: {
    metric: "项目应付",
    query: ({ end }) => `${formatZhMonth(PERIOD_START)}至${formatZhMonth(end)}，按项目列出应付未付金额。`,
    amountPaths: [
      ["data", "contract_summary", "payable_amount"],
      ["data", "project_summary", "payable_amount"],
      ["data", "metrics", "未付款"],
      ["data", "metrics", "项目应付"],
      ["data", "metrics", "应付未付"],
      ["data", "metrics", "项目成本"]
    ],
    amountLabels: ["项目应付", "应付未付", "未付款", "项目成本"]
  }
};

async function main() {
  try {
    const args = parseArgs(process.argv.slice(2));
    const definition = TEMPLATE_DEFINITIONS[args.template];
    if (!definition) {
      throw new Error(`unsupported FinanceQA golden template: ${args.template || "(missing)"}`);
    }
    const question = readQuestionForAudit(args.questionFile);
    const asOf = parseDate(args.asOfDate ?? todayIsoDate());
    const end = previousCompleteMonth(asOf);
    const canonicalQuery = definition.query({ asOf, end });
    const mcpPayload = await callFinanceQuery({
      url: readEnv("FINANCEQA_MCP_URL"),
      token: readToken(),
      query: canonicalQuery,
      id: `financeqa-golden-${args.template}`,
      timeoutMs: Number(args.timeoutMs ?? 120_000)
    });
    const financePayload = unwrapFinancePayload(mcpPayload);
    const finalAnswer = extractFinalAnswer(financePayload);
    if (!finalAnswer) {
      throw new Error("FinanceQA golden command could not extract final_answer from MCP response");
    }
    const amount = extractAmount(financePayload, definition);
    if (amount === undefined) {
      throw new Error(`FinanceQA golden command could not extract amount for metric ${definition.metric}`);
    }
    const period = extractPeriod(financePayload, { start: PERIOD_START, end });
    const source = extractSource(financePayload, finalAnswer);

    writeJson({
      result: {
        source: "financeqa_canonical_golden",
        template: args.template,
        canonical_query: canonicalQuery,
        final_answer: finalAnswer,
        structured: {
          metric: definition.metric,
          amount,
          period,
          source
        },
        audit: {
          question_file: args.questionFile,
          original_question_length: question.length,
          as_of_date: `${asOf.year}-${pad2(asOf.month)}-${pad2(asOf.day)}`
        },
        raw: financePayload
      }
    });
  } catch (err) {
    console.error(err instanceof Error ? err.message : String(err));
    process.exit(1);
  }
}

function parseArgs(argv) {
  const parsed = {};
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index];
    if (!item.startsWith("--")) {
      throw new Error(`unexpected argument: ${item}`);
    }
    const key = item.slice(2).replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
    const value = argv[index + 1];
    if (!value || value.startsWith("--")) {
      throw new Error(`missing value for ${item}`);
    }
    parsed[key] = value;
    index += 1;
  }
  if (!parsed.template) throw new Error("missing --template");
  if (!parsed.questionFile) throw new Error("missing --question-file");
  return parsed;
}

function readQuestionForAudit(questionFile) {
  if (!questionFile) return "";
  return fs.readFileSync(questionFile, "utf8");
}

function readEnv(name) {
  const value = process.env[name];
  if (!value || value.includes("${")) {
    throw new Error(`${name} is not configured`);
  }
  return value;
}

function readToken() {
  if (process.env.FINANCEQA_MCP_READ_TOKEN) {
    return process.env.FINANCEQA_MCP_READ_TOKEN;
  }
  const file = process.env.FINANCEQA_MCP_READ_TOKEN_FILE;
  if (!file) return undefined;
  return fs.readFileSync(file, "utf8").trim();
}

async function callFinanceQuery(input) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), input.timeoutMs);
  const headers = {
    "content-type": "application/json",
    accept: "application/json"
  };
  if (input.token) {
    headers.authorization = `Bearer ${input.token}`;
  }
  let response;
  let text;
  try {
    response = await fetch(input.url, {
      method: "POST",
      headers,
      signal: controller.signal,
      body: JSON.stringify({
        jsonrpc: "2.0",
        id: input.id,
        method: "tools/call",
        params: {
          name: "finance-query",
          arguments: {
            query: input.query
          }
        }
      })
    });
    text = await response.text();
  } catch (err) {
    if (err && err.name === "AbortError") {
      throw new Error(`FinanceQA golden MCP call timed out after ${input.timeoutMs}ms`);
    }
    throw err;
  } finally {
    clearTimeout(timer);
  }
  if (!response.ok) {
    throw new Error(`FinanceQA golden MCP call returned HTTP ${response.status}: ${text.slice(0, 500)}`);
  }
  if (!text.trim()) {
    throw new Error("FinanceQA golden MCP call returned empty JSON");
  }
  return JSON.parse(text);
}

function unwrapFinancePayload(value) {
  const result = asRecord(value)?.result;
  const content = asRecord(result)?.content;
  if (Array.isArray(content)) {
    for (const item of content) {
      const text = asString(asRecord(item)?.text) ?? asString(item);
      if (!text) continue;
      const parsed = parseJsonText(text);
      if (parsed !== undefined) return parsed;
      return { final_answer: text };
    }
  }
  const structuredContent = asRecord(result)?.structuredContent;
  if (structuredContent) return structuredContent;
  return value;
}

function extractFinalAnswer(value) {
  const record = asRecord(value);
  for (const key of ["final_answer", "finalAnswer", "answer", "boss_reply_text", "message", "text", "output"]) {
    const answer = asString(record?.[key]);
    if (answer) return answer;
  }
  return undefined;
}

function extractAmount(payload, definition) {
  for (const path of definition.amountPaths) {
    const amount = numericValue(getPath(payload, path));
    if (amount !== undefined) return amount;
  }
  const labelAmount = findAmountByLabel(payload, definition.amountLabels);
  if (labelAmount !== undefined) return labelAmount;
  return extractAmountFromText(extractFinalAnswer(payload) ?? "", definition.amountLabels);
}

function getPath(value, path) {
  let current = value;
  for (const key of path) {
    current = asRecord(current)?.[key];
    if (current === undefined) return undefined;
  }
  return current;
}

function findAmountByLabel(value, labels) {
  const queue = [value];
  while (queue.length > 0) {
    const current = queue.shift();
    const record = asRecord(current);
    if (!record) continue;
    for (const [key, val] of Object.entries(record)) {
      if (labels.some((label) => key.includes(label))) {
        const amount = numericValue(val);
        if (amount !== undefined) return amount;
      }
      if (val && typeof val === "object") queue.push(val);
    }
  }
  return undefined;
}

function extractAmountFromText(text, labels) {
  for (const label of labels) {
    const escaped = escapeRegExp(label);
    const after = new RegExp(`${escaped}[^\\d\\-]{0,30}([\\-]?[\\d,]+(?:\\.\\d+)?)\\s*(?:元|万元)?`);
    const before = new RegExp(`([\\-]?[\\d,]+(?:\\.\\d+)?)\\s*(?:元|万元)[^\\n，。；;]{0,30}${escaped}`);
    for (const pattern of [after, before]) {
      const match = text.match(pattern);
      if (!match) continue;
      const amount = Number(match[1].replace(/,/g, ""));
      if (Number.isFinite(amount)) {
        return match[0].includes("万元") ? amount * 10_000 : amount;
      }
    }
  }
  return undefined;
}

function extractPeriod(payload, fallback) {
  const data = asRecord(asRecord(payload)?.data) ?? asRecord(payload);
  const from = asString(data?.period_from) ?? asString(data?.periodStart) ?? asString(data?.from) ?? formatIsoMonth(fallback.start);
  const to = asString(data?.period_to) ?? asString(data?.periodEnd) ?? asString(data?.to) ?? formatIsoMonth(fallback.end);
  return { from, to };
}

function extractSource(payload, finalAnswer) {
  const data = asRecord(asRecord(payload)?.data) ?? asRecord(payload);
  const source = asString(data?.source_note) ?? asString(data?.source) ?? asString(data?.sourceNote);
  if (source) return source;
  const match = finalAnswer.match(/来源[:：][^。；;\n]+/);
  return match?.[0];
}

function parseDate(value) {
  const match = String(value).match(/^(\d{4})-(\d{1,2})-(\d{1,2})$/);
  if (!match) throw new Error(`invalid --as-of-date: ${value}`);
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (!Number.isInteger(year) || month < 1 || month > 12 || day < 1 || day > 31) {
    throw new Error(`invalid --as-of-date: ${value}`);
  }
  return { year, month, day };
}

function previousCompleteMonth(date) {
  if (date.month === 1) return { year: date.year - 1, month: 12 };
  return { year: date.year, month: date.month - 1 };
}

function todayIsoDate() {
  return new Date().toISOString().slice(0, 10);
}

function formatZhMonth(value) {
  return `${value.year}年${value.month}月`;
}

function formatIsoMonth(value) {
  return `${value.year}-${pad2(value.month)}`;
}

function pad2(value) {
  return String(value).padStart(2, "0");
}

function numericValue(value) {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value !== "string") return undefined;
  const normalized = value.replace(/,/g, "").trim();
  if (!normalized) return undefined;
  const match = normalized.match(/^-?\d+(?:\.\d+)?/);
  if (!match) return undefined;
  const amount = Number(match[0]);
  if (!Number.isFinite(amount)) return undefined;
  return normalized.includes("万元") ? amount * 10_000 : amount;
}

function parseJsonText(text) {
  const trimmed = text.trim();
  if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) return undefined;
  try {
    return JSON.parse(trimmed);
  } catch {
    return undefined;
  }
}

function asRecord(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : undefined;
}

function asString(value) {
  return typeof value === "string" && value.trim() ? value : undefined;
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function writeJson(value) {
  process.stdout.write(`${JSON.stringify(value)}\n`);
}

await main();
