import { generateCases } from "./cases.ts";
import { runDoctor } from "./doctor.ts";
import { writeReport } from "./report.ts";
import { executeReadonlyReference, type ExecuteReferenceInput } from "./reference.ts";
import { runCommandAgent } from "./runners/command_runner.ts";
import { scoreCase, type CaseScore } from "./scorer.ts";
import type { AgentEnvelope, CaseEvidence, PatrolCase, PatrolConfig, ReferenceEnvelope, TargetConfig } from "./types.ts";

export interface ExecuteAgentInput {
  patrolCase: PatrolCase;
  target: TargetConfig;
  sessionId: string;
}

export type ExecuteAgent = (input: ExecuteAgentInput) => Promise<AgentEnvelope>;
export type ExecuteReference = (input: ExecuteReferenceInput) => Promise<ReferenceEnvelope | undefined>;

export interface RunSuiteOptions {
  suite: string;
  seed: string;
  outDir: string;
  executeAgent?: ExecuteAgent;
  executeReference?: ExecuteReference;
}

export interface RunSuiteResult {
  cases: PatrolCase[];
  results: Array<{
    caseId: string;
    target: string;
    runner: string;
    question: string;
    durationMs: number;
    actual: AgentEnvelope;
  }>;
  scores: CaseScore[];
  evidence: CaseEvidence[];
  aggregate: {
    total: number;
    passed: number;
    accuracy: number;
    durationMs: number;
    thresholdPassed: boolean;
  };
}

export async function runSuite(config: PatrolConfig, options: RunSuiteOptions): Promise<RunSuiteResult> {
  const doctor = runDoctor(config);
  if (!doctor.ok) {
    throw new Error("patrol doctor failed; fix config before run");
  }
  const cases = generateCases(config, { suite: options.suite, seed: options.seed });
  const executeAgent = options.executeAgent ?? defaultExecuteAgent;
  const executeReference = options.executeReference ?? executeReadonlyReference;
  const results: RunSuiteResult["results"] = [];
  const scores: CaseScore[] = [];
  const evidence: CaseEvidence[] = [];
  const startedAt = Date.now();

  for (const patrolCase of cases) {
    const target = config.targets[patrolCase.target];
    if (!target) {
      throw new Error(`missing target for case: ${patrolCase.target}`);
    }
    const sessionId = makeSessionId(target, patrolCase, options.seed);
    const caseStartedAt = Date.now();
    const actual = await executeAgent({ patrolCase, target, sessionId });
    const durationMs = Date.now() - caseStartedAt;
    results.push({
      caseId: patrolCase.id,
      target: patrolCase.target,
      runner: target.runner.type,
      question: patrolCase.question,
      durationMs,
      actual
    });
    const reference = await executeReference({ patrolCase, target });
    const score = scoreCase({
      id: patrolCase.id,
      expected: patrolCase.scoring,
      actual,
      reference
    });
    scores.push(score);
    evidence.push({
      caseId: patrolCase.id,
      target: patrolCase.target,
      runner: target.runner.type,
      question: patrolCase.question,
      expected: patrolCase.scoring,
      actual,
      reference,
      score
    });
  }

  const aggregate = aggregateScores(scores, config.report.minAccuracy, Date.now() - startedAt);
  writeReport(options.outDir, {
    manifest: {
      suite: options.suite,
      seed: options.seed,
      generatedAt: new Date().toISOString()
    },
    cases,
    results,
    scores,
    evidence,
    aggregate
  });
  return { cases, results, scores, evidence, aggregate };
}

async function defaultExecuteAgent(input: ExecuteAgentInput): Promise<AgentEnvelope> {
  if (!input.target.runner.command) {
    throw new Error(`target ${input.patrolCase.target} missing runner command`);
  }
  return runCommandAgent({
    commandTemplate: input.target.runner.command,
    question: input.patrolCase.question,
    sessionId: input.sessionId,
    requireSessionIsolation: input.target.runner.requireSessionIsolation,
    timeoutMs: input.target.runner.timeoutMs,
    values: {
      agent: input.target.runner.agent ?? "",
      profile: input.target.runner.profile ?? "",
      userId: input.target.runner.userId ?? ""
    }
  });
}

function makeSessionId(target: TargetConfig, patrolCase: PatrolCase, seed: string): string {
  const prefix = target.runner.isolatedSessionPrefix ?? "patrol";
  return `${prefix}-${seed}-${patrolCase.id}`;
}

function aggregateScores(scores: CaseScore[], minAccuracy: number, durationMs: number): RunSuiteResult["aggregate"] {
  const total = scores.length;
  const passed = scores.filter((score) => score.pass).length;
  const accuracy = total === 0 ? 0 : passed / total;
  return {
    total,
    passed,
    accuracy,
    durationMs,
    thresholdPassed: accuracy >= minAccuracy
  };
}
