# FinanceQA 巡检改造实施方案

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修正 FinanceQA 巡检自身的误判和运行层噪声，让巡检结果能客观区分业务问答错误、回答格式问题、scorer 误判和 runner/provider 健康问题。

**Architecture:** 仅修改 `agent-patrol` 巡检层，不修改 FinanceQA 业务计算逻辑，不改 OpenClaw 全局 bridge。巡检层负责更准确地识别 Markdown 表格金额、对运行层瞬态失败做有限重试，并在报告中单列 runner 健康失败。

**Tech Stack:** TypeScript `agent-patrol`、Node.js test runner、OpenClaw local runner、FinanceQA snapshot golden reference。

---

## 1. 背景与证据

2026-06-29 在 `clawdbot` 上的全量 41 条复查：

| 项目 | 值 |
| --- | --- |
| run id | `financeqa-2210-full41-after-flexbridge-20260629T095802` |
| 原始结果 | `34/41 = 82.93%` |
| 后续 targeted rerun 替换口径 | `37/41 = 90.24%` |
| 主要残留问题 | 1 条 runner timeout；3 条 scorer 对千分位/表格金额误判 |

剩余 4 条客观归因：

| case | 客观判断 | 巡检侧是否应修 |
| --- | --- | --- |
| `finance_qa_finance_project_payable_unpaid_006` | 原始 run 是 `agent command timed out after 360000ms`；重跑后金额对但缺标准口径词 | 是，runner health 要重试并单列；口径词属于 FinanceQA/OpenClaw 输出规范 |
| `finance_qa_finance_latest_month_revenue_006` | 回答含 `项目结算 912,713.97`、期间、来源；scorer 报 `missing_amount` | 是，scorer 误判 |
| `finance_qa_finance_project_receivable_unpaid_008` | 回答含 `项目应收（应收未收）2,065,398.17`、期间、来源；但出现内部过程话术 | 是，scorer 误判；过程话术巡检应检测 |
| `finance_qa_finance_project_receivable_unpaid_010` | 回答含 `项目应收（应收未收）2,065,398.17`、期间、来源；scorer 报 `missing_amount` | 是，scorer 误判 |

## 2. 修改边界

### 巡检侧要修

1. `agent-patrol` scorer 识别 Markdown 表格金额。
2. `agent-patrol` runner 对 timeout/provider busy/empty stdout 做一次有限重试。
3. `agent-patrol` summary 单列 runner/provider 健康问题，避免污染业务准确率判断。
4. `agent-patrol` 增加老板可见回答的过程话术检测。

### 不属于巡检侧

1. FinanceQA 应付类答案要稳定输出 `项目应付（应付未付/未付款）` 标准口径词。
2. OpenClaw finance 可见回答要禁止输出“读技能文件”“调用 finance-query”等内部过程。
3. 不修改 FinanceQA 最新月份营收、项目应收未收的计算逻辑；复测显示金额、期间、来源是对的。

## 3. 文件结构

目标代码位于 `clawdbot` 的 agent-patrol 目录：

| 文件 | 责任 |
| --- | --- |
| `/opt/finance_qa/agent-patrol/src/scorer.ts` | 金额、期间、来源、口径词打分逻辑 |
| `/opt/finance_qa/agent-patrol/src/run.ts` | 串行执行 case、调用 runner、写入 evidence 前的数据流 |
| `/opt/finance_qa/agent-patrol/src/report.ts` | 失败类型归类和 summary 输出 |
| `/opt/finance_qa/agent-patrol/test/scorer.test.ts` | scorer 单元测试 |
| `/opt/finance_qa/agent-patrol/test/run.test.ts` | runner/retry 单元测试 |
| `/opt/finance_qa/agent-patrol/test/report.test.ts` | summary 分类单元测试 |
| `/opt/finance_qa/agent-patrol/tmp/financeqa-baseline-today-all-questions.yaml` | 当前 41 条复查配置；临时验证用，不作为长期源码 |

长期配置如果有源码 preset，应修改对应 preset，而不是只改 `tmp/` 文件。

## 4. 实施任务

### Task 1: 修 Markdown 表格金额识别

**Files:**
- Modify: `/opt/finance_qa/agent-patrol/src/scorer.ts`
- Test: `/opt/finance_qa/agent-patrol/test/scorer.test.ts`

- [ ] **Step 1: 写失败测试，覆盖表格同一行金额**

新增测试：

```ts
test("scoreCase accepts labeled markdown table row amount with thousands separators", () => {
  const score = scoreCase({
    id: "table-amount",
    expected: {
      referenceChecks: {
        amounts: { labels: ["项目结算"] },
        periods: true,
        sources: true
      }
    },
    actual: {
      source: "agent",
      answer: [
        "老板，从收入表来看，2026年6月情况如下：",
        "| 指标 | 金额（元） |",
        "| --- | ---: |",
        "| 项目结算 | **912,713.97** |",
        "来源：《优集收入、成本计算表 - 上传.xlsx》的【26年Q2收入明细】"
      ].join("\n")
    },
    reference: {
      source: "golden_reference",
      answer: "2026-06 项目结算 912713.97 元。来源：《优集收入、成本计算表 - 上传.xlsx》"
    }
  });

  assert.equal(score.pass, true);
  assert.deepEqual(score.failures, []);
});
```

- [ ] **Step 2: 写失败测试，覆盖带括号别名的表格金额**

新增测试：

```ts
test("scoreCase accepts table row amount when label has parenthetical alias", () => {
  const score = scoreCase({
    id: "table-alias-amount",
    expected: {
      referenceChecks: {
        amounts: { labels: ["项目应收"] },
        periods: true,
        sources: true
      },
      amountLabelGroups: [["项目应收", "应收未收", "未回款"]]
    } as any,
    actual: {
      source: "agent",
      answer: [
        "2025-10~2026-05 所有项目应收情况如下：",
        "| 指标 | 金额 |",
        "| --- | ---: |",
        "| **项目应收（应收未收）** | **2,065,398.17 元** |",
        "来源：《优集收入、成本计算表 - 上传.xlsx》"
      ].join("\n")
    },
    reference: {
      source: "golden_reference",
      answer: "2025-10~2026-05 项目应收 2065398.17 元。来源：《优集收入、成本计算表 - 上传.xlsx》"
    }
  });

  assert.equal(score.pass, true);
  assert.deepEqual(score.failures, []);
});
```

- [ ] **Step 3: 运行测试，确认当前失败**

Run:

```bash
cd /opt/finance_qa/agent-patrol
node --import tsx --test test/scorer.test.ts
```

Expected: 新增两条测试失败，现有 `scoreCase compares labeled headline amounts instead of any repeated detail amount` 仍保持原预期。

- [ ] **Step 4: 最小实现**

修改 `labeledWindows(answer, label)`：

1. 找到 label 所在原始行。
2. 如果该行是 Markdown 表格行，即包含 `|` 且 label 在该行内，则把整行作为 window。
3. 非表格行仍保留现有边界逻辑。
4. 不增加“全局看到金额就算通过”的逻辑。

实现方向：

```ts
function markdownTableRowWindow(answer: string, offset: number): string | undefined {
  const lineStart = answer.lastIndexOf("\n", offset) + 1;
  const nextNewline = answer.indexOf("\n", offset);
  const lineEnd = nextNewline >= 0 ? nextNewline : answer.length;
  const line = answer.slice(lineStart, lineEnd);
  if (!line.includes("|")) return undefined;
  return line;
}
```

在 `labeledWindows` 中优先 push 该 table row window。

- [ ] **Step 5: 验证 scorer 测试**

Run:

```bash
cd /opt/finance_qa/agent-patrol
node --import tsx --test test/scorer.test.ts
```

Expected: 全部通过。

### Task 2: 对 runner/provider 健康失败做一次重试

**Files:**
- Modify: `/opt/finance_qa/agent-patrol/src/run.ts`
- Modify if needed: `/opt/finance_qa/agent-patrol/src/scorer.ts`
- Test: `/opt/finance_qa/agent-patrol/test/run.test.ts`

- [ ] **Step 1: 写失败测试，provider busy 首次失败后重试成功**

测试要点：

1. `executeAgent` 第一次返回 agent answer：`Xunfei request failed ... The system is busy, please try again later.`
2. 第二次返回正常答案。
3. `runSuite` 最终 pass。
4. `executeAgent` 调用次数为 2。

示例结构：

```ts
test("runSuite retries retryable provider busy answers once", async () => {
  let calls = 0;
  const result = await runSuite(config, {
    suite: "smoke",
    seed: "retry-provider-busy",
    outDir: fs.mkdtempSync(path.join(os.tmpdir(), "patrol-retry-")),
    executeAgent: async () => {
      calls += 1;
      if (calls === 1) {
        return {
          source: "agent",
          answer: "Xunfei request failed with code: 10012, msg: EngineInternalError:1105|The system is busy, please try again later."
        };
      }
      return {
        source: "agent",
        answer: "2026-06 项目结算 912,713.97 元。来源：《优集收入、成本计算表 - 上传.xlsx》"
      };
    },
    executeGoldenReference: async () => ({
      source: "golden_reference",
      answer: "2026-06 项目结算 912713.97 元。来源：《优集收入、成本计算表 - 上传.xlsx》"
    })
  });

  assert.equal(calls, 2);
  assert.equal(result.aggregate.passed, 1);
});
```

- [ ] **Step 2: 写失败测试，真实业务错不重试**

测试要点：如果第一次返回的是“账上营收 0 元”这种业务错，不触发重试。

- [ ] **Step 3: 实现 retry 判定**

新增 helper：

```ts
function isRetryableAgentHealthFailure(actual: AgentEnvelope): boolean {
  const text = `${actual.error ?? ""}\n${actual.answer ?? ""}`;
  return /timed out after \d+ms/i.test(text)
    || /empty command stdout/i.test(text)
    || /EngineInternalError:1105/i.test(text)
    || /The system is busy, please try again later/i.test(text)
    || /Xunfei request failed/i.test(text);
}
```

在 `runSuite` 每个 case 执行 agent 时：

1. 先跑一次。
2. 如果是 retryable health failure，最多再跑一次。
3. evidence 中记录 `attempts` 或 `retryCount`，方便审计。
4. 不对 `agent_changed_amount`、`missing_source`、`period_mismatch` 做重试。

- [ ] **Step 4: 验证 run 测试**

Run:

```bash
cd /opt/finance_qa/agent-patrol
node --import tsx --test test/run.test.ts
```

Expected: 新增测试和原有测试通过。

### Task 3: runner/provider 失败分类不污染核心准确性

**Files:**
- Modify: `/opt/finance_qa/agent-patrol/src/scorer.ts`
- Modify: `/opt/finance_qa/agent-patrol/src/report.ts`
- Test: `/opt/finance_qa/agent-patrol/test/scorer.test.ts`
- Test: `/opt/finance_qa/agent-patrol/test/report.test.ts`

- [ ] **Step 1: 写测试，provider busy 归类为 runner health**

当最终 answer 仍是 `Xunfei request failed ... system is busy` 时，score 应产生 `agent_runner_error`，failure category 是 `runner_health`，不要同时报一堆 `agent_changed_amount/missing_source/period_mismatch`。

- [ ] **Step 2: 实现健康失败短路**

在 `scoreCase` 早期加入：

```ts
if (isAgentHealthFailureAnswer(answer) && !input.actual.error) {
  addFailure(failures, failureDetails, "agent_runner_error", "agent_runner_error", {
    message: "agent runner/provider failed before producing a valid answer",
    actual: answer
  });
  return buildScore(...);
}
```

注意：只对明确的 runner/provider 文本短路，不对普通错误金额短路。

- [ ] **Step 3: summary 输出保留严格准确率和健康失败数**

`summary.json` 当前已有 `failureCategoryCounts.runner_health`。保持该字段，并确保 provider busy 被计入 `runner_health`。

- [ ] **Step 4: 验证 report 测试**

Run:

```bash
cd /opt/finance_qa/agent-patrol
node --import tsx --test test/scorer.test.ts test/report.test.ts
```

Expected: runner health 分类正确，核心金额错误仍归 `core_accuracy`。

### Task 4: 增加过程话术检测

**Files:**
- Modify: 长期 FinanceQA patrol config 或 preset
- Test: `/opt/finance_qa/agent-patrol/test/scorer.test.ts`

- [ ] **Step 1: 定义禁止出现在老板可见回答中的过程词**

建议先用最小集合：

```yaml
mustNotContain:
  - 读技能文件
  - 技能文件已确认
  - 调用 finance-query
  - 调 finance-query
  - MCP
```

不要加入过宽泛的 `工具`、`查询`、`执行`，否则容易误伤正常业务表达。

- [ ] **Step 2: 在巡检配置生成源头统一注入**

不要只改 `tmp/financeqa-baseline-today-all-questions.yaml`。应找到生成该配置的源码或 preset，把上述 `mustNotContain` 合并进 FinanceQA 财务问答模板的 scoring。

- [ ] **Step 3: 写 scorer 测试**

```ts
test("scoreCase flags internal process wording in boss-visible answer", () => {
  const score = scoreCase({
    id: "process-wording",
    expected: {
      mustNotContain: ["读技能文件", "调用 finance-query"]
    },
    actual: {
      source: "agent",
      answer: "我先读技能文件，然后调用 finance-query。老板，项目应收 2,065,398.17 元。"
    }
  });

  assert.equal(score.pass, false);
  assert.deepEqual(score.failureDetails.map((item) => item.type), ["forbidden_term", "forbidden_term"]);
});
```

- [ ] **Step 4: 验证配置不会误伤正常来源**

用最近通过的两条回答验证：

1. `最新月份营收` 回答含来源、金额、期间，不能因为来源说明被误伤。
2. `项目应收未收` 回答含表格、客户明细，不能因为业务词被误伤。

### Task 5: 端到端复查

**Files:**
- No code changes beyond Tasks 1-4
- Runtime: `clawdbot:/opt/finance_qa/agent-patrol`

- [ ] **Step 1: 运行 agent-patrol 单元测试**

Run:

```bash
cd /opt/finance_qa/agent-patrol
npm test
```

Expected: all tests pass.

- [ ] **Step 2: 重跑 4 条历史失败 targeted suite**

使用同样问题原文创建临时 suite，覆盖：

1. `按项目成本口径，从2025年10月起至今未付款合计多少？`
2. `老板，从收入表看最新一个月的营收是多少？`
3. `老板，帮我查一下从2025年10月到上个完整自然月底，所有项目的应收未收是多少？`
4. `老板，从项目应收口径看，2025年10月到现在应收未收的总金额是多少？`

Expected:

1. 表格金额不再误判 `missing_amount`。
2. provider busy/timeout 如果仍发生，计入 `runner_health` 并已尝试一次 retry。
3. 过程话术被 `forbidden_term` 捕获。

- [ ] **Step 3: 重跑 41 条全量巡检**

Run id 建议：

```text
financeqa-full41-after-patrol-fix-YYYYMMDDTHHMMSS
```

Expected:

1. `summary.json.aggregate.total = 41`
2. 严格准确率目标 `>= 90%`
3. `failureCategoryCounts.runner_health` 单列，不与 `core_accuracy` 混淆
4. `latest_month_revenue_006`、`receivable_unpaid_008`、`receivable_unpaid_010` 不再因表格千分位金额误判失败

## 5. 验收标准

本巡检改造完成的标准：

1. `agent-patrol` 单元测试全通过。
2. Markdown 表格金额识别通过新增测试。
3. 已有“明细金额不能冒充汇总金额”的测试仍通过。
4. provider busy/timeout 至少 retry 一次，最终失败时归 `runner_health`。
5. 41 条全量重跑严格准确率达到 `>= 90%`，或如果未达到，失败项必须能清楚分成业务错误、格式错误、runner 健康错误。
6. 未修改 FinanceQA 业务计算逻辑，未影响 `lzh`，未改 OpenClaw 全局 bridge。

## 6. 风险与防护

| 风险 | 防护 |
| --- | --- |
| 金额匹配放太宽，明细金额冒充汇总金额 | 只扩展 Markdown 表格整行 window；保留现有 labeled headline 测试 |
| runner 重试增加 `clawdbot` 压力 | 只 retry 一次，只对明确 health failure retry |
| 把真实业务错归为 runner health | health pattern 只匹配 timeout、empty stdout、Xunfei busy 等明确运行层文本 |
| 禁止词误伤正常回答 | 禁止词保持窄集合，不禁用“查询”“来源”“工具”这类泛词 |

## 7. 后续 FinanceQA 侧另行处理

下面事项不放进本巡检改造 PR，但应单独修：

1. 应付类 final answer 稳定输出 `项目应付（应付未付/未付款）`。
2. OpenClaw finance 老板可见回答禁止输出内部过程。
3. FinanceQA/OpenClaw 输出规范修完后，再用巡检的 `mustNotContain` 做回归保护。
