import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { writeReport, redactSensitive } from "../src/report.ts";

test("redactSensitive removes token-like values", () => {
  const redacted = redactSensitive("Authorization: Bearer abcdefghijklmnopqrstuvwxyz1234567890 token=secret-value");
  assert.doesNotMatch(redacted, /abcdefghijklmnopqrstuvwxyz/);
  assert.doesNotMatch(redacted, /secret-value/);
});

test("writeReport writes summary and raw result files", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-report-"));
  writeReport(dir, {
    manifest: { suite: "smoke" },
    cases: [{ id: "case-1" }],
    results: [{ caseId: "case-1", answer: "ok" }],
    scores: [{ caseId: "case-1", pass: true }],
    aggregate: { total: 1, passed: 1, accuracy: 1 }
  });

  assert.equal(fs.existsSync(path.join(dir, "manifest.json")), true);
  assert.equal(fs.existsSync(path.join(dir, "cases.json")), true);
  assert.equal(fs.existsSync(path.join(dir, "summary.json")), true);
  assert.equal(fs.existsSync(path.join(dir, "raw_results.jsonl")), true);
  assert.equal(fs.existsSync(path.join(dir, "scores.json")), true);
  assert.match(fs.readFileSync(path.join(dir, "summary.md"), "utf8"), /Accuracy: 100\.00%/);
  const summary = JSON.parse(fs.readFileSync(path.join(dir, "summary.json"), "utf8"));
  assert.deepEqual(summary.aggregate, { total: 1, passed: 1, accuracy: 1 });
  assert.deepEqual(summary.failedCases, []);
});

test("writeReport includes failed case details in summary", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-patrol-report-failed-"));
  writeReport(dir, {
    manifest: { suite: "smoke" },
    cases: [{ id: "case-1" }],
    results: [{
      caseId: "case-1",
      question: "请只回答：AGENT_PATROL_OK",
      actual: {
        answer: "wrong answer",
        sessionId: "patrol-session-1"
      }
    }],
    scores: [{
      caseId: "case-1",
      pass: false,
      failures: ["missing_term:AGENT_PATROL_OK"]
    }],
    aggregate: { total: 1, passed: 0, accuracy: 0 }
  });

  const summary = fs.readFileSync(path.join(dir, "summary.md"), "utf8");
  assert.match(summary, /Failed Cases/);
  assert.match(summary, /case-1/);
  assert.match(summary, /missing_term:AGENT_PATROL_OK/);
  assert.match(summary, /请只回答：AGENT_PATROL_OK/);
  assert.match(summary, /wrong answer/);
  assert.match(summary, /patrol-session-1/);

  const summaryJson = JSON.parse(fs.readFileSync(path.join(dir, "summary.json"), "utf8"));
  assert.deepEqual(summaryJson.failedCases, [{
    caseId: "case-1",
    failures: ["missing_term:AGENT_PATROL_OK"],
    question: "请只回答：AGENT_PATROL_OK",
    answer: "wrong answer",
    sessionId: "patrol-session-1"
  }]);
});
