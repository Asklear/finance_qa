# FinanceQA Daily Patrol Schedules

These examples run the real OpenClaw FinanceQA path and write local report artifacts only. They do not send IM messages and do not call OpenClaw with delivery flags.

Before installing either cron or systemd templates:

1. Copy `financeqa-daily.env.example` to `financeqa-daily.env`.
2. Point `AGENT_PATROL_OPENCLAW_HOST` at a non-production OpenClaw host.
3. Confirm that host has `openclaw-finance` installed and test data loaded.
4. Adjust `/opt/finance_qa/agent-patrol` to the actual checkout path.
5. Confirm the command manually:

```bash
ssh "$AGENT_PATROL_OPENCLAW_HOST" 'test -d /root/.openclaw/extensions/openclaw-finance'
```

```bash
cd /opt/finance_qa/agent-patrol
set -a
source examples/schedules/financeqa-daily.env
set +a
npm run start -- run --config presets/financeqa.yaml --suite daily --out tmp/financeqa-daily/manual
```

Reports are written under `tmp/financeqa-daily/`.
