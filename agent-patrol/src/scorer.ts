import type { AgentEnvelope, ReferenceEnvelope } from "./types.ts";
import { isWriteTool } from "./guard.ts";

interface ExpectedRules {
  mustContain?: string[];
  mustContainAny?: string[][];
  mustNotContain?: string[];
  amounts?: Array<{ label: string; value: number }>;
  sources?: string[];
  periods?: string[];
  perspectives?: string[];
}

interface ScoreInput {
  id: string;
  expected: ExpectedRules;
  actual: Partial<AgentEnvelope>;
  reference?: Partial<ReferenceEnvelope>;
}

export interface CaseFailure {
  type: string;
  message: string;
  expected?: unknown;
  actual?: unknown;
  reference?: unknown;
}

export interface CaseScore {
  caseId: string;
  pass: boolean;
  invalid: boolean;
  failures: string[];
  failureDetails: CaseFailure[];
  warnings: string[];
}

export function scoreCase(input: ScoreInput): CaseScore {
  const failures: string[] = [];
  const failureDetails: CaseFailure[] = [];
  const warnings: string[] = [];
  const answer = input.actual.answer ?? "";
  const referenceAnswer = input.reference?.answer ?? "";

  if (input.actual.source !== "agent") {
    addFailure(failures, failureDetails, "invalid_actual_path", "invalid_actual_path", {
      message: "actual answer did not come from the agent path",
      actual: input.actual.source
    });
  }
  if (input.reference?.source === "financeqa_mcp" && !referenceAnswer) {
    addFailure(failures, failureDetails, "missing_reference:financeqa_mcp", "missing_reference", {
      message: "FinanceQA MCP reference is missing, empty, or failed",
      reference: input.reference.error ?? input.reference
    });
  }
  for (const amount of input.expected.amounts ?? []) {
    if (!amountPresent(answer, amount.value)) {
      const referenceHasAmount = Boolean(referenceAnswer) && amountPresent(referenceAnswer, amount.value);
      addFailure(
        failures,
        failureDetails,
        `missing_amount:${amount.label}=${amount.value}`,
        referenceHasAmount ? "agent_changed_amount" : "amount_mismatch",
        {
          message: referenceHasAmount
            ? "actual answer does not contain expected amount but reference does"
            : "actual answer does not contain expected amount",
          expected: amount,
          actual: answer,
          reference: referenceAnswer || undefined
        }
      );
    }
  }
  for (const source of input.expected.sources ?? []) {
    if (!normalize(answer).includes(normalize(source))) {
      addFailure(failures, failureDetails, `missing_source:${source}`, "missing_source", {
        message: `actual answer is missing source evidence: ${source}`,
        expected: source,
        actual: answer
      });
    }
  }
  for (const period of input.expected.periods ?? []) {
    if (!normalize(answer).includes(normalize(period))) {
      addFailure(failures, failureDetails, `period_mismatch:${period}`, "period_mismatch", {
        message: `actual answer is missing expected period evidence: ${period}`,
        expected: period,
        actual: answer
      });
    }
  }
  for (const perspective of input.expected.perspectives ?? []) {
    if (!normalize(answer).includes(normalize(perspective))) {
      addFailure(failures, failureDetails, `perspective_mismatch:${perspective}`, "perspective_mismatch", {
        message: `actual answer is missing expected perspective evidence: ${perspective}`,
        expected: perspective,
        actual: answer
      });
    }
  }
  for (const term of input.expected.mustContain ?? []) {
    if (!normalize(answer).includes(normalize(term))) {
      addFailure(failures, failureDetails, `missing_term:${term}`, "scorer_term_miss", {
        message: `actual answer is missing required term: ${term}`,
        expected: term,
        actual: answer
      });
    }
  }
  for (const group of input.expected.mustContainAny ?? []) {
    if (group.length > 0 && !group.some((term) => normalize(answer).includes(normalize(term)))) {
      addFailure(failures, failureDetails, `missing_any_term:${group.join("|")}`, "scorer_term_miss", {
        message: `actual answer is missing one term from required group: ${group.join("|")}`,
        expected: group,
        actual: answer
      });
    }
  }
  for (const term of input.expected.mustNotContain ?? []) {
    if (normalize(answer).includes(normalize(term))) {
      addFailure(failures, failureDetails, `forbidden_term:${term}`, "forbidden_term", {
        message: `actual answer contains forbidden term: ${term}`,
        expected: term,
        actual: answer
      });
    }
  }
  for (const call of input.actual.toolCalls ?? []) {
    if (call.name && isWriteTool(call.name)) {
      addFailure(failures, failureDetails, `write_tool_called:${call.name}`, "write_tool_called", {
        message: `actual agent called write tool: ${call.name}`,
        actual: call.name
      });
    }
  }

  return {
    caseId: input.id,
    pass: failures.length === 0,
    invalid: failures.includes("invalid_actual_path"),
    failures,
    failureDetails,
    warnings
  };
}

function addFailure(
  failures: string[],
  failureDetails: CaseFailure[],
  code: string,
  type: string,
  detail: Omit<CaseFailure, "type">
): void {
  failures.push(code);
  failureDetails.push({ type, ...detail });
}

function amountPresent(answer: string, value: number): boolean {
  const variants = [
    value.toFixed(2),
    value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 }),
    String(Math.round(value)),
    (value / 10_000).toFixed(2),
    `${(value / 10_000).toFixed(2)}万`
  ];
  const normalizedAnswer = normalize(answer);
  return variants.some((variant) => normalizedAnswer.includes(normalize(variant)));
}

function normalize(value: string): string {
  return value.replace(/[\s,，_`|]/g, "").toLowerCase();
}
