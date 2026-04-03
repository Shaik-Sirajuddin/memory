import path from "node:path";
import { cosmiconfig } from "cosmiconfig";

export interface AgentStoreConfig {
  sqlitePath?: string;
}

export interface MemConfig {
  agentStore?: AgentStoreConfig;
}

const explorer = cosmiconfig("mem");

export async function loadMemConfig(searchFrom: string): Promise<MemConfig> {
  const result = await explorer.search(searchFrom);
  return result?.config ?? {};
}

export function resolveAgentStoreDatabasePath(
  configuredPath: string | undefined,
  projectRoot: string,
  fallback: string,
): string {
  if (!configuredPath) {
    return fallback;
  }

  return path.isAbsolute(configuredPath)
    ? configuredPath
    : path.join(projectRoot, configuredPath);
}
