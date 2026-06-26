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

For FinanceQA dry-runs that need structured golden evidence, set `FINANCEQA_GOLDEN_CMD` to `node examples/golden/financeqa_canonical_golden.mjs --template {template} --question-file {questionFile}` and enable the `goldenReference` block in the patrol YAML. The example command derives a canonical FinanceQA query from the case template; it reads the original question file only for audit context.

The script can run `AGENT_PATROL_CLEANUP_CMD` after each run; `AGENT_PATROL_CLEANUP_KINDS` must explicitly match the agent runner(s) used by that patrol job, such as `openclaw`, `hermes`, `claude`, or a comma-separated list. Do not enable cleanup for agent runtimes that the patrol job does not use. The cleanup adapters prune old `patrol-*` transcripts only. Tune `AGENT_PATROL_SESSION_RETENTION_DAYS` in the env file.
