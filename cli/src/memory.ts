import { promises as fs } from "node:fs";
import path from "node:path";
import { GitRunner, defaultGitRunner } from "./gitRunner.js";
export const ROOT_DIR = "memory";
const FALLBACK_MEMORY_YAML = `apiVersion: memory.v1
definitions:
  - agent_folder: $${ROOT_DIR}/agents/\${AGENT_NAME}/
  - co_agent_folder: ${ROOT_DIR}/agents/*/generated
instructions:
  - agent_folder/entry/instructions are your primary instructions
  - agent_folder/entry/tasks contains the tasks you need to execute
  - agent_folder/generated is where you update the program specification after each prompt is processed
  - instructions:
      - observations need to be recorded are specified during entrypoint
  - ground your local file reading to docs/ , agent_folder , co_agent_folder
  - record your work progress in agent_folder/state/ after each prompt execution under version files with .md
`;

export interface InitMemoryResult {
  status: "initialized" | "noop";
  message: string;
  memoryPath: string;
  symlinkPath?: string;
}

export interface InitMemoryOptions {
  cwd: string;
  runGit?: GitRunner;
  templateContent?: string;
}

export async function resolveActiveMemory(cwd: string): Promise<string | null> {
  return findMemoryInParents(cwd);
}

export async function findMemoryInParents(startDir: string): Promise<string | null> {
  let cursor = path.resolve(startDir);

  while (true) {
    const candidate = path.join(cursor, ROOT_DIR);
    if (await pathExists(candidate)) {
      return candidate;
    }

    const parent = path.dirname(cursor);
    if (parent === cursor) {
      return null;
    }
    cursor = parent;
  }
}

export async function initMemory(options: InitMemoryOptions): Promise<InitMemoryResult> {
  const cwd = path.resolve(options.cwd);
  const existingMemory = await resolveActiveMemory(cwd);
  if (existingMemory) {
    const memoryYaml = path.join(existingMemory, "memory.yaml");
    if (!(await pathExists(memoryYaml))) {
      const template = options.templateContent ?? (await loadTemplateContent(cwd));
      await fs.writeFile(memoryYaml, template, "utf8");
      return {
        status: "initialized",
        message: `Found existing ${ROOT_DIR} at ${existingMemory}; created missing ${path.basename(memoryYaml)}.`,
        memoryPath: existingMemory,
      };
    }

    return {
      status: "noop",
      message: `Found existing ${ROOT_DIR} at ${existingMemory}; init skipped.`,
      memoryPath: existingMemory,
    };
  }

  const runGit = options.runGit ?? defaultGitRunner;
  const insideWorktree = await isInsideWorktree(cwd, runGit);

  let targetRoot = cwd;
  if (insideWorktree) {
    const gitCommonDirRaw = await runGit(["rev-parse", "--git-common-dir"], cwd);
    const gitCommonDir = gitCommonDirRaw.trim();
    if (!gitCommonDir) {
      throw new Error("Failed to resolve git common dir.");
    }
    targetRoot = path.dirname(path.resolve(cwd, gitCommonDir));
  }

  const memoryPath = path.join(targetRoot, ROOT_DIR);
  await fs.mkdir(memoryPath, { recursive: true });

  const memoryYaml = path.join(memoryPath, "memory.yaml");
  if (!(await pathExists(memoryYaml))) {
    const template = options.templateContent ?? (await loadTemplateContent(cwd));
    await fs.writeFile(memoryYaml, template, "utf8");
  }

  const result: InitMemoryResult = {
    status: "initialized",
    message: `Initialized ${ROOT_DIR} at ${memoryPath}.`,
    memoryPath,
  };

  if (insideWorktree && path.resolve(targetRoot) !== cwd) {
    const cwdMemoryPath = path.join(cwd, ROOT_DIR);
    if (!(await pathExists(cwdMemoryPath))) {
      const relTarget = path.relative(cwd, memoryPath) || ROOT_DIR;
      await fs.symlink(relTarget, cwdMemoryPath, "dir");
      result.symlinkPath = cwdMemoryPath;
      result.message += ` Linked ${cwdMemoryPath} -> ${relTarget}.`;
    }
  }

  return result;
}

export interface InitAgentResult {
  status: "initialized" | "noop";
  message: string;
  agentPath: string;
}

export interface FetchMemoryResult {
  status: "fetched" | "noop";
  message: string;
  memoryPath: string;
  localAgents: string[];
  passiveAgents: string[];
  activeAgents: string[];
  sparsePaths: string[];
}

export async function initAgent(
  cwd: string,
  name: string,
  runGit?: GitRunner,
): Promise<InitAgentResult> {
  if (!name.trim()) {
    throw new Error("Agent name is required. Usage: mem agent init <name>");
  }
  if (!/^[a-zA-Z0-9._-]+$/.test(name)) {
    throw new Error("Agent name contains invalid characters. Use letters, numbers, dot, underscore, or dash.");
  }

  const activeMemoryPath = await resolveActiveMemory(cwd);
  if (!activeMemoryPath) {
    throw new Error(`${ROOT_DIR} was not found in the current directory or parent directories. Run \`mem init\` first.`);
  }

  const gitRunner = runGit ?? defaultGitRunner;
  const repoRoot = await resolveRepoRoot(cwd, gitRunner);
  await ensureCodexHooks(repoRoot);

  const agentRoot = path.join(activeMemoryPath, "agents", name);
  if (await pathExists(agentRoot)) {
    return {
      status: "noop",
      message: `Agent '${name}' already exists at ${agentRoot}; init skipped.`,
      agentPath: agentRoot,
    };
  }

  const requiredDirs = [
    path.join(agentRoot, "entry", "instructions"),
    path.join(agentRoot, "entry", "tasks"),
    path.join(agentRoot, "generated"),
    path.join(agentRoot, "state"),
  ];

  for (const dirPath of requiredDirs) {
    await fs.mkdir(dirPath, { recursive: true });
  }

  return {
    status: "initialized",
    message: `Initialized agent '${name}' at ${agentRoot}.`,
    agentPath: agentRoot,
  };
}

export async function fetchMemory(
  cwd: string,
  localAgentsInput: string[] = [],
  runGit: GitRunner = defaultGitRunner,
): Promise<FetchMemoryResult> {
  const memoryPath = await resolveActiveMemory(cwd);
  if (!memoryPath) {
    throw new Error(`${ROOT_DIR} was not found in the current directory or parent directories. Run \`mem init\` first.`);
  }

  await runGit(["sparse-checkout", "init", "--cone"], memoryPath);

  const localAgents = localAgentsInput.length > 0
    ? uniqueAgents(localAgentsInput)
    : await readLocalAgents(path.join(memoryPath, "memory.local"));

  if (localAgents.length === 0) {
    return {
      status: "noop",
      message: "No local agents found; fetch skipped.",
      memoryPath,
      localAgents: [],
      passiveAgents: [],
      activeAgents: [],
      sparsePaths: [],
    };
  }

  const graphPath = path.join(memoryPath, "agent-team.md");
  const graph = await readDependencyGraph(graphPath);

  const passiveSet = new Set<string>();
  const activeSet = new Set<string>();
  for (const agent of localAgents) {
    const deps = graph.get(agent);
    if (!deps) {
      continue;
    }
    for (const passive of deps.passive) {
      passiveSet.add(passive);
    }
    for (const active of deps.active) {
      activeSet.add(active);
    }
  }

  const passiveAgents = [...passiveSet].sort();
  const activeAgents = [...activeSet].sort();
  const sparsePaths = [
    ...activeAgents.map((name) => path.posix.join("agents", name)),
    ...passiveAgents.map((name) => path.posix.join("agents", name, "generated")),
  ];

  if (sparsePaths.length > 0) {
    await runGit(["sparse-checkout", "set", ...sparsePaths], memoryPath);
  }

  return {
    status: "fetched",
    message: `Fetched agent dependencies for ${localAgents.join(", ")}.`,
    memoryPath,
    localAgents,
    passiveAgents,
    activeAgents,
    sparsePaths,
  };
}

async function isInsideWorktree(cwd: string, runGit: GitRunner): Promise<boolean> {
  try {
    const output = await runGit(["rev-parse", "--is-inside-work-tree"], cwd);
    return output.trim() === "true";
  } catch {
    return false;
  }
}

async function loadTemplateContent(cwd: string): Promise<string> {
  const existingMemoryDir = await resolveActiveMemory(cwd);
  if (!existingMemoryDir) {
    return FALLBACK_MEMORY_YAML;
  }

  const templatePath = path.join(existingMemoryDir, "memory.yaml");
  if (!(await pathExists(templatePath))) {
    return FALLBACK_MEMORY_YAML;
  }

  return fs.readFile(templatePath, "utf8");
}

async function readLocalAgents(memoryLocalPath: string): Promise<string[]> {
  if (!(await pathExists(memoryLocalPath))) {
    return [];
  }

  const content = await fs.readFile(memoryLocalPath, "utf8");
  const lines = content.split(/\r?\n/);

  const inlineMatch = content.match(/^\s*agents\s*:[ \t]*\[([^\]]*)\]/m);
  if (inlineMatch) {
    return uniqueAgents(splitNames(inlineMatch[1]));
  }

  const csvMatch = content.match(/^\s*agents\s*:[ \t]*([a-zA-Z0-9._-][^#\n]*)$/m);
  if (csvMatch) {
    return uniqueAgents(splitNames(csvMatch[1]));
  }

  const agents: string[] = [];
  let inAgentsBlock = false;
  for (const line of lines) {
    if (/^\s*agents\s*:\s*$/.test(line)) {
      inAgentsBlock = true;
      continue;
    }
    if (inAgentsBlock) {
      const bullet = line.match(/^\s*-\s*([a-zA-Z0-9._-]+)\s*$/);
      if (bullet) {
        agents.push(bullet[1]);
        continue;
      }
      if (/^\s*[a-zA-Z0-9._-]+\s*:/.test(line) || /^\S/.test(line)) {
        break;
      }
    }
  }

  return uniqueAgents(agents);
}

type DependencyEntry = { passive: string[]; active: string[] };

async function readDependencyGraph(graphPath: string): Promise<Map<string, DependencyEntry>> {
  if (!(await pathExists(graphPath))) {
    return new Map();
  }

  const content = await fs.readFile(graphPath, "utf8");
  const graph = new Map<string, DependencyEntry>();

  for (const line of content.split(/\r?\n/)) {
    const mdRow = parseMarkdownGraphRow(line);
    if (mdRow) {
      graph.set(mdRow.name, { passive: mdRow.passive, active: mdRow.active });
      continue;
    }

    const scoped = parseScopedGraphLine(line);
    if (scoped) {
      graph.set(scoped.name, { passive: scoped.passive, active: scoped.active });
    }
  }

  return graph;
}

function parseMarkdownGraphRow(line: string): { name: string; passive: string[]; active: string[] } | null {
  if (!line.trim().startsWith("|")) {
    return null;
  }

  const columns = line.split("|").map((part) => part.trim()).filter((part) => part.length > 0);
  if (columns.length < 3) {
    return null;
  }
  if (columns[0].toLowerCase() === "agent") {
    return null;
  }
  if (/^-+$/.test(columns[0])) {
    return null;
  }

  const [name, passiveRaw, activeRaw] = columns;
  if (!isAgentName(name)) {
    return null;
  }

  return {
    name,
    passive: uniqueAgents(splitNames(passiveRaw)),
    active: uniqueAgents(splitNames(activeRaw)),
  };
}

function parseScopedGraphLine(line: string): { name: string; passive: string[]; active: string[] } | null {
  const match = line.match(/^\s*([a-zA-Z0-9._-]+)\s*:\s*(.+)$/);
  if (!match) {
    return null;
  }

  const name = match[1];
  const rest = match[2];
  const passivePart = extractDependencySegment(rest, "passive");
  const activePart = extractDependencySegment(rest, "active");

  return {
    name,
    passive: uniqueAgents(splitNames(passivePart)),
    active: uniqueAgents(splitNames(activePart)),
  };
}

function extractDependencySegment(line: string, label: "passive" | "active"): string {
  const bracketed = line.match(new RegExp(`${label}\\s*(?:_agents)?\\s*[:=]\\s*\\[([^\\]]*)\\]`, "i"));
  if (bracketed) {
    return bracketed[1];
  }

  const plain = line.match(new RegExp(`${label}\\s*(?:_agents)?\\s*[:=]\\s*([^;|]+)`, "i"));
  return plain ? plain[1] : "";
}

function splitNames(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter((item) => isAgentName(item));
}

function uniqueAgents(input: string[]): string[] {
  const seen = new Set<string>();
  for (const name of input.map((item) => item.trim()).filter((item) => isAgentName(item))) {
    seen.add(name);
  }
  return [...seen].sort();
}

function isAgentName(value: string): boolean {
  return /^[a-zA-Z0-9._-]+$/.test(value);
}

async function pathExists(targetPath: string): Promise<boolean> {
  try {
    await fs.lstat(targetPath);
    return true;
  } catch {
    return false;
  }
}

const REQUIRED_CODEX_HOOKS: CodexHookEntry[] = [
  { event: "UserPromptSubmit", command: "mem", args: ["agent", "pre-execute"] },
  { event: "PostToolUse", command: "mem", args: ["agent", "post-execute"] },
  { event: "Stop", command: "mem", args: ["agent", "finalize"] },
];

interface CodexHookEntry {
  event: string;
  command: string;
  args?: string[];
  env?: Record<string, string>;
  timeout_ms?: number;
  enabled?: boolean;
}

async function ensureCodexHooks(repoRoot: string): Promise<void> {
  const hooksDir = path.join(repoRoot, ".codex");
  await fs.mkdir(hooksDir, { recursive: true });
  const hooksFile = path.join(hooksDir, "hooks.json");

  let currentHooks: CodexHookEntry[] = [];
  if (await pathExists(hooksFile)) {
    try {
      const raw = await fs.readFile(hooksFile, "utf8");
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed?.hooks)) {
        currentHooks = parsed.hooks;
      }
    } catch {
      currentHooks = [];
    }
  }

  let mutated = false;
  for (const required of REQUIRED_CODEX_HOOKS) {
    const existing = currentHooks.find((entry) => entry.event === required.event && entry.command === required.command);
    if (existing) {
      if (!arraysEqual(existing.args, required.args) || existing.enabled !== true) {
        existing.args = required.args;
        existing.enabled = true;
        mutated = true;
      }
      continue;
    }
    currentHooks.push({ ...required, enabled: true });
    mutated = true;
  }

  if (mutated) {
    const payload = { hooks: currentHooks };
    await fs.writeFile(hooksFile, JSON.stringify(payload, null, 2) + "\n", "utf8");
  }
}

function arraysEqual(a?: string[], b?: string[]): boolean {
  if (!a && !b) {
    return true;
  }
  if (!a || !b || a.length !== b.length) {
    return false;
  }
  return a.every((value, index) => value === b[index]);
}

async function resolveRepoRoot(cwd: string, runGit: GitRunner): Promise<string> {
  try {
    const output = await runGit(["rev-parse", "--show-toplevel"], cwd);
    const trimmed = output.trim();
    if (trimmed) {
      return trimmed;
    }
  } catch {
    // ignore
  }
  return cwd;
}
