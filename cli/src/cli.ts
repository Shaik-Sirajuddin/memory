import process from "node:process";
import { agentInitHelp, initHelp, rootHelp } from "./help.js";
import { initAgent, initMemory } from "./memory.js";

export interface CliIO {
  stdout: (message: string) => void;
  stderr: (message: string) => void;
}

export interface CliContext {
  cwd: () => string;
}

const DEFAULT_IO: CliIO = {
  stdout: (message) => process.stdout.write(`${message}\n`),
  stderr: (message) => process.stderr.write(`${message}\n`),
};

const DEFAULT_CONTEXT: CliContext = {
  cwd: () => process.cwd(),
};

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
      const result = await initMemory({ cwd: context.cwd() });
      io.stdout(result.message);
      return 0;
    } catch (error) {
      io.stderr(`init failed: ${errorMessage(error)}`);
      return 1;
    }
  }

  if (command === "agent") {
    return runAgentCommand(rest, io, context);
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
    const result = await initAgent(context.cwd(), name);
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

  return rootHelp();
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
