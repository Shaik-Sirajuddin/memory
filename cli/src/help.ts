export function rootHelp(): string {
  return [
    "mem - memory bootstrap CLI",
    "",
    "Usage:",
    "  mem init",
    "  mem agent init <name>",
    "  mem help [command]",
    "",
    "Commands:",
    "  init              Initialize .memory in this workspace context",
    "  agent init <name> Initialize .memory/agents/<name> structure",
    "  help              Show command help",
  ].join("\n");
}

export function initHelp(): string {
  return [
    "Usage:",
    "  mem init",
    "",
    "Behavior:",
    "  - Recursively checks current and parent directories for existing .memory.",
    "  - If found, init is skipped as a successful no-op.",
    "  - In a git worktree, initializes .memory at dirname($(git rev-parse --git-common-dir)).",
    "  - In a worktree subdirectory, also creates .memory symlink in the current directory.",
    "  - Outside git, initializes .memory in the current directory.",
  ].join("\n");
}

export function agentInitHelp(): string {
  return [
    "Usage:",
    "  mem agent init <name>",
    "",
    "Behavior:",
    "  - Requires .memory in current or parent directories.",
    "  - Creates:",
    "    .memory/agents/<name>/entry/instructions",
    "    .memory/agents/<name>/entry/tasks",
    "    .memory/agents/<name>/generated",
    "    .memory/agents/<name>/state",
    "  - Safe to re-run; creation is idempotent.",
  ].join("\n");
}
