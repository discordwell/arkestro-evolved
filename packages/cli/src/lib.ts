import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

// Pure, side-effect-light helpers for the evo CLI, kept out of index.ts so the
// command wiring (which calls program.parseAsync at import time) does not run
// when these are unit-tested.

export interface StoredAuth {
  baseUrl: string;
  accessToken: string;
  userEmail?: string;
  tokenPreview?: string;
}

export interface ResolvedClientOptions {
  baseUrl?: string;
  accessToken?: string;
  actorSurface: string;
  actorAgent: string;
  actorUser?: string;
}

// Terminal run statuses: the run is finished and will never change again.
// Mirrors isTerminalRunStatus in the Go API.
export const TERMINAL_RUN_STATUSES = ["completed", "failed", "rejected"] as const;

const terminalRunStatuses = new Set<string>(TERMINAL_RUN_STATUSES);

export function isTerminalRunStatus(status: string | undefined): boolean {
  return terminalRunStatuses.has(status ?? "");
}

// Settled run statuses for `run watch`: the run has either finished or parked
// at an approval gate, where it cannot make further automated progress until an
// out-of-band approve/reject decision. Mirrors SETTLED_RUN_STATUSES in the MCP
// server so the CLI and MCP agree on when to hand control back. Without
// awaiting_approval here, watching any approval-required runbook would poll
// forever, re-emitting an unchanged envelope, since that status is not terminal.
export const SETTLED_RUN_STATUSES = [...TERMINAL_RUN_STATUSES, "awaiting_approval"] as const;

const settledRunStatuses = new Set<string>(SETTLED_RUN_STATUSES);

export function isSettledRunStatus(status: string | undefined): boolean {
  return settledRunStatuses.has(status ?? "");
}

// parseContext turns repeated `key=value` flags into an object. The first `=`
// separates the key from the value, so values may themselves contain `=`. A
// bare `key` maps to an empty string; an entry with an empty key is ignored.
export function parseContext(values: string[]): Record<string, string> {
  return values.reduce<Record<string, string>>((acc, entry) => {
    const [key, ...rest] = entry.split("=");
    if (key) acc[key] = rest.join("=");
    return acc;
  }, {});
}

// authFilePath resolves the on-disk auth token location, honoring
// XDG_CONFIG_HOME and falling back to ~/.config. env and homedir are injectable
// for testing.
export function authFilePath(
  env: NodeJS.ProcessEnv = process.env,
  homedir: () => string = os.homedir
): string {
  const configRoot = env.XDG_CONFIG_HOME || path.join(homedir(), ".config");
  return path.join(configRoot, "evo", "auth.json");
}

export async function readStoredAuth(filePath: string): Promise<StoredAuth | null> {
  try {
    const raw = await readFile(filePath, "utf8");
    return JSON.parse(raw) as StoredAuth;
  } catch {
    return null;
  }
}

export async function writeStoredAuth(filePath: string, auth: StoredAuth): Promise<void> {
  await mkdir(path.dirname(filePath), { recursive: true });
  await writeFile(filePath, `${JSON.stringify(auth, null, 2)}\n`, "utf8");
}

export async function clearStoredAuth(filePath: string): Promise<void> {
  await rm(filePath, { force: true });
}

// resolveClientOptions merges process environment over persisted auth: an
// explicit env var always wins, otherwise the saved session is used.
export function resolveClientOptions(
  env: NodeJS.ProcessEnv,
  stored: StoredAuth | null
): ResolvedClientOptions {
  return {
    baseUrl: env.EVO_API_BASE_URL || stored?.baseUrl,
    accessToken: env.EVO_API_TOKEN || stored?.accessToken,
    actorSurface: "cli",
    actorAgent: env.EVO_ACTOR_AGENT || "human",
    actorUser: env.EVO_ACTOR_USER || stored?.userEmail
  };
}

// parseIntervalMs coerces a CLI-provided poll interval into a safe number.
// Non-numeric, zero, or negative input falls back to the default so a typo like
// `--interval-ms abc` cannot turn the watch loop into setTimeout(fn, NaN) and
// poll the API with no delay.
export function parseIntervalMs(value: unknown, fallback = 1000, minimum = 1): number {
  const num = Number(value);
  return Number.isFinite(num) && num >= minimum ? num : fallback;
}

const defaultSleep = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));

export interface WatchRunLoopOptions<T> {
  getRun: () => Promise<T>;
  emit: (run: T) => void;
  intervalMs?: number;
  sleep?: (ms: number) => Promise<void>;
}

// watchRunLoop polls a run, emitting each envelope, until it reaches a settled
// status (terminal, or parked awaiting approval), then returns the final
// envelope. Stopping at an approval gate mirrors the MCP server: the run cannot
// advance without an out-of-band decision, so polling on would only re-emit the
// same envelope forever. Sleep is injectable so tests do not wait on real timers.
export async function watchRunLoop<T extends { run: { status?: string } }>({
  getRun,
  emit,
  intervalMs = 1000,
  sleep = defaultSleep
}: WatchRunLoopOptions<T>): Promise<T> {
  for (;;) {
    const run = await getRun();
    emit(run);
    if (isSettledRunStatus(run.run.status)) {
      return run;
    }
    await sleep(intervalMs);
  }
}
