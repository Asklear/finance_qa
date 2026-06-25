#!/usr/bin/env node
import fs from "node:fs";

const args = new Map();
for (let i = 2; i < process.argv.length; i += 2) {
  args.set(process.argv[i], process.argv[i + 1]);
}

const questionFile = args.get("--question-file");
const sessionId = args.get("--session-id");
if (!questionFile || !sessionId) {
  console.error("usage: command_agent_stub.mjs --question-file <path> --session-id <id>");
  process.exit(2);
}

const question = fs.readFileSync(questionFile, "utf8");
console.log(JSON.stringify({
  result: {
    answer: `AGENT_PATROL_OK: ${question}`,
    sessionId,
    toolCalls: [{ name: "read_status" }]
  }
}));
