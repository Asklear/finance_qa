# FinanceQA Low-frequency Dry-run Schedules

These examples run the real OpenClaw FinanceQA path as a low-frequency dry-run and write local report artifacts only. They do not send IM messages and do not call OpenClaw with delivery flags.

Before installing either cron or systemd templates:

1. Copy `financeqa-daily.env.example` to `financeqa-daily.env`.
2. Install this checkout on a non-production OpenClaw host.
3. Confirm that host has `openclaw-finance` installed and test data loaded.
4. Adjust `/opt/finance_qa/agent-patrol` to the actual checkout path.
5. Confirm `FINANCEQA_MCP_URL` and `FINANCEQA_MCP_READ_TOKEN_FILE` point at a read-only FinanceQA MCP endpoint/token. This is the direct-tool baseline, not the golden answer.
6. Confirm the command manually:

```bash
test -d /root/.openclaw/extensions/openclaw-finance
```

```bash
cd /opt/finance_qa/agent-patrol
AGENT_PATROL_ENV_FILE=examples/schedules/financeqa-daily.env \
examples/schedules/run-financeqa-dry-run.sh
```

The default schedule runs two `smoke` suite dry-runs per day, with jitter in the systemd timer. Reports are written under `tmp/financeqa-dry-run/`. Without a configured structured `goldenReference`, these reports use direct `finance-query` only as a diagnostic baseline. Do not use that mode as a 90% business-accuracy gate.

For FinanceQA dry-runs that need structured golden evidence, prefer a local snapshot reference:

```bash
# On a host with read-only RDS env such as PGHOST/PGUSER/PGDATABASE/FINANCEQA_PG_SCHEMA:
FINANCEQA_SNAPSHOT_OUTPUT=tmp/reference-snapshots/financeqa-latest.json.gz \
examples/golden/export_financeqa_snapshot.sh
```

Then set `FINANCEQA_REFERENCE_SNAPSHOT` and `FINANCEQA_GOLDEN_CMD` as shown in `financeqa-daily.env.example`, copy the preset YAML to a local config, enable its `goldenReference` block, and point `AGENT_PATROL_CONFIG` at that file. The snapshot command reads `fin_*` rows from the local `.json.gz` and does not call `finance-query`; the export includes direct rows, merged-group rows, and group-member rows so the reference can net merged amounts against member receipts/payments. The older `financeqa_canonical_golden.mjs` command is still available for diagnostics, but it canonicalizes the prompt and then calls `finance-query`, so it is not an independent business reference.

This snapshot pattern is a FinanceQA-specific provider implementation. Other agent targets can use a different command, MCP, or HTTP reference provider as long as it returns the same golden-reference envelope.

The script can run `AGENT_PATROL_CLEANUP_CMD` after each run; `AGENT_PATROL_CLEANUP_KINDS` must explicitly match the agent runner(s) used by that patrol job, such as `openclaw`, `hermes`, `claude`, or a comma-separated list. Do not enable cleanup for agent runtimes that the patrol job does not use. The cleanup adapters prune old `patrol-*` transcripts only. Tune `AGENT_PATROL_SESSION_RETENTION_DAYS` in the env file.
