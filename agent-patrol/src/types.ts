export type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue };

export interface OracleConfig {
  type: string;
  mcpUrl?: string;
  bearerTokenEnv?: string;
  allowedTools: string[];
}

export interface RunnerConfig {
  type: string;
  command?: string;
  agent?: string;
  profile?: string;
  userId?: string;
  isolatedSessionPrefix?: string;
  requireSessionIsolation?: boolean;
  disableDelivery?: boolean;
}

export interface TargetConfig {
  kind?: string;
  runner: RunnerConfig;
  oracle: OracleConfig;
  suites?: Record<string, SuiteConfig>;
}

export interface SuiteConfig {
  caseCount?: number;
  templates?: string[];
}

export interface CaseTemplateConfig {
  questions?: string[];
  question?: string;
  fallbackQuestion?: string;
  scoring?: Record<string, unknown>;
}

export interface PatrolConfig {
  version?: number;
  timezone?: string;
  writeToolPatterns?: string[];
  report: {
    minAccuracy: number;
    outputDir?: string;
  };
  templates?: Record<string, CaseTemplateConfig>;
  targets: Record<string, TargetConfig>;
}

export interface PatrolCase {
  id: string;
  target: string;
  template: string;
  question: string;
  actualRunner: string;
  oracle: string;
  scoring: Record<string, unknown>;
}

export interface AgentEnvelope {
  source: "agent" | "direct_mcp" | string;
  answer: string;
  sessionId?: string;
  sessionKey?: string;
  toolCalls?: Array<{ name?: string; [key: string]: unknown }>;
  raw?: unknown;
}
