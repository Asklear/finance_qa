import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import zlib from "node:zlib";
import { spawn, spawnSync, type SpawnOptionsWithoutStdio } from "node:child_process";
import { main } from "../src/index.ts";
import { loadConfig } from "../src/config.ts";
import { generateCases } from "../src/cases.ts";

test("generic command-agent stub example runs through the agent-patrol CLI", async () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-command-stub-"));

  const code = await main([
    "run",
    "--config",
    "examples/stub.command-agent.yaml",
    "--suite",
    "smoke",
    "--seed",
    "fixed",
    "--out",
    outDir
  ]);

  assert.equal(code, 0);
  assert.match(fs.readFileSync(path.join(outDir, "summary.md"), "utf8"), /Accuracy: 100\.00%/);
});

test("live OpenClaw SSH runner fails closed unless explicitly enabled", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-live-runner-"));
  const questionFile = path.join(dir, "question.txt");
  fs.writeFileSync(questionFile, "请只回答：AGENT_PATROL_OK", "utf8");

  const result = spawnSync("node", [
    "examples/runners/openclaw_ssh_runner.mjs",
    "--host",
    "clawdbot",
    "--question-file",
    questionFile,
    "--session-id",
    "patrol-test"
  ], {
    cwd: process.cwd(),
    encoding: "utf8",
    env: { ...process.env, AGENT_PATROL_LIVE: "" }
  });

  assert.equal(result.status, 2);
  assert.match(result.stderr, /AGENT_PATROL_LIVE=1/);
});

test("live OpenClaw local runner fails closed unless explicitly enabled", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-live-local-runner-"));
  const questionFile = path.join(dir, "question.txt");
  fs.writeFileSync(questionFile, "请只回答：AGENT_PATROL_OK", "utf8");

  const result = spawnSync("node", [
    "examples/runners/openclaw_local_runner.mjs",
    "--question-file",
    questionFile,
    "--session-id",
    "patrol-test"
  ], {
    cwd: process.cwd(),
    encoding: "utf8",
    env: { ...process.env, AGENT_PATROL_LIVE: "" }
  });

  assert.equal(result.status, 2);
  assert.match(result.stderr, /AGENT_PATROL_LIVE=1/);
});

test("financeqa preset generates varied daily sample pool", () => {
  const config = loadConfig("presets/financeqa.yaml", {
    OPENCLAW_AGENT_CMD: "node examples/runners/openclaw_ssh_runner.mjs --host clawdbot --question-file {questionFile} --session-id {sessionId}",
    FINANCEQA_MCP_URL: "http://127.0.0.1/stub"
  });

  const cases = generateCases(config, { suite: "daily", seed: "2026-06-25" });
  const questions = cases.map((item) => item.question);

  assert.equal(cases.length, 8);
  assert.equal(new Set(questions).size, 8);
  assert.equal(questions.some((item) => item.includes("最新月份") || item.includes("最新可见月份")), true);
  assert.equal(questions.some((item) => item.includes("上一个完整自然月月底")), true);
  assert.equal(questions.some((item) => item.includes("应收未收")), true);
  assert.equal(questions.some((item) => item.includes("应付未付") || item.includes("未付款")), true);
  assert.equal(questions.some((item) => item.includes("已开票未回款") || item.includes("已开票未收款")), true);
  assert.equal(questions.some((item) => item.includes("已收票未付款")), true);
});

test("financeqa preset defines reference-check labels for finance answer comparison", () => {
  const config = loadConfig("presets/financeqa.yaml", {
    OPENCLAW_AGENT_CMD: "node examples/runners/openclaw_ssh_runner.mjs --host clawdbot --question-file {questionFile} --session-id {sessionId}",
    FINANCEQA_MCP_URL: "http://127.0.0.1/stub"
  });

  for (const [name, template] of Object.entries(config.templates ?? {})) {
    const referenceChecks = template.scoring?.referenceChecks as Record<string, unknown> | undefined;
    assert.ok(referenceChecks, `${name} missing referenceChecks`);
    assert.equal(referenceChecks.periods, true, `${name} should compare reference periods`);
    assert.equal(referenceChecks.sources, true, `${name} should compare reference sources`);
    assert.notEqual(referenceChecks.perspectives, true, `${name} should not hard-fail reference boilerplate wording`);
  }
});

test("financeqa snapshot reference provider computes project receivable without FinanceQA MCP", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-financeqa-snapshot-"));
  const snapshotPath = path.join(dir, "financeqa-snapshot.json");
  const questionFile = path.join(dir, "question.txt");
  fs.writeFileSync(questionFile, "项目口径看，从2025年10月起到上一个完整自然月月底还有多少应收未收？", "utf8");
  fs.writeFileSync(snapshotPath, JSON.stringify({
    metadata: {
      generated_at: "2026-06-26T18:10:00+08:00",
      source_database: "bossagent_app",
      source_schema: "tenant_uhub_etl_shadow"
    },
    tables: {
      fin_fund_income: [
        { year_month: "2025-10", settlement_amount: 1000, received_amount: 200, invoice_amount: 500 },
        { year_month: "2026-05", settlement_amount: 300, received_amount: 100, invoice_amount: 150 },
        { year_month: "2026-06", settlement_amount: 999, received_amount: 0, invoice_amount: 0 }
      ],
      fin_file_mappings: [
        {
          table_type: "fund-income",
          period: "2025-Q4",
          file_name: "收入Q4.xlsx",
          source_version_id: "收入Q4.xlsx:hash-q4",
          updated_at: "2026-06-26T18:00:00+08:00"
        },
        {
          table_type: "fund-income",
          period: "2026-Q2",
          file_name: "收入Q2.xlsx",
          source_version_id: "收入Q2.xlsx:hash-q2",
          updated_at: "2026-06-26T18:00:00+08:00"
        }
      ]
    }
  }), "utf8");

  const result = await spawnNode([
    "examples/golden/financeqa_snapshot_reference.mjs",
    "--template", "finance_project_receivable_unpaid",
    "--question-file", questionFile,
    "--snapshot", snapshotPath,
    "--as-of-date", "2026-06-26"
  ], {
    cwd: process.cwd(),
    env: {
      ...process.env,
      FINANCEQA_MCP_URL: "http://127.0.0.1:1/must-not-be-called"
    }
  }, { timeoutMs: 5_000 });

  assert.equal(result.status, 0, result.stderr);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.result.source, "financeqa_snapshot_reference");
  assert.equal(payload.result.structured.metric, "项目应收");
  assert.equal(payload.result.structured.amount, 1000);
  assert.deepEqual(payload.result.structured.period, { from: "2025-10", to: "2026-05" });
  assert.deepEqual(payload.result.structured.source.files, ["收入Q4.xlsx", "收入Q2.xlsx"]);
  assert.equal(payload.result.structured.row_count, 2);
  assert.match(payload.result.final_answer, /2025-10~2026-05/);
  assert.match(payload.result.final_answer, /项目应收 1000\.00 元/);
  assert.equal(payload.result.structured.totals.settlement, 1300);
});

test("financeqa snapshot reference provider reads gzip snapshots and computes latest revenue month", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-financeqa-snapshot-gzip-"));
  const snapshotPath = path.join(dir, "financeqa-snapshot.json.gz");
  const questionFile = path.join(dir, "question.txt");
  fs.writeFileSync(questionFile, "收入表中最新月份的营收是多少？", "utf8");
  fs.writeFileSync(snapshotPath, zlib.gzipSync(JSON.stringify({
    metadata: {
      generated_at: "2026-06-26T18:10:00+08:00",
      source_database: "bossagent_app",
      source_schema: "tenant_uhub_etl_shadow"
    },
    tables: {
      fin_fund_income: [
        { year_month: "2026-04", settlement_amount: 900, received_amount: 800, invoice_amount: 850 },
        { year_month: "2026-05", settlement_amount: 1200, received_amount: 1000, invoice_amount: 1100 }
      ],
      fin_file_mappings: [
        {
          table_type: "fund-income",
          period: "2026-Q2",
          file_name: "优集收入、成本计算表 - 上传.xlsx",
          source_version_id: "优集收入、成本计算表 - 上传.xlsx:c34368e51eb0",
          updated_at: "2026-06-26T18:00:00+08:00"
        }
      ]
    }
  })));

  const result = await spawnNode([
    "examples/golden/financeqa_snapshot_reference.mjs",
    "--template", "finance_latest_month_revenue",
    "--question-file", questionFile,
    "--snapshot", snapshotPath,
    "--as-of-date", "2026-06-26"
  ], {
    cwd: process.cwd(),
    env: process.env
  }, { timeoutMs: 5_000 });

  assert.equal(result.status, 0, result.stderr);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.result.structured.metric, "项目结算");
  assert.equal(payload.result.structured.amount, 1200);
  assert.deepEqual(payload.result.structured.period, { from: "2026-05", to: "2026-05" });
  assert.match(payload.result.final_answer, /2026-05/);
  assert.match(payload.result.final_answer, /项目结算 1200\.00 元/);
  assert.match(payload.result.final_answer, /优集收入、成本计算表 - 上传\.xlsx/);
});

test("financeqa snapshot reference provider nets merged groups against member movements", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-financeqa-snapshot-merged-"));
  const snapshotPath = path.join(dir, "financeqa-snapshot.json");
  const questionFile = path.join(dir, "question.txt");
  fs.writeFileSync(questionFile, "项目口径看，从2025年10月起到上一个完整自然月月底还有多少应收未收？", "utf8");
  fs.writeFileSync(snapshotPath, JSON.stringify({
    metadata: {
      generated_at: "2026-06-26T18:10:00+08:00",
      source_database: "bossagent_app",
      source_schema: "tenant_uhub_etl_shadow"
    },
    tables: {
      fin_contracts: [
        { contract_id: "R1", customer_name: "客户A", contract_content: "收入项目A" },
        { contract_id: "R2", customer_name: "客户A", contract_content: "收入项目B" }
      ],
      fin_fund_income: [
        { contract_id: "R2", year_month: "2026-05", settlement_amount: 0, received_amount: 600, invoice_amount: 0 },
        { contract_id: "R2", year_month: "2026-05", settlement_amount: 0, received_amount: 100, invoice_amount: 0 }
      ],
      fin_fund_income_groups: [
        {
          id: 1,
          customer_name: "客户A",
          year_month: "2026-05",
          source_start_row: 3,
          source_end_row: 4,
          settlement_amount: 1000,
          received_amount: 400,
          invoice_amount: 1000
        }
      ],
      fin_fund_income_group_members: [
        { group_id: 1, contract_id: "R1", source_row_number: 3 },
        { group_id: 1, contract_id: "R2", source_row_number: 4 }
      ]
    }
  }), "utf8");

  const result = await spawnNode([
    "examples/golden/financeqa_snapshot_reference.mjs",
    "--template", "finance_project_receivable_unpaid",
    "--question-file", questionFile,
    "--snapshot", snapshotPath,
    "--as-of-date", "2026-06-26"
  ], {
    cwd: process.cwd(),
    env: process.env
  }, { timeoutMs: 5_000 });

  assert.equal(result.status, 0, result.stderr);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.result.structured.amount, 0);
  assert.equal(payload.result.structured.totals.open, 0);
  assert.equal(payload.result.structured.totals.settlement, 1000);
  assert.equal(payload.result.structured.totals.movement, 1100);
});

test("financeqa snapshot reference provider computes cost invoice open with offsets and item details", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-financeqa-snapshot-cost-"));
  const snapshotPath = path.join(dir, "financeqa-snapshot.json");
  const questionFile = path.join(dir, "question.txt");
  fs.writeFileSync(questionFile, "项目成本口径看，25年至26年未付款的项目及对应金额有哪些？", "utf8");
  fs.writeFileSync(snapshotPath, JSON.stringify({
    metadata: {
      generated_at: "2026-06-26T18:10:00+08:00",
      source_database: "bossagent_app",
      source_schema: "tenant_uhub_etl_shadow"
    },
    tables: {
      fin_contracts: [
        { contract_id: "S1", customer_name: "供应商A", contract_content: "成本项目A" },
        { contract_id: "S2", customer_name: "供应商A", contract_content: "成本项目B" }
      ],
      fin_cost_settlements: [
        {
          contract_id: "S1",
          year_month: "2026-05",
          settlement_amount: 1000,
          paid_amount: 300,
          invoice_amount: 900,
          invoice_open_offset_amount: 100
        }
      ],
      fin_cost_settlement_groups: [
        {
          id: 10,
          customer_name: "供应商A",
          year_month: "2026-05",
          settlement_amount: 500,
          paid_amount: 100,
          invoice_amount: 500,
          invoice_open_offset_amount: 50
        }
      ],
      fin_cost_settlement_group_members: [
        { group_id: 10, contract_id: "S2", source_row_number: 7 }
      ]
    }
  }), "utf8");

  const payable = await spawnNode([
    "examples/golden/financeqa_snapshot_reference.mjs",
    "--template", "finance_project_payable_unpaid",
    "--question-file", questionFile,
    "--snapshot", snapshotPath,
    "--as-of-date", "2026-06-26"
  ], {
    cwd: process.cwd(),
    env: process.env
  }, { timeoutMs: 5_000 });
  assert.equal(payable.status, 0, payable.stderr);
  const payablePayload = JSON.parse(payable.stdout);
  assert.equal(payablePayload.result.structured.metric, "项目应付");
  assert.equal(payablePayload.result.structured.amount, 1100);
  assert.equal(payablePayload.result.structured.totals.invoice_open, 850);

  const unpaidProjects = await spawnNode([
    "examples/golden/financeqa_snapshot_reference.mjs",
    "--template", "finance_unpaid_projects",
    "--question-file", questionFile,
    "--snapshot", snapshotPath,
    "--as-of-date", "2026-06-26"
  ], {
    cwd: process.cwd(),
    env: process.env
  }, { timeoutMs: 5_000 });
  assert.equal(unpaidProjects.status, 0, unpaidProjects.stderr);
  const unpaidPayload = JSON.parse(unpaidProjects.stdout);
  assert.equal(unpaidPayload.result.structured.metric, "已收票未付款");
  assert.equal(unpaidPayload.result.structured.amount, 850);
  assert.deepEqual(unpaidPayload.result.structured.items.map((item: { name: string; amount: number }) => [item.name, item.amount]), [
    ["供应商A/成本项目A", 500],
    ["供应商A/成本项目B", 350]
  ]);
  assert.match(unpaidPayload.result.final_answer, /明细前2项/);
  assert.match(unpaidPayload.result.final_answer, /供应商A\/成本项目A 500\.00 元/);
});

test("financeqa snapshot reference provider defaults as-of date to Asia Shanghai", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-financeqa-snapshot-local-date-"));
  const snapshotPath = path.join(dir, "financeqa-snapshot.json");
  const questionFile = path.join(dir, "question.txt");
  fs.writeFileSync(questionFile, "项目口径看，从2025年10月起到上一个完整自然月月底还有多少应收未收？", "utf8");
  fs.writeFileSync(snapshotPath, JSON.stringify({
    metadata: {
      generated_at: "2026-06-01T00:30:00+08:00",
      source_database: "bossagent_app",
      source_schema: "tenant_uhub_etl_shadow"
    },
    tables: {
      fin_fund_income: [
        { year_month: "2026-04", settlement_amount: 100, received_amount: 0, invoice_amount: 0 },
        { year_month: "2026-05", settlement_amount: 200, received_amount: 0, invoice_amount: 0 }
      ]
    }
  }), "utf8");

  const result = await spawnNode([
    "examples/golden/financeqa_snapshot_reference.mjs",
    "--template", "finance_project_receivable_unpaid",
    "--question-file", questionFile,
    "--snapshot", snapshotPath,
    "--now-epoch-ms", String(Date.parse("2026-05-31T16:30:00.000Z"))
  ], {
    cwd: process.cwd(),
    env: {
      ...process.env,
      TZ: "UTC"
    }
  }, { timeoutMs: 5_000 });

  assert.equal(result.status, 0, result.stderr);
  const payload = JSON.parse(result.stdout);
  assert.equal(payload.result.audit.as_of_date, "2026-06-01");
  assert.deepEqual(payload.result.structured.period, { from: "2025-10", to: "2026-05" });
  assert.equal(payload.result.structured.amount, 300);
});

test("financeqa canonical golden runner uses template-derived query instead of raw question", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-financeqa-golden-"));
  const questionFile = path.join(dir, "question.txt");
  fs.writeFileSync(questionFile, "老板随口问：上个月项目还有多少钱没收？", "utf8");
  const seenBodies: string[] = [];
  const server = http.createServer((req, res) => {
    let body = "";
    req.setEncoding("utf8");
    req.on("data", (chunk) => {
      body += chunk;
    });
    req.on("end", () => {
      seenBodies.push(body);
      res.setHeader("content-type", "application/json");
      res.end(JSON.stringify({
        jsonrpc: "2.0",
        id: "golden",
        result: {
          content: [{
            type: "text",
            text: JSON.stringify({
              final_answer: "2025-10~2026-05 项目应收 800000.00 元。来源：《fin-revenue-0601.xlsx》",
              data: {
                period_from: "2025-10",
                period_to: "2026-05",
                metrics: { "应收": 800000 },
                contract_summary: { receivable_amount: 800000 },
                source_note: "来源：《fin-revenue-0601.xlsx》"
              }
            })
          }]
        }
      }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  assert.ok(address && typeof address === "object");

  try {
    const result = await spawnNode([
      "examples/golden/financeqa_canonical_golden.mjs",
      "--template", "finance_project_receivable_unpaid",
      "--question-file", questionFile,
      "--as-of-date", "2026-06-26"
    ], {
      cwd: process.cwd(),
      env: {
        ...process.env,
        FINANCEQA_MCP_URL: `http://127.0.0.1:${address.port}/mcp`,
        FINANCEQA_MCP_READ_TOKEN: "test-token"
      }
    }, { timeoutMs: 5_000 });

    assert.equal(result.status, 0, result.stderr);
    const payload = JSON.parse(result.stdout);
    assert.equal(payload.result.source, "financeqa_canonical_golden");
    assert.match(payload.result.final_answer, /项目应收 800000\.00 元/);
    assert.equal(payload.result.structured.metric, "项目应收");
    assert.equal(payload.result.structured.amount, 800000);
    assert.equal(payload.result.structured.period.from, "2025-10");
    assert.equal(payload.result.structured.period.to, "2026-05");

    const request = JSON.parse(seenBodies[0]);
    assert.equal(request.params.name, "finance-query");
    assert.equal(request.params.arguments.query, "2025年10月至2026年5月，所有项目的应收未收是多少？");
    assert.doesNotMatch(request.params.arguments.query, /老板随口问/);
  } finally {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
});

test("financeqa canonical golden runner covers every financeqa preset template", async () => {
  const cases = [
    {
      template: "finance_latest_month_revenue",
      expectedQuery: "收入表中最新月份的营收是多少？",
      metric: "项目结算",
      amount: 100000,
      period: { from: "2026-05", to: "2026-05" }
    },
    {
      template: "finance_project_receivable_unpaid",
      expectedQuery: "2025年10月至2026年5月，所有项目的应收未收是多少？",
      metric: "项目应收",
      amount: 200000,
      period: { from: "2025-10", to: "2026-05" }
    },
    {
      template: "finance_project_invoiced_receivable_unpaid",
      expectedQuery: "2025年10月至2026年5月，所有项目已开票未回款是多少？",
      metric: "已开票未回款",
      amount: 300000,
      period: { from: "2025-10", to: "2026-05" }
    },
    {
      template: "finance_project_payable_unpaid",
      expectedQuery: "2025年10月至2026年5月，所有项目的应付未付是多少？",
      metric: "项目应付",
      amount: 400000,
      period: { from: "2025-10", to: "2026-05" }
    },
    {
      template: "finance_project_invoiced_payable_unpaid",
      expectedQuery: "2025年10月至2026年5月，所有项目已收票未付款是多少？",
      metric: "已收票未付款",
      amount: 500000,
      period: { from: "2025-10", to: "2026-05" }
    },
    {
      template: "finance_unpaid_projects",
      expectedQuery: "2025年10月至2026年5月，按项目列出已收票未付款金额。",
      metric: "已收票未付款",
      amount: 600000,
      period: { from: "2025-10", to: "2026-05" }
    }
  ];

  for (const item of cases) {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-financeqa-golden-template-"));
    const questionFile = path.join(dir, "question.txt");
    fs.writeFileSync(questionFile, `老板问法不参与金标语义：${item.template}`, "utf8");
    const seenBodies: string[] = [];
    const server = http.createServer((req, res) => {
      let body = "";
      req.setEncoding("utf8");
      req.on("data", (chunk) => {
        body += chunk;
      });
      req.on("end", () => {
        seenBodies.push(body);
        res.setHeader("content-type", "application/json");
        res.end(JSON.stringify({
          jsonrpc: "2.0",
          id: "golden",
          result: {
            content: [{
              type: "text",
              text: JSON.stringify({
                final_answer: `${item.period.from}~${item.period.to} ${item.metric} ${item.amount.toFixed(2)} 元。来源：《golden-fixture.xlsx》`,
                data: {
                  period_from: item.period.from,
                  period_to: item.period.to,
                  metrics: { [item.metric]: item.amount },
                  source_note: "来源：《golden-fixture.xlsx》"
                }
              })
            }]
          }
        }));
      });
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    assert.ok(address && typeof address === "object");

    try {
      const result = await spawnNode([
        "examples/golden/financeqa_canonical_golden.mjs",
        "--template", item.template,
        "--question-file", questionFile,
        "--as-of-date", "2026-06-26"
      ], {
        cwd: process.cwd(),
        env: {
          ...process.env,
          FINANCEQA_MCP_URL: `http://127.0.0.1:${address.port}/mcp`,
          FINANCEQA_MCP_READ_TOKEN: "test-token"
        }
      }, { timeoutMs: 5_000 });

      assert.equal(result.status, 0, `${item.template}: ${result.stderr}`);
      const payload = JSON.parse(result.stdout);
      assert.equal(payload.result.structured.metric, item.metric, item.template);
      assert.equal(payload.result.structured.amount, item.amount, item.template);
      assert.deepEqual(payload.result.structured.period, item.period, item.template);

      const request = JSON.parse(seenBodies[0]);
      assert.equal(request.params.name, "finance-query", item.template);
      assert.equal(request.params.arguments.query, item.expectedQuery, item.template);
      assert.doesNotMatch(request.params.arguments.query, /老板问法不参与金标语义/);
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
      fs.rmSync(dir, { recursive: true, force: true });
    }
  }
});

test("financeqa canonical golden runner fails closed for unsupported templates", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-financeqa-golden-unsupported-"));
  const questionFile = path.join(dir, "question.txt");
  fs.writeFileSync(questionFile, "未知问题", "utf8");

  const result = spawnSync("node", [
    "examples/golden/financeqa_canonical_golden.mjs",
    "--template", "unknown_template",
    "--question-file", questionFile,
    "--as-of-date", "2026-06-26"
  ], {
    cwd: process.cwd(),
    encoding: "utf8",
    env: {
      ...process.env,
      FINANCEQA_MCP_URL: "http://127.0.0.1:1/mcp",
      FINANCEQA_MCP_READ_TOKEN: "test-token"
    }
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /unsupported FinanceQA golden template/);
});

test("spawnNode helper kills hung children", async () => {
  const result = await spawnNode(["-e", "setInterval(() => {}, 1000)"], {
    cwd: process.cwd()
  }, { timeoutMs: 50 });

  assert.equal(result.timedOut, true);
  assert.match(result.stderr, /timed out after 50ms/);
});

test("financeqa low-frequency dry-run schedule examples only write local reports", () => {
  const files = [
    "examples/cleanup/claude-session-cleanup.sh",
    "examples/cleanup/hermes-json-cleanup.sh",
    "examples/cleanup/openclaw-jsonl-cleanup.sh",
    "examples/cleanup/run-agent-cleanups.sh",
    "examples/schedules/README.md",
    "examples/schedules/financeqa-daily.env.example",
    "examples/schedules/financeqa-daily.cron.example",
    "examples/schedules/financeqa-daily.service",
    "examples/schedules/financeqa-daily.timer",
    "examples/schedules/run-financeqa-dry-run.sh"
  ];
  const contents = files.map((file) => fs.readFileSync(file, "utf8")).join("\n");
  const scriptMode = fs.statSync("examples/schedules/run-financeqa-dry-run.sh").mode;

  assert.match(contents, /presets\/financeqa\.yaml/);
  assert.match(contents, /AGENT_PATROL_CONFIG=presets\/financeqa\.yaml/);
  assert.match(contents, /--config "\$CONFIG"/);
  assert.match(contents, /--suite "\$\{AGENT_PATROL_SUITE:-smoke\}"/);
  assert.match(contents, /tmp\/financeqa-dry-run/);
  assert.match(contents, /AGENT_PATROL_LIVE=1/);
  assert.match(contents, /uuidgen|\/proc\/sys\/kernel\/random\/uuid/);
  assert.doesNotMatch(contents, /date \+%F-%H/);
  assert.match(contents, /OPENCLAW_AGENT_CMD="/);
  assert.match(contents, /FINANCEQA_MCP_READ_TOKEN_FILE/);
  assert.match(contents, /FINANCEQA_GOLDEN_CMD/);
  assert.match(contents, /financeqa_canonical_golden\.mjs/);
  assert.match(contents, /FINANCEQA_REFERENCE_SNAPSHOT/);
  assert.match(contents, /financeqa_snapshot_reference\.mjs/);
  assert.match(contents, /flock/);
  assert.match(contents, /AGENT_PATROL_CLEANUP_CMD/);
  assert.match(contents, /AGENT_PATROL_CLEANUP_KINDS/);
  assert.match(contents, /AGENT_PATROL_SESSION_RETENTION_DAYS/);
  assert.match(contents, /openclaw-jsonl-cleanup\.sh/);
  assert.match(contents, /hermes-json-cleanup\.sh/);
  assert.match(contents, /claude-session-cleanup\.sh/);
  assert.match(contents, /patrol-\*\.jsonl/);
  assert.match(contents, /patrol-\*\.json/);
  assert.match(contents, /09,17/);
  assert.doesNotMatch(contents, /09,13,18/);
  assert.match(contents, /openclaw-finance/);
  assert.match(contents, /non-production|非生产|dry-run/i);
  assert.doesNotMatch(contents, /--deliver/);
  assert.doesNotMatch(contents, /\blzh\b/);
  assert.notEqual(scriptMode & 0o111, 0);
});

test("financeqa snapshot export example is read-only and table-whitelisted", () => {
  const contents = fs.readFileSync("examples/golden/export_financeqa_snapshot.sh", "utf8");

  assert.match(contents, /psql/);
  assert.match(contents, /gzip/);
  assert.match(contents, /fin_contracts/);
  assert.match(contents, /fin_fund_income/);
  assert.match(contents, /fin_fund_income_groups/);
  assert.match(contents, /fin_fund_income_group_members/);
  assert.match(contents, /fin_cost_settlements/);
  assert.match(contents, /fin_cost_settlement_groups/);
  assert.match(contents, /fin_cost_settlement_group_members/);
  assert.match(contents, /fin_file_mappings/);
  assert.doesNotMatch(contents, /fin_journal/);
  assert.doesNotMatch(contents, /fin_bank_statement/);
  assert.doesNotMatch(contents, /\bINSERT\b|\bUPDATE\b|\bDELETE\b|\bDROP\b|\bTRUNCATE\b/i);
});

function spawnNode(
  args: string[],
  options: SpawnOptionsWithoutStdio,
  control: { timeoutMs?: number } = {}
): Promise<{ status: number | null; stdout: string; stderr: string; timedOut: boolean }> {
  return new Promise((resolve, reject) => {
    const child = spawn("node", args, options);
    let stdout = "";
    let stderr = "";
    let timedOut = false;
    const timeoutMs = control.timeoutMs ?? 30_000;
    const timer = setTimeout(() => {
      timedOut = true;
      stderr += `spawnNode timed out after ${timeoutMs}ms\n`;
      child.kill("SIGTERM");
    }, timeoutMs);
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk: string) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk: string) => {
      stderr += chunk;
    });
    child.on("error", (err) => {
      clearTimeout(timer);
      reject(err);
    });
    child.on("close", (status) => {
      clearTimeout(timer);
      resolve({ status, stdout, stderr, timedOut });
    });
  });
}

test("agent cleanup examples prune only expired patrol session files", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-cleanup-"));
  const oldStamp = "202001010000";

  const cases = [
    {
      script: "examples/cleanup/openclaw-jsonl-cleanup.sh",
      envKey: "AGENT_PATROL_OPENCLAW_SESSION_DIR",
      oldPatrol: "patrol-finance-old.jsonl",
      freshPatrol: "patrol-finance-fresh.jsonl",
      regular: "regular-session.jsonl"
    },
    {
      script: "examples/cleanup/hermes-json-cleanup.sh",
      envKey: "AGENT_PATROL_HERMES_SESSION_DIR",
      oldPatrol: "patrol-hermes-old.json",
      freshPatrol: "patrol-hermes-fresh.json",
      regular: "session_cron_keep.json"
    },
    {
      script: "examples/cleanup/claude-session-cleanup.sh",
      envKey: "AGENT_PATROL_CLAUDE_SESSION_DIR",
      oldPatrol: "patrol-claude-old.jsonl",
      freshPatrol: "patrol-claude-fresh.jsonl",
      regular: "conversation-keep.jsonl"
    }
  ];

  for (const item of cases) {
    const sessionDir = path.join(dir, item.envKey);
    fs.mkdirSync(sessionDir, { recursive: true });
    for (const name of [item.oldPatrol, item.freshPatrol, item.regular]) {
      fs.writeFileSync(path.join(sessionDir, name), "{}", "utf8");
    }
    spawnSync("touch", ["-t", oldStamp, path.join(sessionDir, item.oldPatrol)], { encoding: "utf8" });
    spawnSync("touch", ["-t", oldStamp, path.join(sessionDir, item.regular)], { encoding: "utf8" });

    const result = spawnSync("bash", [item.script], {
      cwd: process.cwd(),
      encoding: "utf8",
      env: {
        ...process.env,
        [item.envKey]: sessionDir,
        AGENT_PATROL_SESSION_RETENTION_DAYS: "1"
      }
    });

    assert.equal(result.status, 0, `${item.script} failed: ${result.stderr}`);
    assert.equal(fs.existsSync(path.join(sessionDir, item.oldPatrol)), false, `${item.oldPatrol} should be removed`);
    assert.equal(fs.existsSync(path.join(sessionDir, item.freshPatrol)), true, `${item.freshPatrol} should stay`);
    assert.equal(fs.existsSync(path.join(sessionDir, item.regular)), true, `${item.regular} should stay`);
  }
});

test("agent cleanup dispatcher requires explicitly configured kinds", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-cleanup-dispatcher-"));
  const oldStamp = "202001010000";
  const files = [
    {
      envKey: "AGENT_PATROL_OPENCLAW_SESSION_DIR",
      name: "patrol-openclaw-old.jsonl"
    },
    {
      envKey: "AGENT_PATROL_HERMES_SESSION_DIR",
      name: "patrol-hermes-old.json"
    },
    {
      envKey: "AGENT_PATROL_CLAUDE_SESSION_DIR",
      name: "patrol-claude-old.jsonl"
    }
  ];
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    AGENT_PATROL_SESSION_RETENTION_DAYS: "1"
  };

  for (const item of files) {
    const sessionDir = path.join(dir, item.envKey);
    fs.mkdirSync(sessionDir, { recursive: true });
    const file = path.join(sessionDir, item.name);
    fs.writeFileSync(file, "{}", "utf8");
    spawnSync("touch", ["-t", oldStamp, file], { encoding: "utf8" });
    env[item.envKey] = sessionDir;
  }
  delete env.AGENT_PATROL_CLEANUP_KINDS;

  const result = spawnSync("bash", ["examples/cleanup/run-agent-cleanups.sh"], {
    cwd: process.cwd(),
    encoding: "utf8",
    env
  });

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /skip agent cleanup: no AGENT_PATROL_CLEANUP_KINDS/);
  for (const item of files) {
    assert.equal(
      fs.existsSync(path.join(dir, item.envKey, item.name)),
      true,
      `${item.name} should stay when cleanup kinds are not configured`
    );
  }
});
