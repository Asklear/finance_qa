import fs from "node:fs";
import yaml from "js-yaml";
import type { PatrolConfig, TargetConfig } from "./types.ts";

type Env = Record<string, string | undefined>;

export function loadConfig(filePath: string, env: Env = process.env): PatrolConfig {
  const raw = fs.readFileSync(filePath, "utf8");
  const loaded = yaml.load(raw) as unknown;
  const expanded = expandEnv(loaded, env) as Partial<PatrolConfig>;
  const config: PatrolConfig = {
    version: expanded.version,
    timezone: expanded.timezone,
    writeToolPatterns: expanded.writeToolPatterns,
    report: {
      minAccuracy: expanded.report?.minAccuracy ?? 0.9,
      outputDir: expanded.report?.outputDir
    },
    templates: expanded.templates,
    targets: expanded.targets ?? {}
  };
  validateConfig(config);
  return config;
}

function expandEnv(value: unknown, env: Env): unknown {
  if (typeof value === "string") {
    return value.replace(/\$\{([A-Z0-9_]+)\}/g, (match, key: string) => env[key] ?? match);
  }
  if (Array.isArray(value)) {
    return value.map((item) => expandEnv(item, env));
  }
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, expandEnv(item, env)])
    );
  }
  return value;
}

function validateConfig(config: PatrolConfig): void {
  if (!config.targets || Object.keys(config.targets).length === 0) {
    throw new Error("config must define at least one target");
  }
  for (const [name, target] of Object.entries(config.targets)) {
    validateTarget(name, target);
  }
}

function validateTarget(name: string, target: TargetConfig): void {
  if (!target.runner || typeof target.runner.type !== "string" || !target.runner.type) {
    throw new Error(`target ${name} missing runner`);
  }
  if (!target.oracle) {
    throw new Error(`target ${name} missing oracle`);
  }
  if (!target.oracle.mcpUrl) {
    throw new Error(`target ${name} missing oracle mcpUrl`);
  }
  if (!Array.isArray(target.oracle.allowedTools) || target.oracle.allowedTools.length === 0) {
    throw new Error(`target ${name} missing oracle allowedTools`);
  }
}
