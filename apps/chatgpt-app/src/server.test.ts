import test from "node:test";
import assert from "node:assert/strict";
import { once } from "node:events";
import { bearerToken, createApp } from "./server.js";

test("bearer token parser extracts bearer credentials", () => {
  assert.equal(bearerToken("Bearer token-123"), "token-123");
  assert.equal(bearerToken("Basic token-123"), "");
});

test("connect endpoint advertises remote MCP metadata", async () => {
  const app = createApp();
  const server = app.listen(0);
  await once(server, "listening");

  try {
    const address = server.address();
    if (!address || typeof address === "string") {
      throw new Error("unexpected address");
    }
    const response = await fetch(`http://127.0.0.1:${address.port}/api/connect`);
    assert.equal(response.status, 200);
    const body = (await response.json()) as { transport: string; mcp_url: string };
    assert.equal(body.transport, "streamable_http");
    assert.match(body.mcp_url, /^http/);
  } finally {
    server.close();
  }
});
