import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

export type GitRunner = (args: string[], cwd: string) => Promise<string>;

export function defaultGitRunner(args: string[], cwd: string): Promise<string> {
  return execFileAsync("git", args, { cwd }).then((res) => res.stdout);
}
