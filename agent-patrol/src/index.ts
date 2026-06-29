#!/usr/bin/env -S tsx
import fs from "node:fs";
import path from "node:path";
import { loadConfig } from "./config.ts";
import { generateCases } from "./cases.ts";
import { runDoctor } from "./doctor.ts";
import { runSuite } from "./run.ts";

export interface ParsedCli {
  command: string;
  flags: Record<string, string | boolean>;
}

export function parseCliArgs(argv: string[]): ParsedCli {
  const [command = "help", ...rest] = argv;
  const flags: Record<string, string | boolean> = {};
  for (let i = 0; i < rest.length; i += 1) {
    const item = rest[i]!;
    if (!item.startsWith("--")) continue;
    const key = item.slice(2);
    const next = rest[i + 1];
    if (!next || next.startsWith("--")) {
      flags[key] = true;
    } else {
      flags[key] = next;
      i += 1;
    }
  }
  return { command, flags };
}

export async function main(argv = process.argv.slice(2)): Promise<number> {
  const { command, flags } = parseCliArgs(argv);
  if (command === "help" || command === "--help") {
    console.log("Usage: agent-patrol <generate|doctor|run> --config patrol.yaml");
    return 0;
  }
  if (command === "generate") {
    const configPath = stringFlag(flags.config);
    if (!configPath) throw new Error("--config is required");
    const suite = stringFlag(flags.suite) ?? "smoke";
    const seed = stringFlag(flags.seed) ?? new Date().toISOString().slice(0, 10);
    const config = loadConfig(configPath);
    const cases = generateCases(config, { suite, seed });
    const outDir = stringFlag(flags.out);
    if (outDir) {
      fs.mkdirSync(outDir, { recursive: true });
      fs.writeFileSync(path.join(outDir, "cases.json"), JSON.stringify(cases, null, 2) + "\n", "utf8");
    } else {
      console.log(JSON.stringify({ cases }, null, 2));
    }
    return 0;
  }
  if (command === "doctor") {
    const configPath = stringFlag(flags.config);
    if (!configPath) throw new Error("--config is required");
    const report = runDoctor(loadConfig(configPath), {
      requireGoldenReference: Boolean(flags["require-golden-reference"]),
      requireResolvedEnv: Boolean(flags["require-resolved-env"])
    });
    console.log(JSON.stringify(report, null, 2));
    return report.ok ? 0 : 2;
  }
  if (command === "run") {
    const configPath = stringFlag(flags.config);
    if (!configPath) throw new Error("--config is required");
    const suite = stringFlag(flags.suite) ?? "smoke";
    const seed = stringFlag(flags.seed) ?? new Date().toISOString().slice(0, 10);
    const config = loadConfig(configPath);
    const outDir = stringFlag(flags.out) ?? config.report.outputDir ?? path.join("tmp", "patrol");
    const result = await runSuite(config, { suite, seed, outDir });
    console.log(JSON.stringify(result.aggregate, null, 2));
    return result.aggregate.thresholdPassed ? 0 : 1;
  }
  throw new Error(`command not implemented yet: ${command}`);
}

function stringFlag(value: string | boolean | undefined): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().then((code) => {
    process.exitCode = code;
  }).catch((err: unknown) => {
    console.error(err instanceof Error ? err.stack ?? err.message : String(err));
    process.exit(2);
  });
}
