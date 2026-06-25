# Agent Patrol MVP

## Goal

Build a standalone patrol tool that can run from cron/systemd, generate varied questions, ask the real agent surface, and write a report without sending IM messages or writing business data.

The first version is intentionally a health patrol for agent paths: can the agent be invoked, can the answer be parsed, is the session isolated, and did the answer satisfy simple scoring rules. Full business-answer accuracy checks are a later layer.

## Boundary

```text
                              dynamic cases
                                   |
                         actual agent command
                  OpenClaw / Claude SDK dry-run / other agent CLI
                                   |
                         final answer + trace
                                   |
              simple scoring rules + read-only tool policy
                                   |
                             report artifacts
```

Direct `finance-query` or bossa MCP calls are not the system under test. They can be added later as independent fact references, anchor discovery, and diagnostics. A run that has only direct MCP output and no OpenClaw/Claude Agent SDK output is invalid.

The config field currently named `oracle` is a legacy schema name. In the MVP it means read-only tool policy and future reference-source metadata; it does not mean the patrol tool is already computing a business standard answer.

## Independent Project Shape

The implementation lives under `agent-patrol/` with its own TypeScript `package.json`, tests, docs, and CLI. It does not import `finance_qa` Go packages or bossa internals. The core has one runtime integration shape:

- A command adapter executes an external agent command.
- The external command returns a common JSON envelope with answer, session, and optional tool-call metadata.
- YAML presets provide target-specific commands, templates, and scoring.
- Example wrappers live under `examples/`; they are not core SDK adapters.

This keeps the巡检工具 portable to other agent systems that expose an agent runner plus simple scoring rules.

## KISS Boundary

- Do not add OpenClaw-specific or Claude-SDK-specific code to `src`.
- Do not put finance/bossa business rules in `src` or generic examples.
- Do not implement write-capable tools.
- Do not implement MCP fact-reference logic in the MVP.
- Keep live runners opt-in and example-only.

## Safety Rules

- No production IM delivery.
- No boss cron trigger.
- No business write tools.
- Explicit write-tool denylist even when a config allowlist is wrong.
- Isolated patrol session IDs.
- Fail closed when the actual agent envelope is missing.
- Redact tokens and credential-looking strings from reports.

## Scoring Rules

MVP scoring is deterministic string/amount matching, not an LLM judge.

- `mustContain`: every listed term must appear in the answer.
- `mustContainAny`: every listed group must have at least one matching term. Use this for acceptable wording variants such as `["项目口径", "所有项目", "项目应付"]`.
- `mustNotContain`: none of the listed terms may appear.
- `amounts`: each amount must appear in a normalized numeric form.

## MVP Modules

- `src/config.ts`: load YAML/JSON config, expand environment variables, validate runner and read-only policy shape.
- `src/guard.ts`: read-only allowlist and write-tool denylist.
- `src/cases.ts`: deterministic dynamic case generation from templates and anchors.
- `src/runners/command_runner.ts`: generic JSON command adapter for OpenClaw and Claude SDK dry-runs.
- `src/run.ts`: run generated cases through the actual agent runner, score the answers, and write reports.
- `src/scorer.ts`: hard-rule scoring for actual-vs-expected checks.
- `src/report.ts`: JSONL/Markdown report writer with redaction.
- `src/index.ts`: CLI for `doctor`, `generate`, and `run`.

## Report Artifacts

Each `run` writes a directory of local artifacts:

- `manifest.json`: suite, seed, and generation timestamp.
- `cases.json`: generated questions and scoring rules.
- `raw_results.jsonl`: one raw agent answer per line, including runner type, session id, and case duration.
- `scores.json`: deterministic scoring output.
- `summary.json`: cron-friendly structured summary with aggregate accuracy and failed case details.
- `summary.md`: human-readable summary.

The CLI also prints the aggregate JSON to stdout and exits non-zero when accuracy is below `report.minAccuracy`.

## First Acceptance Target

Local acceptance:

```bash
cd agent-patrol
npm install
npm test
npm run typecheck
npm run start -- generate --config config.example.yaml --suite smoke --out tmp/generate
npm run start -- doctor --config config.example.yaml
npm run start -- run --config examples/stub.command-agent.yaml --suite smoke --out tmp/stub-command-agent
```

Business presets are optional YAML examples:

```bash
npm run start -- generate --config presets/financeqa.yaml --suite smoke --out tmp/financeqa-generate
npm run start -- generate --config presets/bossa.yaml --suite smoke --out tmp/bossa-generate
```

Live acceptance is opt-in only. The first live smoke test uses OpenClaw on `ssh clawdbot` and must prove:

- The answer came from `openclaw agent --json`, not a direct MCP call.
- The command did not include `--deliver`.
- Session metadata is present and does not reuse `agent:main:main`.
- The result report is written locally.

```bash
AGENT_PATROL_LIVE=1 \
AGENT_PATROL_OPENCLAW_HOST=clawdbot \
npm run start -- run --config examples/live/clawdbot-openclaw.example.yaml --suite smoke --out tmp/live-clawdbot-openclaw
```

FinanceQA live smoke should run against the real OpenClaw agent surface on `ssh lzh`:

```bash
AGENT_PATROL_LIVE=1 \
OPENCLAW_AGENT_CMD='node examples/runners/openclaw_ssh_runner.mjs --host lzh --question-file {questionFile} --session-id {sessionId} --thinking off --timeout 300' \
FINANCEQA_MCP_URL=http://127.0.0.1/stub \
npm run start -- run --config presets/financeqa.yaml --suite smoke --out tmp/live-lzh-financeqa
```

For the default `main` agent on `lzh`, do not pass `--agent main`: that path maps to the protected `agent:main:main` session and defeats explicit patrol session IDs. The runner rejects this when session isolation is required.
