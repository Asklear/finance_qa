#!/usr/bin/env node
import fs from "node:fs";

const DEFAULT_TIMEOUT_MS = 120_000;

main().catch((error) => {
  console.error(error instanceof Error ? error.stack ?? error.message : String(error));
  process.exit(2);
});

async function main() {
  loadEnvFile(process.env.AGENT_PATROL_LLM_ENV_FILE);

  const prompt = await readStdin();
  if (!prompt.trim()) throw new Error("stdin prompt is required");

  const apiKey = envValue("AGENT_PATROL_LLM_API_KEY", "AGENT_PATROL_LLM_API_KEY_ENV")
    ?? process.env.OPENAI_API_KEY
    ?? process.env.DEEPSEEK_API_KEY;
  const baseUrl = envValue("AGENT_PATROL_LLM_BASE_URL", "AGENT_PATROL_LLM_BASE_URL_ENV")
    ?? process.env.OPENAI_BASE_URL
    ?? process.env.DEEPSEEK_BASE_URL;
  const model = envValue("AGENT_PATROL_LLM_MODEL", "AGENT_PATROL_LLM_MODEL_ENV")
    ?? process.env.OPENAI_MODEL
    ?? process.env.DEEPSEEK_MODEL
    ?? "deepseek-chat";

  if (!apiKey) throw new Error("AGENT_PATROL_LLM_API_KEY, OPENAI_API_KEY, or DEEPSEEK_API_KEY is required");
  if (!baseUrl) throw new Error("AGENT_PATROL_LLM_BASE_URL, OPENAI_BASE_URL, or DEEPSEEK_BASE_URL is required");

  const response = await callChatCompletions({
    url: chatCompletionsUrl(baseUrl),
    apiKey,
    model,
    prompt,
    timeoutMs: Number(process.env.AGENT_PATROL_LLM_TIMEOUT_MS) || DEFAULT_TIMEOUT_MS
  });

  process.stdout.write(response.trim() + "\n");
}

function loadEnvFile(filePath) {
  if (!filePath) return;
  if (!fs.existsSync(filePath)) throw new Error(`AGENT_PATROL_LLM_ENV_FILE not found: ${filePath}`);
  const lines = fs.readFileSync(filePath, "utf8").split(/\r?\n/);
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const match = trimmed.match(/^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$/);
    if (!match) continue;
    const key = match[1];
    if (process.env[key] !== undefined) continue;
    process.env[key] = unquoteEnvValue(match[2] ?? "");
  }
}

function unquoteEnvValue(value) {
  const trimmed = value.trim();
  if ((trimmed.startsWith("\"") && trimmed.endsWith("\"")) || (trimmed.startsWith("'") && trimmed.endsWith("'"))) {
    return trimmed.slice(1, -1);
  }
  return trimmed;
}

function envValue(name, indirectionName) {
  if (process.env[name]) return process.env[name];
  const envName = process.env[indirectionName];
  if (envName && process.env[envName]) return process.env[envName];
  return undefined;
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

function chatCompletionsUrl(baseUrl) {
  const base = baseUrl.replace(/\/+$/, "");
  if (base.endsWith("/chat/completions")) return base;
  if (/\/v\d+$/i.test(base)) return `${base}/chat/completions`;
  return `${base}/v1/chat/completions`;
}

async function callChatCompletions(options) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), options.timeoutMs);
  try {
    const response = await fetch(options.url, {
      method: "POST",
      headers: {
        "authorization": `Bearer ${options.apiKey}`,
        "content-type": "application/json"
      },
      body: JSON.stringify({
        model: options.model,
        messages: [{ role: "user", content: options.prompt }],
        temperature: Number(process.env.AGENT_PATROL_LLM_TEMPERATURE ?? "0.7"),
        max_tokens: Number(process.env.AGENT_PATROL_LLM_MAX_TOKENS ?? "1024")
      }),
      signal: controller.signal
    });
    const text = await response.text();
    if (!response.ok) {
      throw new Error(`OpenAI-compatible LLM request failed ${response.status}: ${text.slice(0, 500)}`);
    }
    const payload = JSON.parse(text);
    const content = payload?.choices?.[0]?.message?.content
      ?? payload?.choices?.[0]?.text
      ?? payload?.output_text;
    if (typeof content !== "string" || !content.trim()) {
      throw new Error("OpenAI-compatible LLM response did not contain message content");
    }
    return content;
  } finally {
    clearTimeout(timer);
  }
}
