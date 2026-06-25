import test from "node:test";
import assert from "node:assert/strict";
import { assertToolAllowed, filterAllowedTools } from "../src/guard.ts";

test("assertToolAllowed allows configured read tools", () => {
  const target = { oracle: { allowedTools: ["finance-query", "get_all_deals"] } };
  assert.doesNotThrow(() => assertToolAllowed(target, "finance-query"));
  assert.doesNotThrow(() => assertToolAllowed(target, "get_all_deals"));
});

test("assertToolAllowed blocks write tools even when listed", () => {
  const target = { oracle: { allowedTools: ["create_scheduled_job"] } };
  assert.throws(() => assertToolAllowed(target, "create_scheduled_job"), /write tool/i);
});

test("filterAllowedTools returns only safe configured tools", () => {
  const target = { oracle: { allowedTools: ["finance-query", "custom-read", "missing"] } };
  assert.deepEqual(filterAllowedTools(target, ["finance-query", "custom-read", "other"]), ["finance-query", "custom-read"]);
});

test("assertToolAllowed supports preset-provided write tool patterns", () => {
  const target = {
    writeToolPatterns: ["^finance-(sync|upload)$"],
    oracle: { allowedTools: ["finance-sync", "finance-query"] }
  };
  assert.throws(() => assertToolAllowed(target, "finance-sync"), /write tool/i);
  assert.doesNotThrow(() => assertToolAllowed(target, "finance-query"));
});

test("assertToolAllowed does not hardcode finance-specific write tools in core", () => {
  const target = { oracle: { allowedTools: ["finance-sync"] } };
  assert.doesNotThrow(() => assertToolAllowed(target, "finance-sync"));
});
