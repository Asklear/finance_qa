import test from "node:test";
import assert from "node:assert/strict";
import { generateCases } from "../src/cases.ts";

test("generateCases creates deterministic agent-level cases", () => {
  const config = {
    templates: {
      latest_revenue: {
        questions: ["收入表中最新月份的营收是多少？", "按最新可见月份，看一下当月营收。"]
      },
      customer_detail: {
        question: "{{customer.name}} 最近进展怎么样？",
        fallbackQuestion: "一个活跃客户 最近进展怎么样？"
      }
    },
    targets: {
      finance: {
        kind: "openclaw_finance_agent",
        runner: { type: "openclaw_agent_cli" },
        oracle: { type: "financeqa_readonly" },
        suites: { smoke: { templates: ["latest_revenue"], caseCount: 1 } }
      },
      bossa: {
        kind: "bossa_claude_agent",
        runner: { type: "claude_agent_sdk" },
        oracle: { type: "bossa_readonly_mcp" },
        suites: { smoke: { templates: ["customer_detail"], caseCount: 1 } }
      }
    }
  };
  const anchors = {
    bossa: { customers: [{ name: "测试客户A" }] }
  };

  const first = generateCases(config, { suite: "smoke", seed: "2026-06-25", anchors });
  const second = generateCases(config, { suite: "smoke", seed: "2026-06-25", anchors });

  assert.deepEqual(first, second);
  assert.equal(first.length, 2);
  assert.equal(first[0].actualRunner, "openclaw_agent_cli");
  assert.equal(first[0].oracle, "financeqa_readonly");
  assert.equal(first[1].actualRunner, "claude_agent_sdk");
  assert.equal(first[1].oracle, "bossa_readonly_mcp");
  assert.equal(first[1].question, "测试客户A 最近进展怎么样？");
  assert.notEqual(first[0].actualRunner, "direct_mcp");
});

test("generateCases rejects unknown templates instead of using built-in business defaults", () => {
  const config = {
    templates: {},
    targets: {
      finance: {
        runner: { type: "openclaw_agent_cli" },
        oracle: { type: "financeqa_readonly" },
        suites: { smoke: { templates: ["finance_latest_month_revenue"], caseCount: 1 } }
      }
    }
  };

  assert.throws(() => generateCases(config, { suite: "smoke", seed: "fixed" }), /unknown case template/i);
});
