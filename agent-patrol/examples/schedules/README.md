# FinanceQA Daily Patrol Schedules

These examples run the real OpenClaw FinanceQA path and write local report artifacts only. They do not send IM messages and do not call OpenClaw with delivery flags.

Before installing either cron or systemd templates:

1. Copy `financeqa-daily.env.example` to `financeqa-daily.env`.
2. Adjust `/opt/finance_qa/agent-patrol` to the actual checkout path.
3. Confirm the command manually:

```bash
cd /opt/finance_qa/agent-patrol
set -a
source examples/schedules/financeqa-daily.env
set +a
npm run start -- run --config presets/financeqa.yaml --suite daily --out tmp/financeqa-daily/manual
```

Reports are written under `tmp/financeqa-daily/`.
