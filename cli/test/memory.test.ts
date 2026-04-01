import { afterEach, test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, lstat, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { findMemoryInParents, initAgent, initMemory, resolveActiveMemory, ROOT_DIR } from "../src/memory.js";

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

test("findMemoryInParents returns nearest parent ${ROOT_DIR}", async () => {
  const root = await makeTempDir("mem-cli-find-");
  const nested = path.join(root, "a", "b", "c");
  await mkdir(path.join(root, ROOT_DIR), { recursive: true });
  await mkdir(nested, { recursive: true });

  const found = await findMemoryInParents(nested);
  assert.equal(found, path.join(root, ROOT_DIR));
});

test("resolveActiveMemory finds ${ROOT_DIR} from nested directory", async () => {
  const root = await makeTempDir("mem-cli-resolve-");
  const nested = path.join(root, "a", "b", "c");
  await mkdir(path.join(root, ROOT_DIR), { recursive: true });
  await mkdir(nested, { recursive: true });

  const resolved = await resolveActiveMemory(nested);
  assert.equal(resolved, path.join(root, ROOT_DIR));
});

test("initMemory initializes local ${ROOT_DIR} outside worktree", async () => {
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
  const yaml = await readFile(path.join(cwd, ROOT_DIR, "memory.yaml"), "utf8");
  assert.equal(yaml, "apiVersion: memory.v1\n");
});

test("initMemory no-ops when parent already has ${ROOT_DIR}", async () => {
  const root = await makeTempDir("mem-cli-init-noop-");
  const nested = path.join(root, "x", "y");
  const memoryDir = path.join(root, ROOT_DIR);
  await mkdir(memoryDir, { recursive: true });
  await writeFile(path.join(memoryDir, "memory.yaml"), "apiVersion: memory.v1\n", "utf8");
  await mkdir(nested, { recursive: true });

  const result = await initMemory({
    cwd: nested,
    runGit: async () => {
      throw new Error("git should not be called");
    },
  });

  assert.equal(result.status, "noop");
  assert.equal(result.memoryPath, memoryDir);
});

test("initMemory re-initializes missing nested memory.yaml when parent ${ROOT_DIR} exists", async () => {
  const root = await makeTempDir("mem-cli-init-reinit-");
  const nested = path.join(root, "x", "y");
  const memoryDir = path.join(root, ROOT_DIR);
  const memoryYaml = path.join(memoryDir, "memory.yaml");

  await mkdir(memoryDir, { recursive: true });
  await mkdir(nested, { recursive: true });
  await rm(memoryYaml, { force: true });

  const result = await initMemory({
    cwd: nested,
    runGit: async () => {
      throw new Error("git should not be called");
    },
    templateContent: "apiVersion: memory.v1\n",
  });

  assert.equal(result.status, "initialized");
  assert.equal(result.memoryPath, memoryDir);
  const yaml = await readFile(memoryYaml, "utf8");
  assert.equal(yaml, "apiVersion: memory.v1\n");
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
  assert.equal(result.memoryPath, path.join(repoRoot, ROOT_DIR));

  const stat = await lstat(path.join(cwd, ROOT_DIR));
  assert.equal(stat.isSymbolicLink(), true);
});

test("initAgent requires ${ROOT_DIR} in current or parent directories", async () => {
  const cwd = await makeTempDir("mem-cli-agent-missing-");

  await assert.rejects(() => initAgent(cwd, "cli"), /Run `mem init` first/);
});

test("initAgent creates required directories", async () => {
  const cwd = await makeTempDir("mem-cli-agent-init-");
  await mkdir(path.join(cwd, ROOT_DIR), { recursive: true });

  const result = await initAgent(cwd, "cli");
  assert.equal(result.status, "initialized");

  const expectedDirs = [
    path.join(cwd, ROOT_DIR, "agents", "cli", "entry", "instructions"),
    path.join(cwd, ROOT_DIR, "agents", "cli", "entry", "tasks"),
    path.join(cwd, ROOT_DIR, "agents", "cli", "generated"),
    path.join(cwd, ROOT_DIR, "agents", "cli", "state"),
  ];

  for (const dirPath of expectedDirs) {
    const stat = await lstat(dirPath);
    assert.equal(stat.isDirectory(), true);
  }
});

test("initAgent no-ops when agent already exists", async () => {
  const cwd = await makeTempDir("mem-cli-agent-noop-");
  await mkdir(path.join(cwd, ROOT_DIR, "agents", "cli"), { recursive: true });

  const result = await initAgent(cwd, "cli");
  assert.equal(result.status, "noop");
  assert.match(result.message, /init skipped/);
});

test("initAgent resolves parent ${ROOT_DIR} when called from nested path", async () => {
  const root = await makeTempDir("mem-cli-agent-parent-");
  const nested = path.join(root, "pkg", "sub");
  await mkdir(path.join(root, ROOT_DIR), { recursive: true });
  await mkdir(nested, { recursive: true });

  const result = await initAgent(nested, "cli");
  assert.equal(result.status, "initialized");

  const stat = await lstat(path.join(root, ROOT_DIR, "agents", "cli", "state"));
  assert.equal(stat.isDirectory(), true);
});
