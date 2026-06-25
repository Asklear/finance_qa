import type { CaseTemplateConfig, PatrolCase, SuiteConfig } from "./types.ts";

interface GenerateOptions {
  suite: string;
  seed: string;
  anchors?: Record<string, { customers?: Array<{ name?: string }> }>;
}

interface CaseTargetConfig {
  runner: { type: string };
  oracle: { type: string };
  suites?: Record<string, SuiteConfig>;
}

export function generateCases(
  config: { targets: Record<string, CaseTargetConfig>; templates?: Record<string, CaseTemplateConfig> },
  options: GenerateOptions
): PatrolCase[] {
  const cases: PatrolCase[] = [];
  for (const [targetName, target] of Object.entries(config.targets)) {
    cases.push(...generateTargetCases(targetName, target, config.templates ?? {}, options));
  }
  return cases;
}

function generateTargetCases(
  targetName: string,
  target: CaseTargetConfig,
  templateCatalog: Record<string, CaseTemplateConfig>,
  options: GenerateOptions
): PatrolCase[] {
  const suite = target.suites?.[options.suite];
  const templates = suite?.templates ?? [];
  const selected = templates.slice(0, suite?.caseCount ?? templates.length);
  return selected.map((templateName, index) => {
    const def = templateCatalog[templateName];
    if (!def) {
      throw new Error(`unknown case template: ${templateName}`);
    }
    const anchor = options.anchors?.[targetName] ?? {};
    const question = renderQuestion(def, anchor, `${options.seed}:${targetName}:${templateName}`);
    return {
      id: stableId(targetName, templateName, index),
      target: targetName,
      template: templateName,
      question,
      actualRunner: target.runner.type,
      oracle: target.oracle.type,
      scoring: def.scoring ?? {}
    };
  });
}

function renderQuestion(def: CaseTemplateConfig, anchor: { customers?: Array<{ name?: string }> }, seed: string): string {
  if (def.question) {
    const rendered = applyTemplate(def.question, anchor);
    if (rendered) return rendered;
  }
  if (def.questions && def.questions.length > 0) {
    return applyTemplate(choose(def.questions, seed), anchor) || choose(def.questions, seed);
  }
  if (def.fallbackQuestion) {
    return def.fallbackQuestion;
  }
  throw new Error("template has no question variants");
}

function applyTemplate(question: string, anchor: { customers?: Array<{ name?: string }> }): string {
  const customerName = anchor.customers?.[0]?.name;
  if (!customerName && question.includes("{{customer.name}}")) {
    return "";
  }
  return question.replace(/\{\{customer\.name\}\}/g, customerName ?? "");
}

function choose(items: string[], seed: string): string {
  if (items.length === 0) {
    throw new Error("template has no question variants");
  }
  return items[hash(seed) % items.length]!;
}

function stableId(target: string, template: string, index: number): string {
  return `${target}_${template}_${String(index + 1).padStart(3, "0")}`;
}

function hash(value: string): number {
  let result = 0;
  for (const char of value) {
    result = (result * 31 + char.charCodeAt(0)) >>> 0;
  }
  return result;
}
