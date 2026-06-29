import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { buildCommandInvocation, type CommandInvocation } from "./runners/command_runner.ts";
import type { PatrolCase, TargetConfig } from "./types.ts";

const MAX_GENERATED_QUESTION_CHARS = 280;
const UNSAFE_QUESTION_PATTERN = /(?:上传|同步|删除|修改|创建|重启|发送|发给|推送|导入|导出|写入|更新数据|sync|upload|delete|restart)/i;

export interface GeneratedQuestion {
  caseId: string;
  template?: string;
  question: string;
}

export interface QuestionGeneratorEnvelope {
  source?: string;
  questions?: GeneratedQuestion[];
  error?: string;
  raw?: unknown;
}

export interface ExecuteQuestionGeneratorInput {
  targetName: string;
  target: TargetConfig;
  suite: string;
  seed: string;
  cases: PatrolCase[];
}

export type ExecuteQuestionGenerator = (input: ExecuteQuestionGeneratorInput) => Promise<QuestionGeneratorEnvelope | undefined>;

export async function applyQuestionGenerators(
  cases: PatrolCase[],
  options: {
    targets: Record<string, TargetConfig>;
    suite: string;
    seed: string;
    executeQuestionGenerator?: ExecuteQuestionGenerator;
  }
): Promise<PatrolCase[]> {
  const executeQuestionGenerator = options.executeQuestionGenerator ?? executeCommandQuestionGenerator;
  const byTarget = groupCasesByTarget(cases);
  const rewritten: PatrolCase[] = [];
  for (const [targetName, targetCases] of byTarget) {
    const target = options.targets[targetName];
    if (!target?.questionGenerator) {
      rewritten.push(...targetCases);
      continue;
    }
    const generated = await safeExecuteQuestionGenerator({
      executeQuestionGenerator,
      targetName,
      target,
      suite: options.suite,
      seed: options.seed,
      cases: targetCases
    });
    rewritten.push(...applyGeneratedQuestions(targetCases, generated));
  }
  return cases.map((item) => rewritten.find((candidate) => candidate.id === item.id) ?? item);
}

export async function executeCommandQuestionGenerator(
  input: ExecuteQuestionGeneratorInput
): Promise<QuestionGeneratorEnvelope | undefined> {
  const generator = input.target.questionGenerator;
  if (!generator) return undefined;
  if (generator.type !== "command") {
    throw new Error(`unsupported question generator type: ${generator.type}`);
  }
  if (!generator.command?.trim()) {
    throw new Error("question generator command is required");
  }

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-question-generator-"));
  const inputFile = path.join(tmpDir, "input.json");
  const payload = buildQuestionGeneratorPayload(input);
  fs.writeFileSync(inputFile, JSON.stringify(payload, null, 2) + "\n", "utf8");
  try {
    const invocation = buildCommandInvocation(generator.command, {
      inputFile,
      seed: input.seed,
      suite: input.suite,
      target: input.targetName
    });
    const stdout = await runProcessWithInput(invocation, {
      input: JSON.stringify(payload) + "\n",
      timeoutMs: generator.timeoutMs ?? 120_000
    });
    return parseQuestionGeneratorOutput(stdout);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

function buildQuestionGeneratorPayload(input: ExecuteQuestionGeneratorInput): Record<string, unknown> {
  return {
    version: 1,
    target: input.targetName,
    suite: input.suite,
    seed: input.seed,
    cases: input.cases.map((item) => ({
      caseId: item.id,
      template: item.template,
      originalQuestion: item.question,
      questionAnchors: item.questionAnchors,
      scoring: item.scoring
    })),
    constraints: {
      outputShape: {
        questions: [{
          caseId: "must match one input caseId",
          template: "must match that case template",
          question: "natural-language read-only user question"
        }]
      },
      readOnly: true,
      maxQuestionChars: MAX_GENERATED_QUESTION_CHARS,
      forbiddenActions: ["upload", "sync", "delete", "modify", "create", "restart", "send"]
    }
  };
}

function applyGeneratedQuestions(cases: PatrolCase[], envelope: QuestionGeneratorEnvelope | undefined): PatrolCase[] {
  if (!envelope) {
    return cases.map((item) => fallbackCase(item, "question_generator_missing_result"));
  }
  if (envelope.error) {
    return cases.map((item) => fallbackCase(item, `question_generator_error:${envelope.error}`));
  }

  const byCaseId = new Map<string, GeneratedQuestion>();
  for (const item of envelope.questions ?? []) {
    if (!byCaseId.has(item.caseId)) byCaseId.set(item.caseId, item);
  }

  return cases.map((item) => {
    const generated = byCaseId.get(item.id);
    if (!generated) return fallbackCase(item, "missing_generated_question");
    const validation = validateGeneratedQuestion(item, generated);
    if (!validation.ok) return fallbackCase(item, validation.warning);
    return {
      ...item,
      originalQuestion: item.question,
      question: validation.question,
      questionSource: envelope.source ?? "llm_question_generator"
    };
  });
}

function fallbackCase(item: PatrolCase, warning: string): PatrolCase {
  return {
    ...item,
    originalQuestion: item.originalQuestion ?? item.question,
    questionSource: "template",
    questionGeneratorWarning: warning
  };
}

function validateGeneratedQuestion(
  patrolCase: PatrolCase,
  generated: GeneratedQuestion
): { ok: true; question: string } | { ok: false; warning: string } {
  if (generated.template && generated.template !== patrolCase.template) {
    return { ok: false, warning: "template_mismatch" };
  }
  const question = typeof generated.question === "string" ? generated.question.trim() : "";
  if (!question) return { ok: false, warning: "empty_generated_question" };
  if (question.length > MAX_GENERATED_QUESTION_CHARS) return { ok: false, warning: "generated_question_too_long" };
  if (UNSAFE_QUESTION_PATTERN.test(question)) return { ok: false, warning: "unsafe_generated_question" };
  for (const group of patrolCase.questionAnchors ?? []) {
    if (group.length > 0 && !group.some((anchor) => normalize(question).includes(normalize(anchor)))) {
      return { ok: false, warning: `missing_question_anchor:${group.join("|")}` };
    }
  }
  return { ok: true, question };
}

function normalize(value: string): string {
  return value.replace(/[\s,，_`|]/g, "").toLowerCase();
}

function parseQuestionGeneratorOutput(stdout: string): QuestionGeneratorEnvelope {
  const parsed = parseGeneratorJson(stdout);
  if (Array.isArray(parsed)) {
    return { source: "llm_question_generator", questions: parsed.map(normalizeGeneratedQuestion).filter(isGeneratedQuestion) };
  }
  if (parsed && typeof parsed === "object") {
    const record = parsed as Record<string, unknown>;
    const questions = Array.isArray(record.questions)
      ? record.questions.map(normalizeGeneratedQuestion).filter(isGeneratedQuestion)
      : [];
    return {
      source: typeof record.source === "string" ? record.source : "llm_question_generator",
      questions,
      raw: parsed
    };
  }
  return { source: "llm_question_generator", questions: [] };
}

function parseGeneratorJson(stdout: string): unknown {
  const trimmed = stdout.trim();
  if (!trimmed) throw new Error("question generator returned empty stdout");
  try {
    return JSON.parse(trimmed);
  } catch {
    const jsonl = trimmed
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter(Boolean)
      .map((line) => JSON.parse(line));
    if (jsonl.length > 0) return jsonl;
    throw new Error("question generator stdout was not JSON or JSONL");
  }
}

function normalizeGeneratedQuestion(item: unknown): GeneratedQuestion | undefined {
  if (!item || typeof item !== "object") return undefined;
  const record = item as Record<string, unknown>;
  return {
    caseId: typeof record.caseId === "string" ? record.caseId : "",
    template: typeof record.template === "string" ? record.template : undefined,
    question: typeof record.question === "string" ? record.question : ""
  };
}

function isGeneratedQuestion(item: GeneratedQuestion | undefined): item is GeneratedQuestion {
  return Boolean(item?.caseId && item.question);
}

async function safeExecuteQuestionGenerator(
  input: ExecuteQuestionGeneratorInput & { executeQuestionGenerator: ExecuteQuestionGenerator }
): Promise<QuestionGeneratorEnvelope | undefined> {
  try {
    return await input.executeQuestionGenerator(input);
  } catch (err) {
    return {
      source: "llm_question_generator",
      error: err instanceof Error ? err.message : String(err)
    };
  }
}

function groupCasesByTarget(cases: PatrolCase[]): Map<string, PatrolCase[]> {
  const grouped = new Map<string, PatrolCase[]>();
  for (const item of cases) {
    const group = grouped.get(item.target) ?? [];
    group.push(item);
    grouped.set(item.target, group);
  }
  return grouped;
}

function runProcessWithInput(invocation: CommandInvocation, options: { input: string; timeoutMs: number }): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn(invocation.command, invocation.args, {
      stdio: ["pipe", "pipe", "pipe"]
    });
    let stdout = "";
    let stderr = "";
    const timer = setTimeout(() => {
      child.kill("SIGTERM");
      reject(new Error(`question generator timed out after ${options.timeoutMs}ms`));
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
        reject(new Error(`question generator exited ${code}: ${stderr || stdout}`));
      }
    });
    child.stdin.end(options.input);
  });
}
