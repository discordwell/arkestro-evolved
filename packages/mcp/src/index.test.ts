import test from "node:test";
import assert from "node:assert/strict";
import { bearerToken, toolDefinitions, transportModeFromArgs } from "./index.js";

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
