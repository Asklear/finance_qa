export const WRITE_TOOL_PATTERNS: RegExp[] = [
  /^(create|update|cancel|pause|resume)_/,
  /^upsert_/,
  /^write_/,
  /_write$/
];

interface ToolGuardTarget {
  writeToolPatterns?: string[];
  oracle?: {
    allowedTools?: string[];
  };
}

export function assertToolAllowed(target: ToolGuardTarget, toolName: string): void {
  if (isWriteTool(toolName, target.writeToolPatterns)) {
    throw new Error(`blocked write tool: ${toolName}`);
  }
  const allowed = target.oracle?.allowedTools ?? [];
  if (!allowed.includes(toolName)) {
    throw new Error(`tool is not allowed: ${toolName}`);
  }
}

export function filterAllowedTools(target: ToolGuardTarget, toolNames: string[]): string[] {
  return toolNames.filter((toolName) => {
    try {
      assertToolAllowed(target, toolName);
      return true;
    } catch {
      return false;
    }
  });
}

export function isWriteTool(toolName: string, extraPatterns: string[] = []): boolean {
  return [...WRITE_TOOL_PATTERNS, ...compilePatterns(extraPatterns)].some((pattern) => pattern.test(toolName));
}

function compilePatterns(patterns: string[]): RegExp[] {
  return patterns.map((pattern) => new RegExp(pattern));
}
