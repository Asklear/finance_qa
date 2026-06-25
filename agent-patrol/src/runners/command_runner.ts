import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { AgentEnvelope } from "../types.ts";

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

function runProcess(invocation: CommandInvocation, options: { cwd?: string; timeoutMs: number }): Promise<string> {
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
