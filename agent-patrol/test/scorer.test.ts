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
