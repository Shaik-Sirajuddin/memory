import { afterEach, test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { runCli } from "../src/cli.js";
import { ROOT_DIR } from "../src/memory.js";

const tmpDirs: string[] = [];

afterEach(async () => {
  while (tmpDirs.length > 0) {
    const dir = tmpDirs.pop();
    if (dir) {
      await rm(dir, { recursive: true, force: true });
    }
  }
});

async function makeTempDir(prefix: string): Promise<string> {
  const dir = await mkdtemp(path.join(os.tmpdir(), prefix));
  tmpDirs.push(dir);
  return dir;
}

function createBuffers() {
  const stdout: string[] = [];
  const stderr: string[] = [];
  return {
    io: {
      stdout: (message: string) => stdout.push(message),
      stderr: (message: string) => stderr.push(message),
    },
    stdout,
    stderr,
  };
}

test("help output is available from root and --help", async () => {
  const a = createBuffers();
  const b = createBuffers();

  const codeA = await runCli([], a.io, { cwd: () => process.cwd() });
  const codeB = await runCli(["--help"], b.io, { cwd: () => process.cwd() });

  assert.equal(codeA, 0);
  assert.equal(codeB, 0);
  assert.match(a.stdout.join("\n"), /mem - memory bootstrap CLI/);
  assert.match(b.stdout.join("\n"), /Commands:/);
});

test("agent init fails cleanly when ${ROOT_DIR} is missing", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-missing-");
  const buffers = createBuffers();

  const code = await runCli(["agent", "init", "cli"], buffers.io, { cwd: () => cwd });

  assert.equal(code, 1);
  assert.match(buffers.stderr.join("\n"), /Run `mem init` first/);
});

test("agent init creates structure when ${ROOT_DIR} exists", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-agent-");
  const memoryDir = path.join(cwd, ROOT_DIR);
  await mkdir(memoryDir, { recursive: true });
  await writeFile(path.join(memoryDir, "memory.yaml"), "apiVersion: memory.v1\n", "utf8");
  const buffers = createBuffers();

  const code = await runCli(["agent", "init", "cli"], buffers.io, { cwd: () => cwd });

  assert.equal(code, 0);
  assert.match(buffers.stdout.join("\n"), /Initialized agent 'cli'/);
});

test("agent init skips when agent already exists", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-agent-noop-");
  const memoryDir = path.join(cwd, ROOT_DIR);
  await mkdir(memoryDir, { recursive: true });
  await writeFile(path.join(memoryDir, "memory.yaml"), "apiVersion: memory.v1\n", "utf8");
  const buffers = createBuffers();

  const first = await runCli(["agent", "init", "cli"], buffers.io, { cwd: () => cwd });
  const second = await runCli(["agent", "init", "cli"], buffers.io, { cwd: () => cwd });

  assert.equal(first, 0);
  assert.equal(second, 0);
  assert.match(buffers.stdout.join("\n"), /init skipped/);
});

test("agent init resolves parent ${ROOT_DIR} from nested path", async () => {
  const root = await makeTempDir("mem-cli-cmd-agent-parent-");
  const nested = path.join(root, "nested", "deeper");
  const memoryDir = path.join(root, ROOT_DIR);
  await mkdir(memoryDir, { recursive: true });
  await writeFile(path.join(memoryDir, "memory.yaml"), "apiVersion: memory.v1\n", "utf8");
  await mkdir(nested, { recursive: true });
  const buffers = createBuffers();

  const code = await runCli(["agent", "init", "test"], buffers.io, { cwd: () => nested });

  assert.equal(code, 0);
  assert.match(buffers.stdout.join("\n"), /Initialized agent 'test'/);
});

test("init command no-ops when parent already has ${ROOT_DIR}", async () => {
  const root = await makeTempDir("mem-cli-cmd-init-");
  const cwd = path.join(root, "nested");
  await mkdir(cwd, { recursive: true });
  const memoryDir = path.join(root, ROOT_DIR);
  await mkdir(memoryDir, { recursive: true });
  await writeFile(path.join(memoryDir, "memory.yaml"), "apiVersion: memory.v1\n", "utf8");
  const buffers = createBuffers();

  const code = await runCli(["init"], buffers.io, { cwd: () => cwd });

  assert.equal(code, 0);
  assert.match(buffers.stdout.join("\n"), /init skipped/);
});

test("missing agent name is a validation error", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-agent-name-");
  const memoryDir = path.join(cwd, ROOT_DIR);
  await mkdir(memoryDir, { recursive: true });
  await writeFile(path.join(memoryDir, "memory.yaml"), "apiVersion: memory.v1\n", "utf8");
  const buffers = createBuffers();

  const code = await runCli(["agent", "init"], buffers.io, { cwd: () => cwd });

  assert.equal(code, 1);
  assert.match(buffers.stderr.join("\n"), /Agent name is required/);
});

test("init with valid repo url adds submodule and seeds memory", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-init-remote-");
  const buffers = createBuffers();
  const gitCalls: Array<{ args: string[]; cwd: string }> = [];

  const code = await runCli(["init", "https://example.com/repo.git"], buffers.io, {
    cwd: () => cwd,
    runGit: async (args, commandCwd) => {
      gitCalls.push({ args, cwd: commandCwd });
      if (args[0] === "rev-parse" && args[1] === "--is-inside-work-tree") {
        return "true\n";
      }
      if (args[0] === "rev-parse" && args[1] === "--show-toplevel") {
        return `${cwd}\n`;
      }
      if (args[0] === "rev-parse" && args[1] === "--git-common-dir") {
        return `${path.join(cwd, ".git")}\n`;
      }
      if (args[0] === "submodule" && args[1] === "add") {
        await mkdir(path.join(cwd, ROOT_DIR), { recursive: true });
        return "";
      }
      throw new Error(`unexpected git args: ${args.join(" ")}`);
    },
  });

  assert.equal(code, 0);
  assert.match(buffers.stdout.join("\n"), /Added submodule https:\/\/example.com\/repo.git at memory/);
  const yaml = await readFile(path.join(cwd, ROOT_DIR, "memory.yaml"), "utf8");
  assert.match(yaml, /apiVersion: memory\.v1/);
  assert.equal(
    gitCalls.some((call) => call.args[0] === "submodule" && call.args[1] === "add" && call.args[2] === "https://example.com/repo.git" && call.args[3] === ROOT_DIR),
    true,
  );
});

test("init with invalid repo url initializes local git and seeds memory", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-init-local-git-");
  const buffers = createBuffers();
  const gitCalls: Array<{ args: string[]; cwd: string }> = [];

  const code = await runCli(["init", "not-a-repo-url"], buffers.io, {
    cwd: () => cwd,
    runGit: async (args, commandCwd) => {
      gitCalls.push({ args, cwd: commandCwd });
      if (args[0] === "rev-parse" && args[1] === "--is-inside-work-tree") {
        return "false\n";
      }
      if (args[0] === "init") {
        await mkdir(path.join(commandCwd, ".git"), { recursive: true });
        return "";
      }
      throw new Error(`unexpected git args: ${args.join(" ")}`);
    },
  });

  assert.equal(code, 0);
  assert.match(buffers.stdout.join("\n"), /Ignored invalid repo url 'not-a-repo-url'/);
  assert.equal(gitCalls.some((call) => call.args[0] === "init"), true);
  const yaml = await readFile(path.join(cwd, ROOT_DIR, "memory.yaml"), "utf8");
  assert.match(yaml, /apiVersion: memory\.v1/);
});

test("fetch uses passed local agents and applies sparse paths", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-fetch-args-");
  const memoryDir = path.join(cwd, ROOT_DIR);
  await mkdir(memoryDir, { recursive: true });
  await writeFile(path.join(memoryDir, "memory.yaml"), "apiVersion: memory.v1\n", "utf8");
  await writeFile(
    path.join(memoryDir, "agent-team.md"),
    [
      "planner: passive=[observer], active=[builder,runner]",
      "observer: passive=[], active=[]",
    ].join("\n"),
    "utf8",
  );

  const buffers = createBuffers();
  const gitCalls: Array<{ args: string[]; cwd: string }> = [];
  const code = await runCli(["fetch", "planner"], buffers.io, {
    cwd: () => cwd,
    runGit: async (args, commandCwd) => {
      gitCalls.push({ args, cwd: commandCwd });
      return "";
    },
  });

  assert.equal(code, 0);
  assert.match(buffers.stdout.join("\n"), /Fetched agent dependencies for planner/);
  assert.equal(
    gitCalls.some(
      (call) =>
        call.args[0] === "sparse-checkout" &&
        call.args[1] === "set" &&
        call.args.includes("agents/builder") &&
        call.args.includes("agents/runner") &&
        call.args.includes("agents/observer/generated"),
    ),
    true,
  );
});

test("fetch reads local agents from memory.local when args are omitted", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-fetch-local-");
  const memoryDir = path.join(cwd, ROOT_DIR);
  await mkdir(memoryDir, { recursive: true });
  await writeFile(path.join(memoryDir, "memory.yaml"), "apiVersion: memory.v1\n", "utf8");
  await writeFile(path.join(memoryDir, "memory.local"), "agents:\n  - planner\n", "utf8");
  await writeFile(path.join(memoryDir, "agent-team.md"), "planner: passive=[observer], active=[builder]\n", "utf8");

  const buffers = createBuffers();
  const gitCalls: Array<{ args: string[]; cwd: string }> = [];
  const code = await runCli(["fetch"], buffers.io, {
    cwd: () => cwd,
    runGit: async (args, commandCwd) => {
      gitCalls.push({ args, cwd: commandCwd });
      return "";
    },
  });

  assert.equal(code, 0);
  assert.match(buffers.stdout.join("\n"), /Fetched agent dependencies for planner/);
  assert.equal(
    gitCalls.some(
      (call) =>
        call.args[0] === "sparse-checkout" &&
        call.args[1] === "set" &&
        call.args.includes("agents/builder") &&
        call.args.includes("agents/observer/generated"),
    ),
    true,
  );
});
