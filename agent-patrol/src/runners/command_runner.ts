import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { AgentEnvelope, AgentSessionEvidence } from "../types.ts";

const MAX_TOOL_RESULT_TEXT_CHARS = 50_000;
const MAX_USER_MESSAGE_TEXT_CHARS = 20_000;

export interface CommandInvocation {
  command: string;
  args: string[];
}

export interface RunCommandAgentOptions {
  commandTemplate: string;
  question: string;
  sessionId: string;
  requireSessionIsolation?: boolean;
  cwd?: string;
  timeoutMs?: number;
  values?: Record<string, string>;
}

export async function runCommandAgent(options: RunCommandAgentOptions): Promise<AgentEnvelope> {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-question-"));
  const questionFile = path.join(tmpDir, "question.txt");
  fs.writeFileSync(questionFile, options.question, "utf8");
  try {
    const invocation = buildCommandInvocation(options.commandTemplate, {
      ...(options.values ?? {}),
      question: options.question,
      questionFile,
      sessionId: options.sessionId
    });
    const stdout = await runProcess(invocation, {
      cwd: options.cwd,
      timeoutMs: options.timeoutMs ?? 120_000
    });
    const envelope = parseAgentEnvelope(stdout);
    attachOpenClawSessionEvidence(envelope);
    validateAgentEnvelope(envelope, {
      requireSessionIsolation: options.requireSessionIsolation,
      expectedSessionId: options.sessionId
    });
    return envelope;
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

export function buildCommandInvocation(template: string, values: Record<string, string>): CommandInvocation {
  const rendered = template.replace(/\{([a-zA-Z0-9_]+)\}/g, (_match, key: string) => values[key] ?? "");
  const argv = splitArgs(rendered);
  if (argv.length === 0) {
    throw new Error("command template rendered to an empty command");
  }
  return { command: argv[0]!, args: argv.slice(1) };
}

export function runProcess(invocation: CommandInvocation, options: { cwd?: string; timeoutMs: number }): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn(invocation.command, invocation.args, {
      cwd: options.cwd,
      stdio: ["ignore", "pipe", "pipe"]
    });
    let stdout = "";
    let stderr = "";
    const timer = setTimeout(() => {
      child.kill("SIGTERM");
      reject(new Error(`agent command timed out after ${options.timeoutMs}ms`));
    }, options.timeoutMs);
    child.stdout.on("data", (chunk: Buffer) => {
      stdout += String(chunk);
    });
    child.stderr.on("data", (chunk: Buffer) => {
      stderr += String(chunk);
    });
    child.on("error", (err: Error) => {
      clearTimeout(timer);
      reject(err);
    });
    child.on("close", (code: number | null) => {
      clearTimeout(timer);
      if (code === 0) {
        resolve(stdout);
      } else {
        reject(new Error(`agent command exited ${code}: ${stderr || stdout}`));
      }
    });
  });
}

export function parseAgentEnvelope(stdout: string): AgentEnvelope {
  const parsed = parseJsonFromStdout(stdout);
  const result = asRecord(parsed.result) ?? parsed;
  const meta = asRecord(result.meta);
  const agentMeta = asRecord(meta?.agentMeta);
  const systemPromptReport = asRecord(meta?.systemPromptReport);
  const answer = extractAnswer(result);
  return {
    source: "agent",
    answer,
    sessionId: stringValue(result.sessionId) ?? stringValue(result.session_id) ?? stringValue(agentMeta?.sessionId),
    sessionKey: stringValue(result.sessionKey) ?? stringValue(result.session_key) ?? stringValue(systemPromptReport?.sessionKey),
    toolCalls: Array.isArray(result.toolCalls) ? result.toolCalls as AgentEnvelope["toolCalls"] : [],
    raw: parsed
  };
}

function attachOpenClawSessionEvidence(envelope: AgentEnvelope): void {
  const sessionDir = process.env.AGENT_PATROL_OPENCLAW_SESSION_DIR;
  const sessionId = envelope.sessionId;
  if (!sessionDir || !sessionId) return;

  const sessionFile = path.join(sessionDir, `${sessionId}.jsonl`);
  const evidence = readOpenClawSessionEvidence(sessionFile);
  if (!evidence) return;

  envelope.sessionEvidence = evidence;
  if ((!envelope.toolCalls || envelope.toolCalls.length === 0) && evidence.toolCalls?.length) {
    envelope.toolCalls = evidence.toolCalls.map((call) => ({
      id: call.id,
      name: call.name,
      arguments: call.arguments
    }));
  }
}

function readOpenClawSessionEvidence(sessionFile: string): AgentSessionEvidence | undefined {
  if (!fs.existsSync(sessionFile)) return undefined;

  const evidence: AgentSessionEvidence = { sessionFile, userMessages: [], toolCalls: [], toolResults: [] };
  const parseErrors: string[] = [];
  const lines = fs.readFileSync(sessionFile, "utf8").split(/\r?\n/).filter((line) => line.trim());
  for (let index = 0; index < lines.length; index += 1) {
    try {
      const row = JSON.parse(lines[index]!) as Record<string, unknown>;
      collectSessionMessageEvidence(row, evidence);
    } catch (err) {
      parseErrors.push(`line ${index + 1}: ${err instanceof Error ? err.message : String(err)}`);
    }
  }
  if (parseErrors.length > 0) evidence.parseErrors = parseErrors;
  if (evidence.userMessages?.length === 0) delete evidence.userMessages;
  if (evidence.toolCalls?.length === 0) delete evidence.toolCalls;
  if (evidence.toolResults?.length === 0) delete evidence.toolResults;
  return evidence;
}

function collectSessionMessageEvidence(row: Record<string, unknown>, evidence: AgentSessionEvidence): void {
  const message = asRecord(row.message);
  if (!message) return;

  if (message.role === "user") {
    const text = extractTextBlocks(message.content);
    if (text) {
      const truncated = text.length > MAX_USER_MESSAGE_TEXT_CHARS;
      evidence.userMessages?.push({
        timestamp: stringValue(row.timestamp),
        truncated,
        text: truncated ? text.slice(0, MAX_USER_MESSAGE_TEXT_CHARS) : text
      });
    }
  }

  if (message.role === "assistant") {
    const content = Array.isArray(message.content) ? message.content : [];
    for (const item of content) {
      const block = asRecord(item);
      if (block?.type !== "toolCall") continue;
      evidence.toolCalls?.push({
        id: stringValue(block.id),
        name: stringValue(block.name),
        arguments: block.arguments
      });
    }
  }

  if (message.role === "toolResult") {
    const text = extractTextBlocks(message.content);
    const result: NonNullable<AgentSessionEvidence["toolResults"]>[number] = {
      toolCallId: stringValue(message.toolCallId),
      toolName: stringValue(message.toolName)
    };
    if (text) {
      result.truncated = text.length > MAX_TOOL_RESULT_TEXT_CHARS;
      result.text = result.truncated ? text.slice(0, MAX_TOOL_RESULT_TEXT_CHARS) : text;
      result.json = parseJsonText(text);
    }
    evidence.toolResults?.push(result);
  }
}

function extractTextBlocks(value: unknown): string | undefined {
  if (typeof value === "string") return value;
  if (!Array.isArray(value)) return undefined;
  const parts = value
    .map((item) => {
      const block = asRecord(item);
      return block?.type === "text" ? stringValue(block.text) : undefined;
    })
    .filter((item): item is string => Boolean(item));
  return parts.length > 0 ? parts.join("\n") : undefined;
}

function parseJsonText(text: string): unknown | undefined {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

export function validateAgentEnvelope(
  envelope: Partial<AgentEnvelope>,
  options: { requireSessionIsolation?: boolean; expectedSessionId?: string } = {}
): asserts envelope is AgentEnvelope {
  if (envelope.source !== "agent") {
    throw new Error("missing actual agent envelope; direct MCP output is not a valid actual path");
  }
  if (!envelope.answer || !envelope.answer.trim()) {
    throw new Error("actual agent envelope has no answer");
  }
  if (options.requireSessionIsolation) {
    const session = envelope.sessionId ?? envelope.sessionKey;
    if (!session) {
      throw new Error("agent session isolation could not be verified");
    }
    if (envelope.sessionKey === "agent:main:main" || session === "agent:main:main") {
      throw new Error("agent session isolation failed: protected main session reused");
    }
    if (options.expectedSessionId && envelope.sessionId !== options.expectedSessionId && envelope.sessionKey !== options.expectedSessionId) {
      throw new Error(`agent session isolation failed: expected ${options.expectedSessionId}, got ${session}`);
    }
  }
}

function parseJsonFromStdout(stdout: string): Record<string, unknown> {
  const trimmed = stdout.trim();
  if (!trimmed) {
    throw new Error("empty command stdout");
  }
  try {
    return JSON.parse(trimmed) as Record<string, unknown>;
  } catch {
    // Agent CLIs often print warnings before pretty JSON. Keep parsing strict:
    // only accept the complete JSON object between the first "{" and last "}".
  }
  const start = trimmed.indexOf("{");
  const end = trimmed.lastIndexOf("}");
  if (start < 0 || end <= start) {
    throw new Error("command stdout did not contain JSON");
  }
  return JSON.parse(trimmed.slice(start, end + 1)) as Record<string, unknown>;
}

function extractAnswer(result: Record<string, unknown>): string {
  for (const key of ["answer", "final_answer", "output"]) {
    const value = stringValue(result[key]);
    if (value) return value;
  }
  const payloads = result.payloads;
  if (Array.isArray(payloads)) {
    return payloads
      .map((item) => stringValue(asRecord(item)?.text))
      .filter(Boolean)
      .join("\n\n");
  }
  return "";
}

function splitArgs(command: string): string[] {
  const args: string[] = [];
  let current = "";
  let quote: "'" | "\"" | null = null;
  for (let i = 0; i < command.length; i += 1) {
    const char = command[i]!;
    if ((char === "'" || char === "\"") && quote === null) {
      quote = char;
      continue;
    }
    if (char === quote) {
      quote = null;
      continue;
    }
    if (/\s/.test(char) && quote === null) {
      if (current) {
        args.push(current);
        current = "";
      }
      continue;
    }
    current += char;
  }
  if (current) args.push(current);
  return args;
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value : undefined;
}
