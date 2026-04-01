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

export class GeminiAgent implements CodeAgent {
  async startSession(
    interactive: boolean,
    prompt?: string,
  ): Promise<CodeSession> {
    console.log(
      `Starting Gemini session (interactive: ${interactive}, prompt: ${prompt})`,
    );
    return new GeminiSession();
  }
  async resumeSession(id: string): Promise<CodeSession> {
    console.log(`Resuming Gemini session ${id}`);
    return new GeminiSession();
  }
  async listSessions(): Promise<string[]> {
    return ["gemini-session-1"];
  }
}

class GeminiSession implements CodeSession {
  async prompt(text: string): Promise<string> {
    return `Gemini response to: ${text}`;
  }
  async stop(): Promise<void> {
    console.log("Gemini session stopped.");
  }
}

export class CodexAgent implements CodeAgent {
  async startSession(
    interactive: boolean,
    prompt?: string,
  ): Promise<CodeSession> {
    console.log(
      `Starting Codex session (interactive: ${interactive}, prompt: ${prompt})`,
    );
    return new CodexSession();
  }
  async resumeSession(id: string): Promise<CodeSession> {
    console.log(`Resuming Codex session ${id}`);
    return new CodexSession();
  }
  async listSessions(): Promise<string[]> {
    return ["codex-session-1"];
  }
}

class CodexSession implements CodeSession {
  async prompt(text: string): Promise<string> {
    return `Codex response to: ${text}`;
  }
  async stop(): Promise<void> {
    console.log("Codex session stopped.");
  }
}

export class ClaudeAgent implements CodeAgent {
  async startSession(
    interactive: boolean,
    prompt?: string,
  ): Promise<CodeSession> {
    console.log(
      `Starting Claude session (interactive: ${interactive}, prompt: ${prompt})`,
    );
    return new ClaudeSession();
  }
  async resumeSession(id: string): Promise<CodeSession> {
    console.log(`Resuming Claude session ${id}`);
    return new ClaudeSession();
  }
  async listSessions(): Promise<string[]> {
    return ["claude-session-1"];
  }
}

class ClaudeSession implements CodeSession {
  async prompt(text: string): Promise<string> {
    return `Claude response to: ${text}`;
  }
  async stop(): Promise<void> {
    console.log("Claude session stopped.");
  }
}

export class DefaultAgentOperator implements AgentOperator {
  private agents: Map<CodeAgentType, CodeAgent> = new Map();

  constructor() {
    this.agents.set("gemini", new GeminiAgent());
    this.agents.set("codex", new CodexAgent());
    this.agents.set("claude", new ClaudeAgent());
  }

  async launchAgent(name: string, type: CodeAgentType): Promise<CodeSession> {
    const agent = this.agents.get(type);
    if (!agent) throw new Error(`Unknown agent type: ${type}`);
    return agent.startSession(true, `Hello, I am ${name}`);
  }

  async switchAgent(type: CodeAgentType): Promise<CodeSession> {
    const agent = this.agents.get(type);
    if (!agent) throw new Error(`Unknown agent type: ${type}`);
    return agent.startSession(true);
  }

  async listAgents(project: string, activeOnly: boolean): Promise<string[]> {
    return Array.from(this.agents.keys());
  }
}
