# FinanceQA Low-frequency Dry-run Schedules

These examples run the real OpenClaw FinanceQA path as a low-frequency dry-run and write local report artifacts only. They do not send IM messages and do not call OpenClaw with delivery flags.

Before installing either cron or systemd templates:

1. Copy `financeqa-daily.env.example` to `financeqa-daily.env`.
2. Install this checkout on a non-production OpenClaw host.
3. Confirm that host has `openclaw-finance` installed and test data loaded.
4. Adjust `/opt/finance_qa/agent-patrol` to the actual checkout path.
5. Confirm `FINANCEQA_MCP_URL` and `FINANCEQA_MCP_READ_TOKEN_FILE` point at a read-only FinanceQA MCP endpoint/token.
6. Confirm the command manually:

```bash
test -d /root/.openclaw/extensions/openclaw-finance
```

```bash
cd /opt/finance_qa/agent-patrol
AGENT_PATROL_ENV_FILE=examples/schedules/financeqa-daily.env \
examples/schedules/run-financeqa-dry-run.sh
```

The default schedule runs two `smoke` suite dry-runs per day, with jitter in the systemd timer. Reports are written under `tmp/financeqa-dry-run/`. The script also prunes old OpenClaw patrol transcripts matching `patrol-finance-*.jsonl`; tune `AGENT_PATROL_SESSION_RETENTION_DAYS` in the env file.
