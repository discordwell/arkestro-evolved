import test from "node:test";
import assert from "node:assert/strict";
import { EvoClient } from "./index.js";

test("client uses default headers and base url", () => {
  const client = new EvoClient({ actorAgent: "codex", actorSurface: "cli", accessToken: "token-123" });
  assert.equal(client.baseUrl, process.env.EVO_API_BASE_URL || "http://127.0.0.1:8080");
  assert.equal(client.headers().Authorization, "Bearer token-123");
  assert.equal(client.headers()["X-Actor-Agent"], "codex");
});

test("headers omit Authorization when there is no token", () => {
  const client = new EvoClient({ baseUrl: "http://test" });
  assert.equal(client.headers().Authorization, undefined);
  assert.equal(client.headers()["X-Actor-Surface"], "cli");
});

test("withAccessToken returns a new client carrying the token without mutating the original", () => {
  const base = new EvoClient({ baseUrl: "http://test", accessToken: "old", actorAgent: "codex" });
  const next = base.withAccessToken("new");
  assert.equal(next.accessToken, "new");
  assert.equal(next.headers().Authorization, "Bearer new");
  assert.equal(next.actorAgent, "codex", "actor attribution carries over");
  assert.equal(base.accessToken, "old", "original client is unchanged");
});

// A fetch stub that yields a fresh Response per call so openapi-fetch can parse
// the body without hitting an already-consumed stream.
function clientReturning(body: unknown, init: ResponseInit): EvoClient {
  const fetch = (async () =>
    new Response(JSON.stringify(body), {
      headers: { "Content-Type": "application/json" },
      ...init
    })) as typeof globalThis.fetch;
  return new EvoClient({ baseUrl: "http://test", accessToken: "t", fetch });
}

test("list calls unwrap the items envelope", async () => {
  const client = clientReturning({ items: [{ id: "w1" }, { id: "w2" }] }, { status: 200 });
  const workspaces = await client.listWorkspaces();
  assert.deepEqual(
    workspaces.map((w) => w.id),
    ["w1", "w2"]
  );
});

test("item calls unwrap the item envelope", async () => {
  const client = clientReturning({ item: { id: "w1", name: "Ops" } }, { status: 201 });
  const workspace = await client.createWorkspace({ name: "Ops", slug: "ops" });
  assert.equal(workspace.id, "w1");
});

test("API error envelopes surface as an Error with status and message", async () => {
  const client = clientReturning(
    { error: "workspace_id is required" },
    { status: 400, statusText: "Bad Request" }
  );
  await assert.rejects(client.listWorkspaces(), /400 Bad Request: workspace_id is required/);
});

test("errors without an error field fall back to a generic message", async () => {
  const client = clientReturning({}, { status: 500, statusText: "Internal Server Error" });
  await assert.rejects(client.listWorkspaces(), /500 Internal Server Error: request failed/);
});
