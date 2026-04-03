import { promises as fs } from "node:fs";
import path from "node:path";
import crypto from "node:crypto";
import Database from "better-sqlite3";
import { resolveActiveMemory } from "../memory.js";
import { defaultGitRunner } from "../gitRunner.js";
import { loadMemConfig, resolveAgentStoreDatabasePath } from "../config.js";
import { CodeAgentType } from "../interfaces.js";

export interface ConnectorSessionRecord {
  id: string;
  project: string;
  codeagent: CodeAgentType;
  active: boolean;
  lastPrompt?: string;
  refId?: string;
  version?: string;
  timestamps: {
    created?: string;
    updated?: string;
    ended?: string;
  };
}

export class SessionStore {
  private constructor(
    private readonly stateDir: string,
    private readonly projectRoot: string,
    private readonly memoryRoot: string,
    private readonly projectKey: string,
    private readonly sqlitePath: string,
    private readonly db: InstanceType<typeof Database>,
  ) {}

  static async create(projectRoot: string): Promise<SessionStore> {
    const memoryRoot = await resolveActiveMemory(projectRoot);
    if (!memoryRoot) {
      throw new Error(
        "Memory tree not found. Run `mem init` before using connectors.",
      );
    }

    const projectKey = SessionStore.createProjectKey(projectRoot);
    const stateDir = path.join(
      memoryRoot,
      "agents",
      "connector",
      "state",
      projectKey,
    );
    await fs.mkdir(stateDir, { recursive: true });

    const config = await loadMemConfig(projectRoot);
    const sqlitePath = resolveAgentStoreDatabasePath(
      config.agentStore?.sqlitePath,
      projectRoot,
      path.join(stateDir, "store.db"),
    );

    const db = new Database(sqlitePath, {
      fileMustExist: false,
      timeout: 5000,
    });

    db.pragma("journal_mode = WAL");
    db.pragma("synchronous = NORMAL");
    db.pragma("wal_autocheckpoint = 1000");

    SessionStore.initializeSchema(db);

    return new SessionStore(
      stateDir,
      projectRoot,
      memoryRoot,
      projectKey,
      sqlitePath,
      db,
    );
  }

  close(): void {
    this.db.close();
  }

  get storagePath(): string {
    return this.stateDir;
  }

  get databasePath(): string {
    return this.sqlitePath;
  }

  forProject(): { path: string; key: string; memoryRoot: string } {
    return {
      path: this.stateDir,
      key: this.projectKey,
      memoryRoot: this.memoryRoot,
    };
  }

  private static createProjectKey(projectRoot: string): string {
    const normalized = path.resolve(projectRoot);
    const hash = crypto
      .createHash("sha256")
      .update(normalized)
      .digest("hex")
      .slice(0, 8);

    const slug = normalized
      .replace(/[:\\/]/g, "-")
      .replace(/[^a-zA-Z0-9-]/g, "-")
      .replace(/-+/g, "-")
      .replace(/^-|-$/g, "");

    const trimmed = slug.length > 32 ? slug.slice(-32) : slug;
    return `${hash}-${trimmed}`;
  }

  private static initializeSchema(db: InstanceType<typeof Database>): void {
    db.exec(`
      CREATE TABLE IF NOT EXISTS sessions (
        id TEXT PRIMARY KEY,
        project TEXT NOT NULL,
        codeagent TEXT NOT NULL,
        active INTEGER NOT NULL,
        last_prompt TEXT,
        ref_id TEXT,
        version TEXT,
        created_at TEXT NOT NULL,
        updated_at TEXT,
        ended_at TEXT
      );
    `);

    db.exec(`
      CREATE TABLE IF NOT EXISTS agent_memory (
        project_key TEXT PRIMARY KEY,
        active_session_id TEXT,
        version TEXT,
        updated_at TEXT NOT NULL
      );
    `);
  }

  private static mapRowToRecord(row: any): ConnectorSessionRecord {
    return {
      id: row.id,
      project: row.project,
      codeagent: row.codeagent as CodeAgentType,
      active: Boolean(row.active),
      lastPrompt: row.last_prompt ?? undefined,
      refId: row.ref_id ?? undefined,
      version: row.version ?? undefined,
      timestamps: {
        created: row.created_at,
        updated: row.updated_at ?? undefined,
        ended: row.ended_at ?? undefined,
      },
    };
  }

  private async readRepoVersion(): Promise<string | undefined> {
    try {
      const raw = await defaultGitRunner(
        ["rev-parse", "HEAD"],
        this.projectRoot,
      );
      return raw.trim();
    } catch {
      return undefined;
    }
  }

  listSessionIds(): string[] {
    const rows = this.db
      .prepare("SELECT id FROM sessions ORDER BY created_at DESC")
      .all();
    return rows.map((row: any) => row.id);
  }

  getSession(id: string): ConnectorSessionRecord | undefined {
    const row = this.db.prepare("SELECT * FROM sessions WHERE id = ?").get(id);
    return row ? SessionStore.mapRowToRecord(row) : undefined;
  }

  listSessions(): ConnectorSessionRecord[] {
    const rows = this.db
      .prepare("SELECT * FROM sessions ORDER BY created_at DESC")
      .all();
    return rows.map(SessionStore.mapRowToRecord);
  }

  async upsertSession(session: ConnectorSessionRecord): Promise<void> {
    const version = (await this.readRepoVersion()) ?? session.version;
    const now = new Date().toISOString();

    const record: ConnectorSessionRecord = {
      ...session,
      version,
      timestamps: {
        created: session.timestamps.created ?? now, // ✅ fix
        updated: now,
        ended: session.timestamps.ended,
      },
    };

    const stmt = this.db.prepare(`
      INSERT INTO sessions (
        id, project, codeagent, active, last_prompt, ref_id, version,
        created_at, updated_at, ended_at
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT(id) DO UPDATE SET
        project = excluded.project,
        codeagent = excluded.codeagent,
        active = excluded.active,
        last_prompt = excluded.last_prompt,
        ref_id = excluded.ref_id,
        version = excluded.version,
        updated_at = excluded.updated_at,
        ended_at = excluded.ended_at;
    `);

    stmt.run(
      record.id,
      record.project,
      record.codeagent,
      record.active ? 1 : 0,
      record.lastPrompt,
      record.refId,
      record.version,
      record.timestamps.created,
      record.timestamps.updated,
      record.timestamps.ended,
    );
  }

  async updateSession(
    id: string,
    updates: Partial<ConnectorSessionRecord>,
  ): Promise<ConnectorSessionRecord> {
    const existing = this.getSession(id);
    if (!existing) {
      throw new Error(`No connector session found with id ${id}`);
    }

    const merged: ConnectorSessionRecord = {
      ...existing,
      ...updates,
      timestamps: {
        ...existing.timestamps,
        ...updates.timestamps,
        updated: updates.timestamps?.updated ?? new Date().toISOString(),
      },
    };

    await this.upsertSession(merged);
    const refreshed = this.getSession(id);

    if (!refreshed) {
      throw new Error(`Failed to refresh connector session ${id}`);
    }

    return refreshed;
  }

  async setActiveSessionId(sessionId: string | null): Promise<void> {
    const version = await this.readRepoVersion();
    const now = new Date().toISOString();

    const stmt = this.db.prepare(`
      INSERT INTO agent_memory (project_key, active_session_id, version, updated_at)
      VALUES (?, ?, ?, ?)
      ON CONFLICT(project_key) DO UPDATE SET
        active_session_id = excluded.active_session_id,
        version = excluded.version,
        updated_at = excluded.updated_at;
    `);

    stmt.run(this.projectKey, sessionId, version, now);
  }

  getActiveSessionId(): string | null {
    const row = this.db
      .prepare(
        "SELECT active_session_id FROM agent_memory WHERE project_key = ?",
      )
      .get(this.projectKey) as { active_session_id?: string } | undefined;

    return row?.active_session_id ?? null;
  }
}
