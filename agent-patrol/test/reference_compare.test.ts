import test from "node:test";
import assert from "node:assert/strict";
import { deriveReferenceRules } from "../src/reference_compare.ts";

const referenceAnswer = "2025-10~2026-04 老板口径先看项目汇总：项目应收 10943576.36 元。补充项目结算 45769448.67 元、已到账 35610224.56 元；其中已开票未回款 212890.38 元。 来源：《fin-revenue-0422.xlsx》的【25年Q4收入明细】和【26年Q1收入明细】；《fin-revcost-0601.xlsx》的【26年Q2收入明细】 来源更新时间：2026-06-25 12:35:09";

test("deriveReferenceRules extracts labeled headline amounts, periods, sources, and perspectives", () => {
  const rules = deriveReferenceRules(referenceAnswer, {
    amounts: {
      labels: ["项目应收", "应收未收"]
    },
    periods: true,
    sources: true,
    perspectives: true
  });

  assert.deepEqual(rules.amounts, [{ label: "项目应收", value: 10943576.36 }]);
  assert.deepEqual(rules.periods, ["2025-10", "2026-04"]);
  assert.deepEqual(rules.sources, ["fin-revenue-0422.xlsx", "fin-revcost-0601.xlsx"]);
  assert.deepEqual(rules.perspectives, ["老板口径", "项目汇总"]);
});

test("deriveReferenceRules converts ten-thousand yuan amounts", () => {
  const rules = deriveReferenceRules("项目应收 218.52 万元，来源：《fin-revcost-0601.xlsx》", {
    amounts: {
      labels: ["项目应收"]
    }
  });

  assert.deepEqual(rules.amounts, [{ label: "项目应收", value: 2185200 }]);
});
