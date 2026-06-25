import type { PatrolCase, ReferenceEnvelope, TargetConfig } from "./types.ts";

export interface ExecuteReferenceInput {
  patrolCase: PatrolCase;
  target: TargetConfig;
}

export async function executeReadonlyReference(input: ExecuteReferenceInput): Promise<ReferenceEnvelope | undefined> {
  if (input.target.oracle.type !== "financeqa_readonly") {
    return undefined;
  }
  const tool = selectFinanceTool(input.target.oracle.allowedTools);
  if (!tool) {
    return {
      source: "financeqa_mcp",
      error: "financeqa_readonly oracle has no allowed read tool"
    };
  }
  try {
    const raw = await callMcpTool({
      url: input.target.oracle.mcpUrl ?? "",
      bearerTokenEnv: input.target.oracle.bearerTokenEnv,
      tool,
      query: input.patrolCase.question,
      id: input.patrolCase.id
    });
    return {
      source: "financeqa_mcp",
      tool,
      answer: extractReferenceAnswer(raw),
      raw
    };
  } catch (err: unknown) {
    return {
      source: "financeqa_mcp",
      tool,
      error: err instanceof Error ? err.message : String(err)
    };
  }
}

function selectFinanceTool(allowedTools: string[]): string | undefined {
  if (allowedTools.includes("finance-query")) return "finance-query";
  return allowedTools[0];
}

async function callMcpTool(input: {
  url: string;
  bearerTokenEnv?: string;
  tool: string;
  query: string;
  id: string;
}): Promise<unknown> {
  if (!input.url || input.url.includes("${")) {
    throw new Error("financeqa oracle mcpUrl is not configured");
  }
  const headers: Record<string, string> = {
    "content-type": "application/json",
    accept: "application/json"
  };
  const bearer = input.bearerTokenEnv ? process.env[input.bearerTokenEnv] : undefined;
  if (bearer) {
    headers.authorization = `Bearer ${bearer}`;
  }
  const response = await fetch(input.url, {
    method: "POST",
    headers,
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: input.id,
      method: "tools/call",
      params: {
        name: input.tool,
        arguments: {
          query: input.query
        }
      }
    })
  });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`financeqa oracle returned HTTP ${response.status}: ${text.slice(0, 500)}`);
  }
  if (!text.trim()) {
    return {};
  }
  return JSON.parse(text) as unknown;
}

function extractReferenceAnswer(value: unknown): string {
  const record = asRecord(value);
  const result = asRecord(record?.result) ?? record;
  const direct = extractDirectText(result);
  if (direct) return direct;

  const content = result?.content;
  if (Array.isArray(content)) {
    const parts = content
      .map((item) => extractContentText(item))
      .filter((item): item is string => Boolean(item));
    if (parts.length > 0) return parts.join("\n\n");
  }

  const structured = asRecord(result?.structuredContent);
  const structuredText = extractDirectText(structured);
  if (structuredText) return structuredText;

  return "";
}

function extractContentText(value: unknown): string | undefined {
  const record = asRecord(value);
  const text = stringValue(record?.text) ?? stringValue(value);
  if (!text) return undefined;
  const parsed = parseJsonText(text);
  if (parsed !== undefined) {
    const parsedText = extractReferenceAnswer(parsed);
    if (parsedText) return parsedText;
  }
  return text;
}

function extractDirectText(record: Record<string, unknown> | undefined): string | undefined {
  if (!record) return undefined;
  for (const key of ["answer", "final_answer", "finalAnswer", "boss_reply_text", "message", "text", "output"]) {
    const value = stringValue(record[key]);
    if (value) return value;
  }
  return undefined;
}

function parseJsonText(text: string): unknown {
  const trimmed = text.trim();
  if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) {
    return undefined;
  }
  try {
    return JSON.parse(trimmed) as unknown;
  } catch {
    return undefined;
  }
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value : undefined;
}
