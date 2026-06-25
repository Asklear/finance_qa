#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import fs from "node:fs";

const args = new Map();
for (let i = 2; i < process.argv.length; i += 2) {
  args.set(process.argv[i], process.argv[i + 1]);
}

if (process.env.AGENT_PATROL_LIVE !== "1") {
  console.error("Refusing live OpenClaw run: set AGENT_PATROL_LIVE=1 explicitly.");
  process.exit(2);
}

const host = args.get("--host");
const questionFile = args.get("--question-file");
const sessionId = args.get("--session-id");
const agent = args.get("--agent");
const thinking = args.get("--thinking") ?? "off";
const timeout = args.get("--timeout") ?? "180";

if (!host || !questionFile || !sessionId) {
  console.error("usage: openclaw_ssh_runner.mjs --host <ssh-host> --question-file <path> --session-id <id> [--agent <id>]");
  process.exit(2);
}

const question = fs.readFileSync(questionFile, "utf8");
const remoteArgs = ["openclaw", "agent", "--json", "--message", question, "--session-id", sessionId, "--thinking", thinking, "--timeout", timeout];
if (agent) {
  remoteArgs.splice(2, 0, "--agent", agent);
}

const remoteCommand = remoteArgs.map(shellQuote).join(" ");
const result = spawnSync("ssh", [host, `bash -lc ${shellQuote(remoteCommand)}`], {
  encoding: "utf8",
  maxBuffer: 20 * 1024 * 1024
});

if (result.stdout) process.stdout.write(result.stdout);
if (result.stderr) process.stderr.write(result.stderr);
if (result.error) {
  console.error(result.error.stack ?? result.error.message);
  process.exit(1);
}
process.exit(result.status ?? 1);

function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}
