#!/usr/bin/env node
import fs from "node:fs";
import { spawn } from "node:child_process";

const DEFAULT_TIMEOUT_MS = 120_000;

const args = parseArgs(process.argv.slice(2));

main().catch((error) => {
  console.error(error instanceof Error ? error.stack ?? error.message : String(error));
  process.exit(2);
});

async function main() {
  const input = await readInput(args.input);
  const command = process.env.AGENT_PATROL_LLM_CMD;
  if (!command?.trim()) {
    throw new Error("AGENT_PATROL_LLM_CMD is required");
  }

  const prompt = buildPrompt(input);
  const stdout = await runLlmCommand(command, prompt, Number(process.env.AGENT_PATROL_LLM_TIMEOUT_MS) || DEFAULT_TIMEOUT_MS);
  const parsed = parseLlmJson(stdout);
  const questions = normalizeQuestions(parsed);

  console.log(JSON.stringify({
    source: "llm_command_rewriter",
    questions
  }, null, 2));
}

function parseArgs(argv) {
  const flags = {};
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index];
    if (!item?.startsWith("--")) continue;
    const key = item.slice(2);
    const next = argv[index + 1];
    if (!next || next.startsWith("--")) {
      flags[key] = true;
    } else {
      flags[key] = next;
      index += 1;
    }
  }
  return flags;
}

async function readInput(inputFile) {
  if (typeof inputFile === "string" && inputFile) {
    return JSON.parse(fs.readFileSync(inputFile, "utf8"));
  }
  const stdin = await readStdin();
  if (!stdin.trim()) throw new Error("--input or stdin JSON is required");
  return JSON.parse(stdin);
}

function readStdin() {
  return new Promise((resolve, reject) => {
    let input = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => {
      input += chunk;
    });
    process.stdin.on("error", reject);
    process.stdin.on("end", () => resolve(input));
  });
}

function buildPrompt(input) {
  const cases = Array.isArray(input?.cases) ? input.cases : [];
  return [
    "你是 FinanceQA 日常巡检的问题改写器。",
    "任务：把每个 input case 的 originalQuestion 改写成更像老板日常提问的中文自然语言问题。",
    "硬性规则：",
    "1. 只改写问法，不改变 caseId、template、财务口径、指标、时间范围、实体范围或期望答案。",
    "2. 必须保持只读查询语义，不要要求上传、同步、删除、修改、重启、发送、推送、导入、导出或更新数据。",
    "3. 每个 question 不超过 280 个中文字符。",
    "4. 不要输出解释、Markdown 或额外文本，只输出 JSON。",
    "5. 输出结构必须是 {\"questions\":[{\"caseId\":\"...\",\"template\":\"...\",\"question\":\"...\"}]}。",
    "6. 可以参考 seed 做不同轮次的问法变化，但不要把 seed 写进问题。",
    "7. 如果 case 带有 questionAnchors，每一组锚点都必须至少保留一个等价词在改写后的 question 中。",
    "可选风格：可以使用“老板”“帮我看下”“从项目口径看”“上个完整自然月”等自然说法，但不要改变原始时间语义。",
    "",
    "<cases_json>",
    JSON.stringify({ ...input, cases }, null, 2),
    "</cases_json>"
  ].join("\n");
}

function runLlmCommand(command, prompt, timeoutMs) {
  return new Promise((resolve, reject) => {
    const child = spawn("bash", ["-lc", command], {
      stdio: ["pipe", "pipe", "pipe"]
    });
    let stdout = "";
    let stderr = "";
    const timer = setTimeout(() => {
      child.kill("SIGTERM");
      reject(new Error(`LLM question command timed out after ${timeoutMs}ms`));
    }, timeoutMs);
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
    child.on("close", (code) => {
      clearTimeout(timer);
      if (code === 0) {
        resolve(stdout);
      } else {
        reject(new Error(`LLM question command exited ${code}: ${stderr || stdout}`));
      }
    });
    child.stdin.end(prompt);
  });
}

function parseLlmJson(stdout) {
  const trimmed = stdout.trim();
  if (!trimmed) throw new Error("LLM question command returned empty stdout");

  const direct = tryParseJson(trimmed);
  if (direct !== undefined) {
    const embedded = extractAgentAnswerText(direct);
    if (embedded) return parseLlmJson(embedded);
    return direct;
  }

  const fenced = trimmed.match(/```(?:json)?\s*([\s\S]*?)```/i);
  if (fenced?.[1]) {
    return JSON.parse(fenced[1].trim());
  }

  const objectJson = extractJsonSlice(trimmed, "{", "}");
  if (objectJson) return parseLlmJson(objectJson);
  const arrayJson = extractJsonSlice(trimmed, "[", "]");
  if (arrayJson) return parseLlmJson(arrayJson);
  throw new Error("LLM question command stdout did not contain JSON");
}

function tryParseJson(text) {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

function extractAgentAnswerText(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const result = value.result && typeof value.result === "object" && !Array.isArray(value.result)
    ? value.result
    : value;
  for (const key of ["answer", "final_answer", "output"]) {
    if (typeof result[key] === "string" && result[key].trim()) return result[key];
  }
  if (Array.isArray(result.payloads)) {
    const text = result.payloads
      .map((item) => item && typeof item === "object" && typeof item.text === "string" ? item.text : undefined)
      .filter(Boolean)
      .join("\n\n");
    return text || undefined;
  }
  return undefined;
}

function extractJsonSlice(text, open, close) {
  const start = text.indexOf(open);
  const end = text.lastIndexOf(close);
  if (start < 0 || end <= start) return undefined;
  return text.slice(start, end + 1);
}

function normalizeQuestions(parsed) {
  const rawQuestions = Array.isArray(parsed) ? parsed : parsed?.questions;
  if (!Array.isArray(rawQuestions)) return [];
  return rawQuestions
    .map((item) => ({
      caseId: typeof item?.caseId === "string" ? item.caseId : "",
      template: typeof item?.template === "string" ? item.template : undefined,
      question: typeof item?.question === "string" ? item.question.trim() : ""
    }))
    .filter((item) => item.caseId && item.question);
}
