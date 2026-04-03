export interface CodeSession {
  prompt(text: string): Promise<string>;
  stop(): Promise<void>;
}

export interface CodeAgent {
  startSession(interactive: boolean, prompt?: string): Promise<CodeSession>;
  resumeSession(id: string): Promise<CodeSession>;
  listSessions(): Promise<string[]>;
}

export type CodeAgentType = "gemini" | "codex" | "claude";

export interface AgentOperator {
  launchAgent(name: string, type: CodeAgentType): Promise<CodeSession>;
  switchAgent(type: CodeAgentType): Promise<CodeSession>;
  listAgents(project: string, activeOnly: boolean): Promise<string[]>;
}
