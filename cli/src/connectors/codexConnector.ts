import { execFile } from "node:child_process";
import crypto from "node:crypto";
import { promisify } from "node:util";
import { CodeAgent, CodeSession } from "../interfaces.js";
import { SessionStore, ConnectorSessionRecord } from "./sessionStore.js";

const execFileAsync = promisify(execFile);

export interface CodexConnectorOptions {
  projectRoot?: string;
}

export class CodexConnector implements CodeAgent {
  private readonly projectRoot: string;
  private storePromise?: Promise<SessionStore>;

  constructor(options: CodexConnectorOptions = {}) {
    this.projectRoot = options.projectRoot ?? process.cwd();
  }

  private async ensureStore(): Promise<SessionStore> {
    if (!this.storePromise) {
      this.storePromise = SessionStore.create(this.projectRoot);
    }
    return this.storePromise;
  }

  private async runCodexExec(prompt: string): Promise<string> {
    try {
      const { stdout } = await execFileAsync("codex", ["exec", prompt, "--json", "--sandbox", "read-only"], {
        cwd: this.projectRoot,
        env: process.env,
      });
      return stdout.trim() || `Codex returned an empty response for '${prompt}'.`;
    } catch (error) {
      const err = error as NodeJS.ErrnoException;
      if (err.code === "ENOENT") {
        console.warn("Codex CLI not found in PATH, using fallback response.");
        return `Simulated Codex reply for '${prompt}' (codex binary missing).`;
      }
      console.error("Codex exec failed:", err.message);
      return `Codex CLI error: ${err.message}`;
    }
  }

  private async createSessionRecord(prompt?: string): Promise<ConnectorSessionRecord> {
    return {
      id: crypto.randomUUID(),
      project: this.projectRoot,
      codeagent: "codex",
      active: true,
      lastPrompt: prompt,
      refId: undefined,
      timestamps: {
        created: new Date().toISOString(),
      },
    };
  }

  private async updateActiveSession(sessionId: string): Promise<void> {
    const store = await this.ensureStore();
    await store.setActiveSessionId(sessionId);
  }

  private async sendPrompt(sessionId: string, text: string): Promise<string> {
    const response = await this.runCodexExec(text);
    const store = await this.ensureStore();
    await store.updateSession(sessionId, {
      lastPrompt: text,
      timestamps: {
        updated: new Date().toISOString(),
      },
      refId: sessionId,
    });
    return response;
  }

  private async endSession(sessionId: string): Promise<void> {
    const store = await this.ensureStore();
    await store.updateSession(sessionId, {
      active: false,
      timestamps: {
        ended: new Date().toISOString(),
      },
    });
    const activeId = await store.getActiveSessionId();
    if (activeId === sessionId) {
      await store.setActiveSessionId(null);
    }
  }

  async startSession(interactive: boolean, prompt?: string): Promise<CodeSession> {
    const session = await this.createSessionRecord(prompt);
    const store = await this.ensureStore();
    await store.upsertSession(session);
    await this.updateActiveSession(session.id);
    return new CodexSession(this, session.id);
  }

  async resumeSession(id: string): Promise<CodeSession> {
    const store = await this.ensureStore();
    const metadata = await store.getSession(id);
    if (!metadata) {
      throw new Error(`Codex session '${id}' not found.`);
    }
    await store.updateSession(id, {
      active: true,
      timestamps: {
        updated: new Date().toISOString(),
      },
    });
    await this.updateActiveSession(id);
    return new CodexSession(this, id);
  }

  async listSessions(): Promise<string[]> {
    const store = await this.ensureStore();
    const sessions = await store.listSessions();
    return sessions.map((session) => session.id);
  }

  async sendPromptToSession(sessionId: string, text: string): Promise<string> {
    return this.sendPrompt(sessionId, text);
  }

  async closeSession(sessionId: string): Promise<void> {
    await this.endSession(sessionId);
  }
}

class CodexSession implements CodeSession {
  constructor(private readonly connector: CodexConnector, private readonly id: string) {}

  async prompt(text: string): Promise<string> {
    return this.connector.sendPromptToSession(this.id, text);
  }

  async stop(): Promise<void> {
    await this.connector.closeSession(this.id);
  }
}
