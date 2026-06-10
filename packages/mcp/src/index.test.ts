import test from "node:test";
import assert from "node:assert/strict";
import type { EvoClient } from "@evo/sdk";
import { bearerToken, executeTool, pollRunUntilSettled, toolDefinitions, transportModeFromArgs } from "./index.js";

function envelopeSequence(statuses: string[]): { getRun: () => Promise<{ run: { status: string } }>; calls: () => number } {
  let index = 0;
  return {
    getRun: async () => {
      const status = statuses[Math.min(index, statuses.length - 1)];
      index += 1;
      return { run: { status } };
    },
    calls: () => index
  };
}

const instantSleep = async (): Promise<void> => {};

test("tool catalog exposes the expected remote surfaces", () => {
  assert.ok(toolDefinitions.some((tool) => tool.name === "runbook.run"));
  assert.ok(toolDefinitions.some((tool) => tool.name === "approval.approve"));
});

test("transport mode parser favors explicit http mode", () => {
  assert.equal(transportModeFromArgs(["--transport=http"]), "http");
  assert.equal(transportModeFromArgs([]), "stdio");
});

test("bearer token parser extracts bearer credentials", () => {
  assert.equal(bearerToken("Bearer abc123"), "abc123");
  assert.equal(bearerToken("Basic abc123"), "");
});

test("run.stream tool advertises a poll timeout", () => {
  const stream = toolDefinitions.find((tool) => tool.name === "run.stream");
  assert.ok(stream);
  assert.ok("timeout_ms" in stream.inputSchema.properties);
});

test("pollRunUntilSettled returns once the run is terminal", async () => {
  const { getRun, calls } = envelopeSequence(["queued", "running", "completed"]);
  const envelope = await pollRunUntilSettled(getRun, { sleep: instantSleep });
  assert.equal(envelope.run.status, "completed");
  assert.equal(calls(), 3);
});

test("pollRunUntilSettled hands control back when approval is needed", async () => {
  const { getRun, calls } = envelopeSequence(["running", "awaiting_approval", "completed"]);
  const envelope = await pollRunUntilSettled(getRun, { sleep: instantSleep });
  assert.equal(envelope.run.status, "awaiting_approval");
  assert.equal(calls(), 2);
});

test("pollRunUntilSettled stops at the deadline instead of hanging", async () => {
  const { getRun } = envelopeSequence(["running"]);
  const envelope = await pollRunUntilSettled(getRun, { timeoutMs: 0, sleep: instantSleep });
  assert.equal(envelope.run.status, "running");
});

test("run.stream honors timeout_ms=0 as a single check", async () => {
  const { getRun, calls } = envelopeSequence(["running"]);
  const client = { getRun: (_runId: string) => getRun() } as unknown as EvoClient;
  const envelope = (await executeTool(client, "run.stream", { run_id: "run-1", timeout_ms: 0 })) as {
    run: { status: string };
  };
  assert.equal(envelope.run.status, "running");
  assert.equal(calls(), 1);
});
