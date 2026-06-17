import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import {
  authFilePath,
  clearStoredAuth,
  isTerminalRunStatus,
  parseContext,
  parseIntervalMs,
  readStoredAuth,
  resolveClientOptions,
  watchRunLoop,
  writeStoredAuth,
  type StoredAuth
} from "./lib.js";

test("parseContext splits key=value pairs and keeps the first separator", () => {
  assert.deepEqual(parseContext(["a=b", "c=d"]), { a: "b", c: "d" });
  // Only the first `=` separates; values may contain more.
  assert.deepEqual(parseContext(["url=http://x?a=1&b=2"]), { url: "http://x?a=1&b=2" });
});

test("parseContext treats a bare key as an empty value", () => {
  assert.deepEqual(parseContext(["flag"]), { flag: "" });
  assert.deepEqual(parseContext(["flag="]), { flag: "" });
});

test("parseContext ignores empty entries and entries with no key", () => {
  assert.deepEqual(parseContext([]), {});
  assert.deepEqual(parseContext([""]), {});
  assert.deepEqual(parseContext(["=value"]), {});
});

test("parseContext lets a later duplicate key win", () => {
  assert.deepEqual(parseContext(["k=1", "k=2"]), { k: "2" });
});

test("isTerminalRunStatus recognizes only terminal statuses", () => {
  for (const status of ["completed", "failed", "rejected"]) {
    assert.equal(isTerminalRunStatus(status), true, status);
  }
  for (const status of ["queued", "running", "awaiting_approval", "", undefined]) {
    assert.equal(isTerminalRunStatus(status), false, String(status));
  }
});

test("parseIntervalMs accepts positive numbers and falls back otherwise", () => {
  assert.equal(parseIntervalMs("250"), 250);
  assert.equal(parseIntervalMs(500), 500);
  // Non-numeric, zero, and negative inputs must not become setTimeout(fn, 0/NaN).
  assert.equal(parseIntervalMs("abc"), 1000);
  assert.equal(parseIntervalMs("0"), 1000);
  assert.equal(parseIntervalMs("-5"), 1000);
  assert.equal(parseIntervalMs(undefined), 1000);
  assert.equal(parseIntervalMs("abc", 2000), 2000);
});

test("authFilePath honors XDG_CONFIG_HOME and falls back to the home dir", () => {
  assert.equal(
    authFilePath({ XDG_CONFIG_HOME: "/tmp/cfg" }, () => "/home/me"),
    path.join("/tmp/cfg", "evo", "auth.json")
  );
  assert.equal(
    authFilePath({}, () => "/home/me"),
    path.join("/home/me", ".config", "evo", "auth.json")
  );
});

test("resolveClientOptions prefers env vars over stored auth", () => {
  const stored: StoredAuth = {
    baseUrl: "http://stored",
    accessToken: "stored-token",
    userEmail: "stored@example.com"
  };
  const env = {
    EVO_API_BASE_URL: "http://env",
    EVO_API_TOKEN: "env-token",
    EVO_ACTOR_AGENT: "codex",
    EVO_ACTOR_USER: "env@example.com"
  };
  assert.deepEqual(resolveClientOptions(env, stored), {
    baseUrl: "http://env",
    accessToken: "env-token",
    actorSurface: "cli",
    actorAgent: "codex",
    actorUser: "env@example.com"
  });
});

test("resolveClientOptions falls back to stored auth and sensible defaults", () => {
  const stored: StoredAuth = {
    baseUrl: "http://stored",
    accessToken: "stored-token",
    userEmail: "stored@example.com"
  };
  assert.deepEqual(resolveClientOptions({}, stored), {
    baseUrl: "http://stored",
    accessToken: "stored-token",
    actorSurface: "cli",
    actorAgent: "human",
    actorUser: "stored@example.com"
  });
});

test("resolveClientOptions tolerates a null stored session", () => {
  assert.deepEqual(resolveClientOptions({}, null), {
    baseUrl: undefined,
    accessToken: undefined,
    actorSurface: "cli",
    actorAgent: "human",
    actorUser: undefined
  });
});

test("auth storage round-trips and clears", async () => {
  const dir = await mkdtemp(path.join(os.tmpdir(), "evo-cli-test-"));
  try {
    const filePath = path.join(dir, "evo", "auth.json");
    const auth: StoredAuth = {
      baseUrl: "http://127.0.0.1:8080",
      accessToken: "evo_secret",
      userEmail: "admin@evo.local",
      tokenPreview: "evo_secret12"
    };

    assert.equal(await readStoredAuth(filePath), null, "missing file reads as null");

    await writeStoredAuth(filePath, auth);
    assert.deepEqual(await readStoredAuth(filePath), auth);

    await clearStoredAuth(filePath);
    assert.equal(await readStoredAuth(filePath), null, "cleared file reads as null");

    // Clearing an already-missing file must not throw.
    await clearStoredAuth(filePath);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("readStoredAuth returns null for malformed JSON", async () => {
  const dir = await mkdtemp(path.join(os.tmpdir(), "evo-cli-test-"));
  try {
    const filePath = path.join(dir, "auth.json");
    await writeFile(filePath, "not json", "utf8");
    assert.equal(await readStoredAuth(filePath), null);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("watchRunLoop returns immediately for an already-terminal run", async () => {
  const emitted: string[] = [];
  let sleeps = 0;
  const result = await watchRunLoop({
    getRun: async () => ({ run: { status: "completed" } }),
    emit: (run) => emitted.push(run.run.status ?? ""),
    sleep: async () => {
      sleeps += 1;
    }
  });
  assert.equal(result.run.status, "completed");
  assert.deepEqual(emitted, ["completed"]);
  assert.equal(sleeps, 0, "a terminal run must not sleep");
});

test("watchRunLoop polls until a run settles, emitting and sleeping between", async () => {
  const statuses = ["queued", "running", "completed"];
  const emitted: string[] = [];
  const sleepIntervals: number[] = [];
  let i = 0;
  const result = await watchRunLoop({
    getRun: async () => ({ run: { status: statuses[i++] } }),
    emit: (run) => emitted.push(run.run.status ?? ""),
    intervalMs: 5,
    sleep: async (ms) => {
      sleepIntervals.push(ms);
    }
  });
  assert.equal(result.run.status, "completed");
  assert.deepEqual(emitted, ["queued", "running", "completed"]);
  // Sleeps happen only between non-terminal polls: two here, both at the interval.
  assert.deepEqual(sleepIntervals, [5, 5]);
});
