import fs from "node:fs";
import path from "node:path";

interface ReportInput {
  manifest: unknown;
  cases: unknown[];
  results: unknown[];
  scores: unknown[];
  aggregate: { total: number; passed: number; accuracy: number; durationMs?: number };
}

export function redactSensitive(text: string): string {
  return text
    .replace(/Bearer\s+[A-Za-z0-9._~+/-]{16,}/gi, "Bearer [REDACTED]")
    .replace(/\b(token|api[_-]?key|authorization)=\S+/gi, "$1=[REDACTED]");
}

export function writeReport(dir: string, input: ReportInput): void {
  fs.mkdirSync(dir, { recursive: true });
  writeJson(path.join(dir, "manifest.json"), input.manifest);
  writeJson(path.join(dir, "cases.json"), input.cases);
  writeJson(path.join(dir, "scores.json"), input.scores);
  writeJson(path.join(dir, "summary.json"), renderSummaryJson(input));
  fs.writeFileSync(
    path.join(dir, "raw_results.jsonl"),
    input.results.map((row) => redactSensitive(JSON.stringify(row))).join("\n") + "\n",
    "utf8"
  );
  fs.writeFileSync(path.join(dir, "summary.md"), renderSummary(input), "utf8");
}

function writeJson(filePath: string, value: unknown): void {
  fs.writeFileSync(filePath, redactSensitive(JSON.stringify(value, null, 2)) + "\n", "utf8");
}

function renderSummary(input: ReportInput): string {
  const accuracy = (input.aggregate.accuracy * 100).toFixed(2);
  const failed = input.aggregate.total - input.aggregate.passed;
  const lines = [
    "# Agent Patrol Summary",
    "",
    `Accuracy: ${accuracy}%`,
    `Passed: ${input.aggregate.passed}/${input.aggregate.total}`,
    `Failed: ${failed}`,
    ""
  ];
  const failedCases = failedCaseRows(input);
  if (failedCases.length > 0) {
    lines.push("## Failed Cases", "");
    for (const row of failedCases) {
      lines.push(`- ${row.caseId}`);
      if (row.failures.length > 0) lines.push(`  - Failures: ${row.failures.join(", ")}`);
      if (row.question) lines.push(`  - Question: ${row.question}`);
      if (row.answer) lines.push(`  - Answer: ${truncate(row.answer, 240)}`);
      if (row.sessionId) lines.push(`  - Session: ${row.sessionId}`);
    }
    lines.push("");
  }
  return redactSensitive(lines.join("\n"));
}

function renderSummaryJson(input: ReportInput): unknown {
  return {
    manifest: input.manifest,
    aggregate: input.aggregate,
    failedCases: failedCaseRows(input)
  };
}

function failedCaseRows(input: ReportInput): Array<{
  caseId: string;
  failures: string[];
  question?: string;
  answer?: string;
  sessionId?: string;
}> {
  const resultsByCase = new Map<string, Record<string, unknown>>();
  for (const result of input.results) {
    const row = asRecord(result);
    const caseId = stringValue(row?.caseId);
    if (caseId && row) resultsByCase.set(caseId, row);
  }
  const rows: Array<{
    caseId: string;
    failures: string[];
    question?: string;
    answer?: string;
    sessionId?: string;
  }> = [];
  for (const item of input.scores) {
    const score = asRecord(item);
    if (!score || score.pass !== false) continue;
    const caseId = stringValue(score.caseId) ?? "unknown";
    const result = resultsByCase.get(caseId);
    const actual = asRecord(result?.actual);
    rows.push({
      caseId,
      failures: stringArray(score.failures),
      question: stringValue(result?.question),
      answer: stringValue(actual?.answer) ?? stringValue(result?.answer),
      sessionId: stringValue(actual?.sessionId) ?? stringValue(actual?.sessionKey)
    });
  }
  return rows;
}

function truncate(value: string, maxLength: number): string {
  return value.length <= maxLength ? value : `${value.slice(0, maxLength - 3)}...`;
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value : undefined;
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}
