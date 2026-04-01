export function rootHelp(): string {
  return [
    "mem - memory bootstrap CLI",
    "",
    "Usage:",
    "  mem init [repo_url]",
    "  mem fetch [agent_name ...]",
    "  mem agent init <name>",
    "  mem help [command]",
    "",
    "Commands:",
    "  init              Initialize memory in this workspace context",
    "  fetch             Sparse-fetch dependency agent paths",
    "  agent init <name> Initialize memory/agents/<name> structure",
    "  help              Show command help",
  ].join("\n");
}

export function initHelp(): string {
  return [
    "Usage:",
    "  mem init [repo_url]",
    "",
    "Behavior:",
    "  - Accepts an optional repo url.",
    "  - If a valid repo url is provided, runs: git submodule add <repo_url> memory.",
    "  - If no valid repo url is provided and git is missing, runs git init in the current directory.",
    "  - Seeds memory.yaml regardless of remote/local initialization path.",
    "  - In a git worktree, initializes memory at dirname($(git rev-parse --git-common-dir)).",
    "  - In a worktree subdirectory, also creates memory symlink in the current directory.",
  ].join("\n");
}

export function agentInitHelp(): string {
  return [
    "Usage:",
    "  mem agent init <name>",
    "",
    "Behavior:",
    "  - Requires memory in current or parent directories.",
    "  - Creates:",
    "    memory/agents/<name>/entry/instructions",
    "    memory/agents/<name>/entry/tasks",
    "    memory/agents/<name>/generated",
    "    memory/agents/<name>/state",
    "  - Safe to re-run; creation is idempotent.",
  ].join("\n");
}
