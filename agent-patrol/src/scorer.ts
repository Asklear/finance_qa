import type { AgentEnvelope } from "./types.ts";
import { isWriteTool } from "./guard.ts";

interface ExpectedRules {
  mustContain?: string[];
  mustContainAny?: string[][];
  mustNotContain?: string[];
  amounts?: Array<{ label: string; value: number }>;
}

interface ScoreInput {
  id: string;
  expected: ExpectedRules;
  actual: Partial<AgentEnvelope>;
}

export interface CaseScore {
  caseId: string;
  pass: boolean;
  invalid: boolean;
  failures: string[];
  warnings: string[];
}

export function scoreCase(input: ScoreInput): CaseScore {
  const failures: string[] = [];
  const warnings: string[] = [];
  const answer = input.actual.answer ?? "";

  if (input.actual.source !== "agent") {
    failures.push("invalid_actual_path");
  }
  for (const term of input.expected.mustContain ?? []) {
    if (!normalize(answer).includes(normalize(term))) {
      failures.push(`missing_term:${term}`);
    }
  }
  for (const group of input.expected.mustContainAny ?? []) {
    if (group.length > 0 && !group.some((term) => normalize(answer).includes(normalize(term)))) {
      failures.push(`missing_any_term:${group.join("|")}`);
    }
  }
  for (const term of input.expected.mustNotContain ?? []) {
    if (normalize(answer).includes(normalize(term))) {
      failures.push(`forbidden_term:${term}`);
    }
  }
  for (const amount of input.expected.amounts ?? []) {
    if (!amountPresent(answer, amount.value)) {
      failures.push(`missing_amount:${amount.label}=${amount.value}`);
    }
  }
  for (const call of input.actual.toolCalls ?? []) {
    if (call.name && isWriteTool(call.name)) {
      failures.push(`write_tool_called:${call.name}`);
    }
  }

  return {
    caseId: input.id,
    pass: failures.length === 0,
    invalid: failures.includes("invalid_actual_path"),
    failures,
    warnings
  };
}

function amountPresent(answer: string, value: number): boolean {
  const variants = [
    value.toFixed(2),
    value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 }),
    String(Math.round(value))
  ];
  const normalizedAnswer = normalize(answer);
  return variants.some((variant) => normalizedAnswer.includes(normalize(variant)));
}

function normalize(value: string): string {
  return value.replace(/[\s,，_`|]/g, "").toLowerCase();
}
