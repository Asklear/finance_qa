#!/usr/bin/env node
import fs from "node:fs";
import zlib from "node:zlib";

const PERIOD_START = { year: 2025, month: 10 };
const DEFAULT_TIME_ZONE = "Asia/Shanghai";

const TEMPLATE_DEFINITIONS = {
  finance_latest_month_revenue: {
    metric: "项目结算",
    family: "fund",
    periodMode: "latest",
    amount: (totals) => totals.settlement,
    headline: "项目结算"
  },
  finance_project_receivable_unpaid: {
    metric: "项目应收",
    family: "fund",
    periodMode: "range",
    amount: (totals) => totals.open,
    headline: "项目应收"
  },
  finance_project_invoiced_receivable_unpaid: {
    metric: "已开票未回款",
    family: "fund",
    periodMode: "range",
    amount: (totals) => totals.invoiceOpen,
    headline: "已开票未回款"
  },
  finance_project_payable_unpaid: {
    metric: "项目应付",
    family: "cost",
    periodMode: "range",
    amount: (totals) => totals.open,
    headline: "项目应付"
  },
  finance_project_invoiced_payable_unpaid: {
    metric: "已收票未付款",
    family: "cost",
    periodMode: "range",
    amount: (totals) => totals.invoiceOpen,
    headline: "已收票未付款"
  },
  finance_unpaid_projects: {
    metric: "项目应付",
    family: "cost",
    periodMode: "range",
    amount: (totals) => totals.open,
    headline: "项目应付",
    includeItems: true
  }
};

const TABLES_BY_FAMILY = {
  fund: {
    tableType: "fund-income",
    direct: "fin_fund_income",
    group: "fin_fund_income_groups",
    member: "fin_fund_income_group_members",
    movement: "received_amount",
    settlementLabel: "项目结算",
    movementLabel: "已到账",
    invoiceLabel: "已开票"
  },
  cost: {
    tableType: "cost-settlements",
    direct: "fin_cost_settlements",
    group: "fin_cost_settlement_groups",
    member: "fin_cost_settlement_group_members",
    movement: "paid_amount",
    settlementLabel: "项目成本",
    movementLabel: "已付款",
    invoiceLabel: "已收票"
  }
};

async function main() {
  try {
    const args = parseArgs(process.argv.slice(2));
    const definition = TEMPLATE_DEFINITIONS[args.template];
    if (!definition) {
      throw new Error(`unsupported FinanceQA snapshot template: ${args.template || "(missing)"}`);
    }
    const snapshotPath = args.snapshot || process.env.FINANCEQA_REFERENCE_SNAPSHOT;
    if (!snapshotPath) {
      throw new Error("missing --snapshot or FINANCEQA_REFERENCE_SNAPSHOT");
    }

    const question = readQuestionForAudit(args.questionFile);
    const now = args.nowEpochMs ? new Date(Number(args.nowEpochMs)) : new Date();
    const asOf = parseDate(args.asOfDate ?? todayIsoDate(now, args.timeZone || process.env.AGENT_PATROL_TIMEZONE || DEFAULT_TIME_ZONE));
    const snapshot = readSnapshot(snapshotPath);
    const tableSpec = TABLES_BY_FAMILY[definition.family];
    const facts = financeFacts(snapshot, tableSpec);
    const allRows = [...facts.directRows, ...facts.groupRows];
    const period = resolvePeriod(allRows, definition.periodMode, asOf);
    const rows = allRows.filter((row) => inPeriod(row, period));
    const totals = collectTotals(facts, tableSpec, period, definition);
    const amount = round2(definition.amount(totals));
    const source = collectSources(snapshot, tableSpec.tableType, rows);
    const finalAnswer = renderAnswer({
      definition,
      period,
      amount,
      totals,
      source,
      metadata: snapshot.metadata
    });

    writeJson({
      result: {
        source: "financeqa_snapshot_reference",
        template: args.template,
        final_answer: finalAnswer,
        structured: {
          metric: definition.metric,
          amount,
          period,
          source,
          row_count: totals.rowCount,
          totals: {
            settlement: round2(totals.settlement),
            movement: round2(totals.movement),
            invoice: round2(totals.invoice),
            open: round2(totals.open),
            invoice_open: round2(totals.invoiceOpen)
          },
          items: totals.items.slice(0, 20),
          freshness: {
            generated_at: stringValue(snapshot.metadata?.generated_at),
            source_database: stringValue(snapshot.metadata?.source_database),
            source_schema: stringValue(snapshot.metadata?.source_schema)
          }
        },
        audit: {
          question_file: args.questionFile,
          original_question_length: question.length,
          as_of_date: `${asOf.year}-${pad2(asOf.month)}-${pad2(asOf.day)}`,
          snapshot_path: snapshotPath
        }
      }
    });
  } catch (err) {
    console.error(err instanceof Error ? err.message : String(err));
    process.exit(1);
  }
}

function parseArgs(argv) {
  const parsed = {};
  for (let index = 0; index < argv.length; index += 1) {
    const item = argv[index];
    if (!item.startsWith("--")) {
      throw new Error(`unexpected argument: ${item}`);
    }
    const key = item.slice(2).replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
    const value = argv[index + 1];
    if (!value || value.startsWith("--")) {
      throw new Error(`missing value for ${item}`);
    }
    parsed[key] = value;
    index += 1;
  }
  if (!parsed.template) throw new Error("missing --template");
  if (!parsed.questionFile) throw new Error("missing --question-file");
  return parsed;
}

function readQuestionForAudit(questionFile) {
  if (!questionFile) return "";
  return fs.readFileSync(questionFile, "utf8");
}

function readSnapshot(snapshotPath) {
  const bytes = fs.readFileSync(snapshotPath);
  const text = snapshotPath.endsWith(".gz") ? zlib.gunzipSync(bytes).toString("utf8") : bytes.toString("utf8");
  const parsed = JSON.parse(text);
  const tables = asRecord(parsed.tables);
  if (!tables) {
    throw new Error("snapshot missing tables object");
  }
  return {
    metadata: asRecord(parsed.metadata) ?? {},
    tables
  };
}

function financeFacts(snapshot, tableSpec) {
  const contracts = new Map(arrayValue(snapshot.tables.fin_contracts)
    .map((row) => asRecord(row) ?? {})
    .map((row) => [stringValue(row.contract_id) ?? "", {
      contract_id: stringValue(row.contract_id) ?? "",
      customer_name: stringValue(row.customer_name) ?? "",
      contract_content: stringValue(row.contract_content) ?? ""
    }])
    .filter(([contractId]) => contractId));
  return {
    contracts,
    directRows: arrayValue(snapshot.tables[tableSpec.direct])
      .map((row) => normalizeFinanceRow(row, tableSpec, "direct"))
      .filter((row) => row.year_month),
    groupRows: arrayValue(snapshot.tables[tableSpec.group])
      .map((row) => normalizeFinanceRow(row, tableSpec, "group"))
      .filter((row) => row.year_month),
    memberRows: arrayValue(snapshot.tables[tableSpec.member])
      .map(normalizeMemberRow)
      .filter((row) => row.group_id)
  };
}

function normalizeFinanceRow(value, tableSpec, kind) {
  const row = asRecord(value) ?? {};
  return {
    kind,
    id: stringValue(row.id) ?? numericString(row.id),
    contract_id: stringValue(row.contract_id) ?? "",
    customer_name: stringValue(row.customer_name) ?? "",
    source_start_row: numericValue(row.source_start_row),
    year_month: stringValue(row.year_month) ?? "",
    settlement_amount: numericValue(row.settlement_amount),
    movement_amount: numericValue(row[tableSpec.movement]),
    invoice_amount: numericValue(row.invoice_amount),
    invoice_open_offset_amount: numericValue(row.invoice_open_offset_amount)
  };
}

function normalizeMemberRow(value) {
  const row = asRecord(value) ?? {};
  return {
    group_id: stringValue(row.group_id) ?? numericString(row.group_id),
    contract_id: stringValue(row.contract_id) ?? "",
    source_row_number: numericValue(row.source_row_number)
  };
}

function resolvePeriod(rows, mode, asOf) {
  if (mode === "latest") {
    const latestAvailable = maxMonth(rows.map((row) => row.year_month).filter(Boolean));
    if (!latestAvailable) {
      throw new Error("snapshot has no finance periods for requested template");
    }
    return { from: latestAvailable, to: latestAvailable };
  }

  const previous = previousCompleteMonth(asOf);
  const maxAvailable = maxMonth(rows.map((row) => row.year_month).filter((month) => month <= formatMonth(previous)));
  if (!maxAvailable) {
    throw new Error("snapshot has no complete finance periods for requested template");
  }
  const requestedTo = minMonth(formatMonth(previous), maxAvailable);
  return {
    from: formatMonth(PERIOD_START),
    to: requestedTo
  };
}

function collectTotals(facts, tableSpec, period, definition) {
  const directRows = facts.directRows.filter((row) => inPeriod(row, period));
  const groupRows = facts.groupRows.filter((row) => inPeriod(row, period));
  const directByContractMonth = groupDirectRowsByContractMonth(directRows);
  const membersByGroup = groupMembersByGroup(facts.memberRows);
  const coveredDirectKeys = new Set();
  const openRows = [];
  const totals = { settlement: 0, movement: 0, invoice: 0, open: 0, invoiceOpen: 0, rowCount: directRows.length + groupRows.length, tableSpec, items: [] };

  for (const row of [...directRows, ...groupRows]) {
    totals.settlement += row.settlement_amount;
    totals.movement += row.movement_amount;
    totals.invoice += row.invoice_amount;
  }

  for (const group of groupRows) {
    const memberRows = membersByGroup.get(group.id) ?? [];
    let settlement = group.settlement_amount;
    let movement = group.movement_amount;
    let invoice = group.invoice_amount;
    let offset = group.invoice_open_offset_amount;
    for (const member of memberRows) {
      const key = `${member.contract_id}\t${group.year_month}`;
      const matchedDirectRows = directByContractMonth.get(key) ?? [];
      if (matchedDirectRows.length === 0) continue;
      coveredDirectKeys.add(key);
      for (const direct of matchedDirectRows) {
        settlement += direct.settlement_amount;
        movement += direct.movement_amount;
        invoice += direct.invoice_amount;
        offset += direct.invoice_open_offset_amount;
      }
    }
    openRows.push({
      settlement_amount: settlement,
      movement_amount: movement,
      invoice_amount: invoice,
      invoice_open_offset_amount: offset,
      item_name: groupItemName(group, memberRows, facts.contracts),
      year_month: group.year_month
    });
  }

  for (const row of directRows) {
    const key = `${row.contract_id}\t${row.year_month}`;
    if (coveredDirectKeys.has(key)) continue;
    openRows.push({
      ...row,
      item_name: directItemName(row, facts.contracts)
    });
  }

  for (const row of openRows) {
    const settlementOpen = Math.max(row.settlement_amount - row.movement_amount, 0);
    const invoiceOpen = Math.max(row.invoice_amount - row.movement_amount - row.invoice_open_offset_amount, 0);
    totals.open += settlementOpen;
    totals.invoiceOpen += invoiceOpen;
    const itemAmount = definition.itemAmount === "invoiceOpen" ? invoiceOpen : settlementOpen;
    if (itemAmount > 0) {
      totals.items.push({
        name: row.item_name,
        amount: round2(itemAmount),
        period: row.year_month
      });
    }
  }

  totals.items = rollupItems(totals.items);
  return totals;
}

function groupDirectRowsByContractMonth(rows) {
  const out = new Map();
  for (const row of rows) {
    const key = `${row.contract_id}\t${row.year_month}`;
    const list = out.get(key) ?? [];
    list.push(row);
    out.set(key, list);
  }
  return out;
}

function groupMembersByGroup(memberRows) {
  const out = new Map();
  for (const row of memberRows) {
    const list = out.get(row.group_id) ?? [];
    list.push(row);
    out.set(row.group_id, list);
  }
  return out;
}

function directItemName(row, contracts) {
  const contract = contracts.get(row.contract_id);
  return contractLabel(contract) || row.contract_id || "未命名项目";
}

function groupItemName(group, memberRows, contracts) {
  const labels = unique(memberRows
    .map((member) => contractLabel(contracts.get(member.contract_id)))
    .filter(Boolean));
  if (labels.length === 1) return labels[0];
  if (labels.length > 1) {
    const prefix = group.customer_name || "合并项目";
    return `${prefix}（${labels.slice(0, 2).join("、")}${labels.length > 2 ? `等${labels.length}项` : ""}）`;
  }
  return group.customer_name || group.id || "合并项目";
}

function contractLabel(contract) {
  if (!contract) return "";
  if (contract.customer_name && contract.contract_content) return `${contract.customer_name}/${contract.contract_content}`;
  return contract.contract_content || contract.customer_name || contract.contract_id;
}

function rollupItems(items) {
  const byName = new Map();
  for (const item of items) {
    const existing = byName.get(item.name) ?? { name: item.name, amount: 0, periods: [] };
    existing.amount += item.amount;
    existing.periods.push(item.period);
    byName.set(item.name, existing);
  }
  return [...byName.values()]
    .map((item) => ({
      name: item.name,
      amount: round2(item.amount),
      periods: unique(item.periods).sort()
    }))
    .sort((a, b) => b.amount - a.amount || a.name.localeCompare(b.name));
}

function inPeriod(row, period) {
  return row.year_month >= period.from && row.year_month <= period.to;
}

function collectSources(snapshot, tableType, rows) {
  const mappings = arrayValue(snapshot.tables.fin_file_mappings)
    .map((item) => asRecord(item) ?? {})
    .filter((item) => stringValue(item.table_type) === tableType);
  const neededPeriods = new Set(rows.map((row) => quarterPeriod(row.year_month)));
  const selected = mappings
    .filter((item) => neededPeriods.has(stringValue(item.period) ?? ""))
    .sort((a, b) => (stringValue(a.period) ?? "").localeCompare(stringValue(b.period) ?? ""));
  return {
    files: unique(selected.map((item) => stringValue(item.file_name)).filter(Boolean)),
    version_ids: unique(selected.map((item) => stringValue(item.source_version_id)).filter(Boolean)),
    updated_at: maxTimestamp(selected.map((item) => stringValue(item.updated_at)).filter(Boolean))
  };
}

function renderAnswer(input) {
  const periodText = `${input.period.from}~${input.period.to}`;
  const totals = input.totals;
  const tableSpec = totals.tableSpec;
  const sourceText = input.source.files.length > 0 ? ` 来源：${input.source.files.map((file) => `《${file}》`).join("；")}` : "";
  const updateText = input.source.updated_at ? ` 来源更新时间：${input.source.updated_at}` : "";
  const freshnessText = input.metadata?.generated_at ? ` 快照生成时间：${input.metadata.generated_at}` : "";
  const itemText = input.definition.includeItems && totals.items.length > 0
    ? ` 明细前${Math.min(totals.items.length, 5)}项：${totals.items.slice(0, 5).map((item) => `${item.name} ${formatAmount(item.amount)} 元`).join("；")}。`
    : "";
  return `${periodText} DB金标口径先看项目汇总：${input.definition.headline} ${formatAmount(input.amount)} 元。` +
    `补充${tableSpec.settlementLabel} ${formatAmount(totals.settlement)} 元、${tableSpec.movementLabel} ${formatAmount(totals.movement)} 元、${tableSpec.invoiceLabel} ${formatAmount(totals.invoice)} 元。` +
    `${itemText}${sourceText}${updateText}${freshnessText}`;
}

function parseDate(value) {
  const match = String(value).match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!match) {
    throw new Error(`invalid date: ${value}`);
  }
  return { year: Number(match[1]), month: Number(match[2]), day: Number(match[3]) };
}

function todayIsoDate(now, timeZone) {
  const parts = datePartsInTimeZone(now, timeZone);
  return `${parts.year}-${pad2(parts.month)}-${pad2(parts.day)}`;
}

function datePartsInTimeZone(date, timeZone) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit"
  }).formatToParts(date);
  const values = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return { year: Number(values.year), month: Number(values.month), day: Number(values.day) };
}

function previousCompleteMonth(date) {
  let year = date.year;
  let month = date.month - 1;
  if (month === 0) {
    year -= 1;
    month = 12;
  }
  return { year, month };
}

function formatMonth(value) {
  return `${value.year}-${pad2(value.month)}`;
}

function quarterPeriod(month) {
  const match = String(month).match(/^(\d{4})-(\d{2})$/);
  if (!match) return "";
  const quarter = Math.floor((Number(match[2]) - 1) / 3) + 1;
  return `${match[1]}-Q${quarter}`;
}

function minMonth(a, b) {
  return a <= b ? a : b;
}

function maxMonth(months) {
  return months.reduce((max, item) => (!max || item > max ? item : max), "");
}

function maxTimestamp(values) {
  return values.reduce((max, item) => (!max || item > max ? item : max), "");
}

function round2(value) {
  return Math.round((value + Number.EPSILON) * 100) / 100;
}

function formatAmount(value) {
  return round2(value).toFixed(2);
}

function pad2(value) {
  return String(value).padStart(2, "0");
}

function numericValue(value) {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) {
    const parsed = Number(value.replace(/,/g, ""));
    if (Number.isFinite(parsed)) return parsed;
  }
  return 0;
}

function numericString(value) {
  if (typeof value === "number" && Number.isFinite(value)) return String(value);
  return "";
}

function stringValue(value) {
  return typeof value === "string" && value.trim() ? value.trim() : undefined;
}

function arrayValue(value) {
  return Array.isArray(value) ? value : [];
}

function asRecord(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : undefined;
}

function unique(values) {
  return [...new Set(values)];
}

function writeJson(value) {
  process.stdout.write(`${JSON.stringify(value, null, 2)}\n`);
}

main();
