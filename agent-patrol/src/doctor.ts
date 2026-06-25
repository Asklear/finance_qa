import { isWriteTool } from "./guard.ts";

interface DoctorTargetConfig {
  runner: {
    type: string;
    command?: string;
  };
  oracle: {
    type: string;
    mcpUrl?: string;
    allowedTools?: string[];
  };
}

interface DoctorConfig {
  writeToolPatterns?: string[];
  targets: Record<string, DoctorTargetConfig>;
}

export interface DoctorTargetReport {
  name: string;
  ok: boolean;
  runner: {
    type: string;
    available: boolean;
    problems: string[];
  };
  oracle: {
    type: string;
    mcpUrlPresent: boolean;
    allowedToolsPresent: string[];
  };
  blockedWriteTools: string[];
  problems: string[];
}

export interface DoctorReport {
  ok: boolean;
  targets: DoctorTargetReport[];
}

export function runDoctor(config: DoctorConfig): DoctorReport {
  const targets = Object.entries(config.targets).map(([name, target]) => inspectTarget(name, target, config.writeToolPatterns ?? []));
  return {
    ok: targets.every((target) => target.ok),
    targets
  };
}

function inspectTarget(name: string, target: DoctorTargetConfig, writeToolPatterns: string[]): DoctorTargetReport {
  const problems: string[] = [];
  const runnerProblems: string[] = [];
  const command = target.runner.command?.trim() ?? "";
  if (!command) {
    runnerProblems.push("missing_runner_command");
  }
  const allowedTools = target.oracle.allowedTools ?? [];
  const blockedWriteTools = allowedTools.filter((tool) => isWriteTool(tool, writeToolPatterns));
  if (blockedWriteTools.length > 0) {
    problems.push("oracle_allowlist_contains_write_tools");
  }
  if (!target.oracle.mcpUrl) {
    problems.push("missing_oracle_mcp_url");
  }
  problems.push(...runnerProblems);
  return {
    name,
    ok: problems.length === 0,
    runner: {
      type: target.runner.type,
      available: runnerProblems.length === 0,
      problems: runnerProblems
    },
    oracle: {
      type: target.oracle.type,
      mcpUrlPresent: Boolean(target.oracle.mcpUrl),
      allowedToolsPresent: allowedTools.filter((tool) => !isWriteTool(tool, writeToolPatterns))
    },
    blockedWriteTools,
    problems
  };
}
