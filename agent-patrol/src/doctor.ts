import { isWriteTool } from "./guard.ts";

interface DoctorTargetConfig {
  runner: {
    type: string;
    command?: string;
  };
  questionGenerator?: {
    type: string;
    command?: string;
  };
  oracle: {
    type: string;
    mcpUrl?: string;
    allowedTools?: string[];
  };
  goldenReference?: {
    type: string;
    command?: string;
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
    problems: string[];
  };
  questionGenerator?: {
    type: string;
    available: boolean;
    problems: string[];
  };
  goldenReference?: {
    type?: string;
    available: boolean;
    problems: string[];
  };
  blockedWriteTools: string[];
  problems: string[];
}

export interface DoctorReport {
  ok: boolean;
  targets: DoctorTargetReport[];
}

export interface DoctorOptions {
  requireGoldenReference?: boolean;
  requireResolvedEnv?: boolean;
}

export function runDoctor(config: DoctorConfig, options: DoctorOptions = {}): DoctorReport {
  const targets = Object.entries(config.targets).map(([name, target]) => inspectTarget(name, target, config.writeToolPatterns ?? [], options));
  return {
    ok: targets.every((target) => target.ok),
    targets
  };
}

function inspectTarget(name: string, target: DoctorTargetConfig, writeToolPatterns: string[], options: DoctorOptions): DoctorTargetReport {
  const problems: string[] = [];
  const runnerProblems: string[] = [];
  const command = target.runner.command?.trim() ?? "";
  if (!command) {
    runnerProblems.push("missing_runner_command");
  }
  if (options.requireResolvedEnv && hasUnresolvedPlaceholder(command)) {
    runnerProblems.push("unresolved_runner_command");
  }
  const allowedTools = target.oracle.allowedTools ?? [];
  const oracleProblems: string[] = [];
  const blockedWriteTools = allowedTools.filter((tool) => isWriteTool(tool, writeToolPatterns));
  if (blockedWriteTools.length > 0) {
    problems.push("oracle_allowlist_contains_write_tools");
  }
  if (!target.oracle.mcpUrl) {
    oracleProblems.push("missing_oracle_mcp_url");
  }
  if (options.requireResolvedEnv && hasUnresolvedPlaceholder(target.oracle.mcpUrl ?? "")) {
    oracleProblems.push("unresolved_oracle_mcp_url");
  }
  const questionGeneratorProblems: string[] = [];
  if (target.questionGenerator) {
    if (target.questionGenerator.type !== "command") {
      questionGeneratorProblems.push("unsupported_question_generator_type");
    }
    if (!target.questionGenerator.command?.trim()) {
      questionGeneratorProblems.push("missing_question_generator_command");
    }
    if (options.requireResolvedEnv && hasUnresolvedPlaceholder(target.questionGenerator.command ?? "")) {
      questionGeneratorProblems.push("unresolved_question_generator_command");
    }
    problems.push(...questionGeneratorProblems);
  }
  const goldenReferenceProblems: string[] = [];
  if (options.requireGoldenReference && !target.goldenReference) {
    goldenReferenceProblems.push("missing_golden_reference");
  }
  if (target.goldenReference) {
    if (target.goldenReference.type !== "command") {
      goldenReferenceProblems.push("unsupported_golden_reference_type");
    }
    if (!target.goldenReference.command?.trim()) {
      goldenReferenceProblems.push("missing_golden_reference_command");
    }
    if (options.requireResolvedEnv && hasUnresolvedPlaceholder(target.goldenReference.command ?? "")) {
      goldenReferenceProblems.push("unresolved_golden_reference_command");
    }
  }
  problems.push(...runnerProblems);
  problems.push(...oracleProblems);
  problems.push(...goldenReferenceProblems);
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
      allowedToolsPresent: allowedTools.filter((tool) => !isWriteTool(tool, writeToolPatterns)),
      problems: oracleProblems
    },
    questionGenerator: target.questionGenerator ? {
      type: target.questionGenerator.type,
      available: questionGeneratorProblems.length === 0,
      problems: questionGeneratorProblems
    } : undefined,
    goldenReference: target.goldenReference || options.requireGoldenReference ? {
      type: target.goldenReference?.type,
      available: goldenReferenceProblems.length === 0,
      problems: goldenReferenceProblems
    } : undefined,
    blockedWriteTools,
    problems
  };
}

function hasUnresolvedPlaceholder(value: string): boolean {
  return /\$\{[A-Z0-9_]+\}/.test(value);
}
