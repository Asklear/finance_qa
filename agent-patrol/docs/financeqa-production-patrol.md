# FinanceQA Production Patrol

This document records the production-safe FinanceQA patrol profile for the production host.

## Goal

Run hourly local dry-runs against the real OpenClaw + FinanceQA MCP path, compare answers with a read-only snapshot golden reference, and write reports locally without sending boss messages or restarting FinanceQA/OpenClaw services.

## Runtime Paths

- Local source worktree: `/Users/gaorongvc/.config/superpowers/worktrees/finance_qa/agent-patrol-standalone/agent-patrol`
- Production install path: `/opt/finance_qa/agent-patrol`
- Production report path: `/var/log/agent-patrol/financeqa`
- Production OpenClaw sessions: `/root/.openclaw/agents/main/sessions`
- Production FinanceQA MCP: `http://127.0.0.1:3009/mcp`
- Production FinanceQA read token: `/root/finance_qa/secrets/mcp_read_token`

## Production Profile

Use these files:

- `presets/financeqa-production.yaml`
- `examples/schedules/financeqa-production-hourly.env`
- `examples/schedules/financeqa-production-hourly.service`
- `examples/schedules/financeqa-production-hourly.timer`

The timer runs hourly with jitter:

```ini
OnCalendar=*-*-* *:07:00
RandomizedDelaySec=5m
```

The systemd service sets `AGENT_PATROL_ENV_FILE=/opt/finance_qa/agent-patrol/examples/schedules/financeqa-production-hourly.env` and lets the wrapper source that file. Do not replace it with systemd `EnvironmentFile=`, because same-file references in command variables need bash expansion.

The production question-rewrite profile sets `AGENT_PATROL_LLM_RESPONSE_FORMAT=json_object`, so compatible LLM providers are asked for JSON output at the API layer. The parser still falls back safely if the provider returns malformed text.

## Reference Source

The golden reference comes from a read-only snapshot export of the live `fin_*` tables using:

```bash
examples/golden/export_financeqa_snapshot.sh
```

The snapshot is written to:

```bash
tmp/reference-snapshots/financeqa-production-latest.json.gz
```

The golden provider then reads that snapshot:

```bash
node examples/golden/financeqa_snapshot_reference.mjs \
  --template {template} \
  --question-file {questionFile} \
  --snapshot tmp/reference-snapshots/financeqa-production-latest.json.gz
```

Direct `finance-query` remains diagnostic only. It must not be treated as the 90% accuracy reference when `goldenReference` is configured.

## FinanceQA Metric Boundaries

The FinanceQA golden templates keep project payable and invoice payable separate:

- `finance_project_payable_unpaid` and `finance_unpaid_projects` use project payable: project cost minus paid amount.
- `finance_project_invoiced_payable_unpaid` uses invoice payable: received invoice amount minus paid amount and invoice-open offsets.

Questions that only say `未付款`, `应付未付`, or `项目成本口径未付款` are scored against project payable. Questions that explicitly say `已收票`, `收到发票`, or `发票未付` are scored against invoice payable.

## Safety Rules

- Do not configure `AGENT_PATROL_PREPARE_CMD=examples/schedules/prepare-financeqa-snapshot-mirror.sh` on production.
- Do not restart `financeqa-mcp.service` or `openclaw-gateway.service` for patrol deployment.
- Do not use OpenClaw delivery flags.
- Clean up only the agent runtime actually used by the patrol job. The production profile uses OpenClaw only:

```bash
AGENT_PATROL_CLEANUP_KINDS=openclaw
```

Cleanup only removes old `patrol-*` session files. It does not remove manual user sessions.

## Verification Commands

Run doctor before enabling the timer:

```bash
cd /opt/finance_qa/agent-patrol
set -a
source examples/schedules/financeqa-production-hourly.env
source /root/finance_qa/.env
source tmp/secrets/agent-patrol-llm.env
set +a
npm run start -- doctor \
  --config "$AGENT_PATROL_CONFIG" \
  --require-golden-reference \
  --require-resolved-env
```

Run one local-report dry-run:

```bash
cd /opt/finance_qa/agent-patrol
AGENT_PATROL_ENV_FILE=examples/schedules/financeqa-production-hourly.env \
examples/schedules/run-financeqa-dry-run.sh
```

Check the timer:

```bash
systemctl list-timers --all 'financeqa-production-hourly.timer' --no-pager
systemctl status financeqa-production-hourly.timer --no-pager
```

Check latest report summary:

```bash
node -e '
const fs = require("fs");
const path = require("path");
const base = "/var/log/agent-patrol/financeqa";
const latest = fs.readdirSync(base).filter((name) => /^20|deploy-smoke-/.test(name)).sort().at(-1);
const summary = JSON.parse(fs.readFileSync(path.join(base, latest, "summary.json"), "utf8"));
console.log(JSON.stringify({ latest, aggregate: summary.aggregate, failedCases: (summary.failedCases || []).length }, null, 2));
'
```

## Rollback

Disable the patrol timer:

```bash
systemctl disable --now financeqa-production-hourly.timer
```

This stops future patrol runs only. It does not change FinanceQA MCP, OpenClaw gateway, Feishu scan timers, or boss cron jobs.
