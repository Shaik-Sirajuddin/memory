import { execFile } from "node:child_process";
import { promises as fs } from "node:fs";
import path from "node:path";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

const FALLBACK_MEMORY_YAML = `apiVersion: memory.v1
definitions:
  - agent_folder: .memory/agents/\${AGENT_NAME}/
  - co_agent_folder: .memory/agents/*/generated
instructions:
  - agent_folder/entry/instructions are your primary instructions
  - agent_folder/entry/tasks contains the tasks you need to execute
  - agent_folder/generated is where you update the program specification after each prompt is processed
  - instructions:
      - observations need to be recorded are specified during entrypoint
  - ground your local file reading to docs/ , agent_folder , co_agent_folder
  - record your work progress in agent_folder/state/ after each prompt execution under version files with .md
`;

export type GitRunner = (args: string[], cwd: string) => Promise<string>;

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

export async function findMemoryInParents(startDir: string): Promise<string | null> {
  let cursor = path.resolve(startDir);

  while (true) {
    const candidate = path.join(cursor, ".memory");
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
  const existingMemory = await findMemoryInParents(cwd);
  if (existingMemory) {
    return {
      status: "noop",
      message: `Found existing .memory at ${existingMemory}; init skipped.`,
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

  const memoryPath = path.join(targetRoot, ".memory");
  await fs.mkdir(memoryPath, { recursive: true });

  const memoryYaml = path.join(memoryPath, "memory.yaml");
  if (!(await pathExists(memoryYaml))) {
    const template = options.templateContent ?? (await loadTemplateContent(cwd));
    await fs.writeFile(memoryYaml, template, "utf8");
  }

  const result: InitMemoryResult = {
    status: "initialized",
    message: `Initialized .memory at ${memoryPath}.`,
    memoryPath,
  };

  if (insideWorktree && path.resolve(targetRoot) !== cwd) {
    const cwdMemoryPath = path.join(cwd, ".memory");
    if (!(await pathExists(cwdMemoryPath))) {
      const relTarget = path.relative(cwd, memoryPath) || ".memory";
      await fs.symlink(relTarget, cwdMemoryPath, "dir");
      result.symlinkPath = cwdMemoryPath;
      result.message += ` Linked ${cwdMemoryPath} -> ${relTarget}.`;
    }
  }

  return result;
}

export interface InitAgentResult {
  status: "initialized";
  message: string;
  agentPath: string;
}

export async function initAgent(cwd: string, name: string): Promise<InitAgentResult> {
  if (!name.trim()) {
    throw new Error("Agent name is required. Usage: mem agent init <name>");
  }
  if (!/^[a-zA-Z0-9._-]+$/.test(name)) {
    throw new Error("Agent name contains invalid characters. Use letters, numbers, dot, underscore, or dash.");
  }

  const cwdMemoryPath = path.join(path.resolve(cwd), ".memory");
  if (!(await pathExists(cwdMemoryPath))) {
    throw new Error(".memory was not found in the current directory. Run `mem init` first.");
  }

  const agentRoot = path.join(cwdMemoryPath, "agents", name);
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

export function defaultGitRunner(args: string[], cwd: string): Promise<string> {
  return execFileAsync("git", args, { cwd }).then((res) => res.stdout);
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
  const existingMemoryDir = await findMemoryInParents(cwd);
  if (!existingMemoryDir) {
    return FALLBACK_MEMORY_YAML;
  }

  const templatePath = path.join(existingMemoryDir, "memory.yaml");
  if (!(await pathExists(templatePath))) {
    return FALLBACK_MEMORY_YAML;
  }

  return fs.readFile(templatePath, "utf8");
}

async function pathExists(targetPath: string): Promise<boolean> {
  try {
    await fs.lstat(targetPath);
    return true;
  } catch {
    return false;
  }
}
