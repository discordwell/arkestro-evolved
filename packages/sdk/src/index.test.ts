import test from "node:test";
import assert from "node:assert/strict";
import { EvoClient } from "./index.js";

test("client uses default headers and base url", () => {
  const client = new EvoClient({ actorAgent: "codex", actorSurface: "cli", accessToken: "token-123" });
  assert.equal(client.baseUrl, process.env.EVO_API_BASE_URL || "http://127.0.0.1:8080");
  assert.equal(client.headers().Authorization, "Bearer token-123");
  assert.equal(client.headers()["X-Actor-Agent"], "codex");
});
