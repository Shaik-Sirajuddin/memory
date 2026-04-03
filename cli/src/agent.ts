import { CodexConnector } from "./connectors/codexConnector.js";
import {
  AgentOperator,
  CodeAgent,
  CodeAgentType,
  CodeSession,
} from "./interfaces.js";

export interface AgentOperatorOptions {
  projectRoot?: string;
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

class ClaudeSession implements CodeSession {
  async prompt(text: string): Promise<string> {
    return `Claude response to: ${text}`;
  }
  async stop(): Promise<void> {
    console.log("Claude session stopped.");
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

export class DefaultAgentOperator implements AgentOperator {
  private connectors: Map<CodeAgentType, CodeAgent> = new Map();
  private activeConnector?: CodeAgentType;
  private projectRoot: string;

  constructor(options: AgentOperatorOptions = {}) {
    this.projectRoot = options.projectRoot ?? process.cwd();
    this.connectors.set("gemini", new GeminiAgent());
    this.connectors.set("codex", new CodexConnector({ projectRoot: this.projectRoot }));
    this.connectors.set("claude", new ClaudeAgent());
  }

  private requireConnector(type: CodeAgentType): CodeAgent {
    const connector = this.connectors.get(type);
    if (!connector) {
      throw new Error(`Unknown agent type: ${type}`);
    }
    return connector;
  }

  async launchAgent(name: string, type: CodeAgentType): Promise<CodeSession> {
    const connector = this.requireConnector(type);
    this.activeConnector = type;
    return connector.startSession(true, `Hello, I am ${name}`);
  }

  async switchAgent(type: CodeAgentType): Promise<CodeSession> {
    const connector = this.requireConnector(type);
    this.activeConnector = type;
    return connector.startSession(true);
  }

  async listAgents(project: string, activeOnly: boolean): Promise<string[]> {
    if (activeOnly && this.activeConnector) {
      return [this.activeConnector];
    }
    return [...this.connectors.keys()];
  }
}
