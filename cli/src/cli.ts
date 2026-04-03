import process from "node:process";
import { promises as fs } from "node:fs";
import path from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { agentInitHelp, initHelp, rootHelp } from "./help.js";
import { fetchMemory, initAgent, initMemory, ROOT_DIR } from "./memory.js";

export interface CliIO {
  stdout: (message: string) => void;
  stderr: (message: string) => void;
}

export interface CliContext {
  cwd: () => string;
  runGit?: (args: string[], cwd: string) => Promise<string>;
}

const DEFAULT_IO: CliIO = {
  stdout: (message) => process.stdout.write(`${message}\n`),
  stderr: (message) => process.stderr.write(`${message}\n`),
};

const DEFAULT_CONTEXT: CliContext = {
  cwd: () => process.cwd(),
};
const execFileAsync = promisify(execFile);

export async function runCli(
  args: string[],
  io: CliIO = DEFAULT_IO,
  context: CliContext = DEFAULT_CONTEXT,
): Promise<number> {
  const [command, ...rest] = args;

  if (!command || command === "help" || command === "--help" || command === "-h") {
    io.stdout(resolveHelp(rest));
    return 0;
  }

  if (command === "init") {
    if (rest.includes("--help") || rest.includes("-h")) {
      io.stdout(initHelp());
      return 0;
    }

    try {
      const cwd = context.cwd();
      const runGit = context.runGit ?? defaultGitRunner;
      const [repoUrl] = rest;
      const notes: string[] = [];

      await ensureLocalGitRepo(cwd, runGit, notes);

      if (repoUrl && isValidRepoUrl(repoUrl)) {
        const repoRoot = (await runGit(["rev-parse", "--show-toplevel"], cwd)).trim();
        if (!repoRoot) {
          throw new Error("Failed to resolve git repository root.");
        }

        const submodulePath = path.join(repoRoot, ROOT_DIR);
        if (!(await pathExists(submodulePath))) {
          await runGit(["submodule", "add", repoUrl, ROOT_DIR], repoRoot);
          notes.push(`Added submodule ${repoUrl} at ${ROOT_DIR}.`);
        }
      } else if (repoUrl) {
        notes.push(`Ignored invalid repo url '${repoUrl}'.`);
      }

      const result = await initMemory({ cwd, runGit });
      io.stdout([...notes, result.message].join(" ").trim());
      return 0;
    } catch (error) {
      io.stderr(`init failed: ${errorMessage(error)}`);
      return 1;
    }
  }

  if (command === "agent") {
    return runAgentCommand(rest, io, context);
  }

  if (command === "fetch") {
    if (rest.includes("--help") || rest.includes("-h")) {
      io.stdout(fetchHelp());
      return 0;
    }

    try {
      const result = await fetchMemory(context.cwd(), rest, context.runGit ?? defaultGitRunner);
      io.stdout(result.message);
      return 0;
    } catch (error) {
      io.stderr(`fetch failed: ${errorMessage(error)}`);
      return 1;
    }
  }

  io.stderr(`Unknown command: ${command}`);
  io.stderr("Run `mem help` for usage.");
  return 1;
}

async function runAgentCommand(args: string[], io: CliIO, context: CliContext): Promise<number> {
  const [subcommand, ...rest] = args;

  if (!subcommand || subcommand === "--help" || subcommand === "-h") {
    io.stdout(agentInitHelp());
    return 0;
  }

  if (subcommand !== "init") {
    io.stderr(`Unknown agent subcommand: ${subcommand}`);
    io.stderr("Run `mem help agent init` for usage.");
    return 1;
  }

  if (rest.includes("--help") || rest.includes("-h")) {
    io.stdout(agentInitHelp());
    return 0;
  }

  const [name] = rest;
  if (!name) {
    io.stderr("Agent name is required. Usage: mem agent init <name>");
    return 1;
  }

  try {
    const runGit = context.runGit ?? defaultGitRunner;
    const result = await initAgent(context.cwd(), name, runGit);
    io.stdout(result.message);
    return 0;
  } catch (error) {
    io.stderr(`agent init failed: ${errorMessage(error)}`);
    return 1;
  }
}

function resolveHelp(args: string[]): string {
  if (args.length === 0) {
    return rootHelp();
  }

  if (args[0] === "init") {
    return initHelp();
  }

  if (args[0] === "agent" || args.join(" ") === "agent init") {
    return agentInitHelp();
  }

  if (args[0] === "fetch") {
    return fetchHelp();
  }

  return rootHelp();
}

function fetchHelp(): string {
  return [
    "Usage:",
    "  mem fetch [agent_name ...]",
    "",
    "Behavior:",
    `  - Requires ${ROOT_DIR} in current or parent directories.`,
    "  - If no agent names are passed, reads local agents from memory.local (agents).",
    "  - Reads dependency graph from agent-team.md in memory root.",
    "  - Uses git sparse-checkout:",
    "    active deps -> agents/<name>",
    "    passive deps -> agents/<name>/generated",
  ].join("\n");
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

async function ensureLocalGitRepo(
  cwd: string,
  runGit: (args: string[], cwd: string) => Promise<string>,
  notes: string[],
): Promise<void> {
  let insideWorktree = false;
  try {
    insideWorktree = (await runGit(["rev-parse", "--is-inside-work-tree"], cwd)).trim() === "true";
  } catch {
    insideWorktree = false;
  }

  if (!insideWorktree) {
    await runGit(["init"], cwd);
    notes.push(`Initialized git repository at ${cwd}.`);
  }
}

function isValidRepoUrl(value: string): boolean {
  try {
    const parsed = new URL(value);
    if (parsed.protocol === "http:" || parsed.protocol === "https:" || parsed.protocol === "ssh:" || parsed.protocol === "git:") {
      return true;
    }
  } catch {
    // fall through to SCP-like git remotes below
  }

  return /^[^@\s]+@[^:\s]+:.+/.test(value);
}

function defaultGitRunner(args: string[], cwd: string): Promise<string> {
  return execFileAsync("git", args, { cwd }).then((res) => res.stdout);
}

async function pathExists(targetPath: string): Promise<boolean> {
  try {
    await fs.lstat(targetPath);
    return true;
  } catch {
    return false;
  }
}
