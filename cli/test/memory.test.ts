import { afterEach, test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, lstat, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { findMemoryInParents, initAgent, initMemory } from "../src/memory.js";

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

test("findMemoryInParents returns nearest parent .memory", async () => {
  const root = await makeTempDir("mem-cli-find-");
  const nested = path.join(root, "a", "b", "c");
  await mkdir(path.join(root, ".memory"), { recursive: true });
  await mkdir(nested, { recursive: true });

  const found = await findMemoryInParents(nested);
  assert.equal(found, path.join(root, ".memory"));
});

test("initMemory initializes local .memory outside worktree", async () => {
  const cwd = await makeTempDir("mem-cli-init-local-");

  const result = await initMemory({
    cwd,
    runGit: async (args) => {
      if (args[1] === "--is-inside-work-tree") {
        return "false\n";
      }
      throw new Error("unexpected git command");
    },
    templateContent: "apiVersion: memory.v1\n",
  });

  assert.equal(result.status, "initialized");
  const yaml = await readFile(path.join(cwd, ".memory", "memory.yaml"), "utf8");
  assert.equal(yaml, "apiVersion: memory.v1\n");
});

test("initMemory no-ops when parent already has .memory", async () => {
  const root = await makeTempDir("mem-cli-init-noop-");
  const nested = path.join(root, "x", "y");
  await mkdir(path.join(root, ".memory"), { recursive: true });
  await mkdir(nested, { recursive: true });

  const result = await initMemory({
    cwd: nested,
    runGit: async () => {
      throw new Error("git should not be called");
    },
  });

  assert.equal(result.status, "noop");
  assert.equal(result.memoryPath, path.join(root, ".memory"));
});

test("initMemory creates root memory and cwd symlink in worktree mode", async () => {
  const repoRoot = await makeTempDir("mem-cli-worktree-");
  const cwd = path.join(repoRoot, "packages", "cli");
  await mkdir(cwd, { recursive: true });

  const result = await initMemory({
    cwd,
    runGit: async (args) => {
      if (args[1] === "--is-inside-work-tree") {
        return "true\n";
      }
      if (args[1] === "--git-common-dir") {
        return `${path.join(repoRoot, ".git")}\n`;
      }
      throw new Error(`unexpected git args: ${args.join(" ")}`);
    },
    templateContent: "apiVersion: memory.v1\n",
  });

  assert.equal(result.status, "initialized");
  assert.equal(result.memoryPath, path.join(repoRoot, ".memory"));

  const stat = await lstat(path.join(cwd, ".memory"));
  assert.equal(stat.isSymbolicLink(), true);
});

test("initAgent requires .memory in current directory", async () => {
  const cwd = await makeTempDir("mem-cli-agent-missing-");

  await assert.rejects(() => initAgent(cwd, "cli"), /Run `mem init` first/);
});

test("initAgent creates required directories", async () => {
  const cwd = await makeTempDir("mem-cli-agent-init-");
  await mkdir(path.join(cwd, ".memory"), { recursive: true });

  const result = await initAgent(cwd, "cli");
  assert.equal(result.status, "initialized");

  const expectedDirs = [
    path.join(cwd, ".memory", "agents", "cli", "entry", "instructions"),
    path.join(cwd, ".memory", "agents", "cli", "entry", "tasks"),
    path.join(cwd, ".memory", "agents", "cli", "generated"),
    path.join(cwd, ".memory", "agents", "cli", "state"),
  ];

  for (const dirPath of expectedDirs) {
    const stat = await lstat(dirPath);
    assert.equal(stat.isDirectory(), true);
  }
});
