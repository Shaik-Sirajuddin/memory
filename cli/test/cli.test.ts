import { afterEach, test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { runCli } from "../src/cli.js";

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

test("agent init fails cleanly when .memory is missing", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-missing-");
  const buffers = createBuffers();

  const code = await runCli(["agent", "init", "cli"], buffers.io, { cwd: () => cwd });

  assert.equal(code, 1);
  assert.match(buffers.stderr.join("\n"), /Run `mem init` first/);
});

test("agent init creates structure when .memory exists", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-agent-");
  await mkdir(path.join(cwd, ".memory"), { recursive: true });
  const buffers = createBuffers();

  const code = await runCli(["agent", "init", "cli"], buffers.io, { cwd: () => cwd });

  assert.equal(code, 0);
  assert.match(buffers.stdout.join("\n"), /Initialized agent 'cli'/);
});

test("agent init skips when agent already exists", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-agent-noop-");
  await mkdir(path.join(cwd, ".memory"), { recursive: true });
  const buffers = createBuffers();

  const first = await runCli(["agent", "init", "cli"], buffers.io, { cwd: () => cwd });
  const second = await runCli(["agent", "init", "cli"], buffers.io, { cwd: () => cwd });

  assert.equal(first, 0);
  assert.equal(second, 0);
  assert.match(buffers.stdout.join("\n"), /init skipped/);
});

test("agent init resolves parent .memory from nested path", async () => {
  const root = await makeTempDir("mem-cli-cmd-agent-parent-");
  const nested = path.join(root, "nested", "deeper");
  await mkdir(path.join(root, ".memory"), { recursive: true });
  await mkdir(nested, { recursive: true });
  const buffers = createBuffers();

  const code = await runCli(["agent", "init", "test"], buffers.io, { cwd: () => nested });

  assert.equal(code, 0);
  assert.match(buffers.stdout.join("\n"), /Initialized agent 'test'/);
});

test("init command no-ops when parent already has .memory", async () => {
  const root = await makeTempDir("mem-cli-cmd-init-");
  const cwd = path.join(root, "nested");
  await mkdir(cwd, { recursive: true });
  await mkdir(path.join(root, ".memory"), { recursive: true });
  const buffers = createBuffers();

  const code = await runCli(["init"], buffers.io, { cwd: () => cwd });

  assert.equal(code, 0);
  assert.match(buffers.stdout.join("\n"), /init skipped/);
});

test("missing agent name is a validation error", async () => {
  const cwd = await makeTempDir("mem-cli-cmd-agent-name-");
  await mkdir(path.join(cwd, ".memory"), { recursive: true });
  const buffers = createBuffers();

  const code = await runCli(["agent", "init"], buffers.io, { cwd: () => cwd });

  assert.equal(code, 1);
  assert.match(buffers.stderr.join("\n"), /Agent name is required/);
});
