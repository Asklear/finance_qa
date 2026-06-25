import test from "node:test";
import assert from "node:assert/strict";
import { scoreCase } from "../src/scorer.ts";

test("scoreCase marks missing actual path invalid", () => {
  const score = scoreCase({
    id: "case-1",
    expected: { mustContain: ["项目口径"] },
    actual: { source: "direct_mcp", answer: "项目口径 100 元" }
  });

  assert.equal(score.pass, false);
  assert.equal(score.invalid, true);
  assert.match(score.failures.join(" "), /invalid_actual_path/);
});

test("scoreCase applies required terms, forbidden terms, amount checks, and write tool guard", () => {
  const score = scoreCase({
    id: "case-2",
    expected: {
      mustContain: ["项目口径", "2026-05"],
      mustNotContain: ["合同应付"],
      amounts: [{ label: "应收未收", value: 12345.67 }]
    },
    actual: {
      source: "agent",
      answer: "项目口径看，2026-05 应收未收为 12,345.67 元。",
      toolCalls: [{ name: "finance-query" }]
    }
  });

  assert.equal(score.pass, true);
  assert.deepEqual(score.failures, []);
});

test("scoreCase accepts one term from each required synonym group", () => {
  const score = scoreCase({
    id: "case-3",
    expected: {
      mustContain: ["应收未收"],
      mustContainAny: [
        ["项目口径", "所有项目", "项目应收"],
        ["2025年10月", "2025-10"]
      ]
    },
    actual: {
      source: "agent",
      answer: "2025年10月 – 2026年5月，所有项目应收未收 218.52 万元。"
    }
  });

  assert.equal(score.pass, true);
  assert.deepEqual(score.failures, []);
});

test("scoreCase reports missing synonym groups", () => {
  const score = scoreCase({
    id: "case-4",
    expected: {
      mustContainAny: [
        ["项目口径", "所有项目", "项目应收"]
      ]
    },
    actual: {
      source: "agent",
      answer: "按客户统计，应收未收 218.52 万元。"
    }
  });

  assert.equal(score.pass, false);
  assert.deepEqual(score.failures, ["missing_any_term:项目口径|所有项目|项目应收"]);
});

test("scoreCase classifies term-only misses as scorer_term_miss", () => {
  const score = scoreCase({
    id: "case-5",
    expected: {
      mustContainAny: [["项目口径", "所有项目", "项目应收"]]
    },
    actual: {
      source: "agent",
      answer: "按客户统计，应收未收 218.52 万元。"
    }
  });

  assert.equal(score.pass, false);
  assert.deepEqual(score.failureDetails.map((failure) => failure.type), ["scorer_term_miss"]);
});

test("scoreCase detects agent_changed_amount when reference contains expected amount but actual does not", () => {
  const score = scoreCase({
    id: "case-6",
    expected: {
      amounts: [{ label: "应收未收", value: 2185200 }]
    },
    actual: {
      source: "agent",
      answer: "项目口径看，应收未收为 2,000,000.00 元。"
    },
    reference: {
      source: "financeqa_mcp",
      answer: "项目应收口径，应收未收为 2,185,200.00 元。"
    }
  });

  assert.equal(score.pass, false);
  assert.deepEqual(score.failures, ["missing_amount:应收未收=2185200"]);
  assert.equal(score.failureDetails[0]?.type, "agent_changed_amount");
});

test("scoreCase marks missing FinanceQA MCP reference as insufficient evidence", () => {
  const score = scoreCase({
    id: "case-6b",
    expected: {
      amounts: [{ label: "应收未收", value: 2185200 }]
    },
    actual: {
      source: "agent",
      answer: "项目口径看，应收未收为 2,185,200.00 元。"
    },
    reference: {
      source: "financeqa_mcp",
      tool: "finance-query",
      error: "financeqa oracle returned HTTP 503"
    }
  });

  assert.equal(score.pass, false);
  assert.equal(score.failureDetails[0]?.type, "missing_reference");
  assert.match(String(score.failureDetails[0]?.reference), /HTTP 503/);
});

test("scoreCase prioritizes finance source, period, and perspective evidence", () => {
  const score = scoreCase({
    id: "case-7",
    expected: {
      sources: ["项目应收"],
      periods: ["2025年10月", "2026年5月"],
      perspectives: ["项目口径"]
    },
    actual: {
      source: "agent",
      answer: "应收未收为 2,185,200.00 元。"
    }
  });

  assert.equal(score.pass, false);
  assert.deepEqual(score.failureDetails.map((failure) => failure.type), [
    "missing_source",
    "period_mismatch",
    "period_mismatch",
    "perspective_mismatch"
  ]);
});

test("scoreCase orders finance evidence failures before auxiliary term failures", () => {
  const score = scoreCase({
    id: "case-8",
    expected: {
      amounts: [{ label: "应收未收", value: 2185200 }],
      sources: ["项目应收"],
      periods: ["2025年10月"],
      perspectives: ["项目口径"],
      mustContainAny: [["项目口径", "所有项目", "项目应收"]]
    },
    actual: {
      source: "agent",
      answer: "按客户统计，应收未收为 2,000,000.00 元。"
    },
    reference: {
      source: "financeqa_mcp",
      answer: "项目应收口径，2025年10月至2026年5月，应收未收为 2,185,200.00 元。"
    }
  });

  assert.deepEqual(score.failureDetails.map((failure) => failure.type), [
    "agent_changed_amount",
    "missing_source",
    "period_mismatch",
    "perspective_mismatch",
    "scorer_term_miss"
  ]);
});
