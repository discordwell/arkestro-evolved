import test from "node:test";
import assert from "node:assert/strict";
import { once } from "node:events";
import { EvoApiError, type EvoClient, type RunEnvelope } from "@evo/sdk";
import { bearerToken, createApp } from "./server.js";

// Run app on an ephemeral port and read the response before the server closes,
// mirroring the connect test below.
async function withServer<T>(
  app: ReturnType<typeof createApp>,
  fn: (baseUrl: string) => Promise<T>
): Promise<T> {
  const server = app.listen(0);
  await once(server, "listening");
  try {
    const address = server.address();
    if (!address || typeof address === "string") {
      throw new Error("unexpected address");
    }
    return await fn(`http://127.0.0.1:${address.port}`);
  } finally {
    server.close();
  }
}

// Build an app whose upstream client is a stub, so proxy behavior can be tested
// without a live control plane. Any token/surface yields the same fake.
function appWithClient(fake: Partial<EvoClient>): ReturnType<typeof createApp> {
  return createApp(() => fake as unknown as EvoClient);
}

const AUTH = { authorization: "Bearer test-token" };

test("bearer token parser extracts bearer credentials", () => {
  assert.equal(bearerToken("Bearer token-123"), "token-123");
  assert.equal(bearerToken("Basic token-123"), "");
});

test("connect endpoint advertises remote MCP metadata", async () => {
  await withServer(createApp(), async (baseUrl) => {
    const response = await fetch(`${baseUrl}/api/connect`);
    assert.equal(response.status, 200);
    const body = (await response.json()) as { transport: string; mcp_url: string };
    assert.equal(body.transport, "streamable_http");
    assert.match(body.mcp_url, /^http/);
  });
});

test("upstream API status is preserved instead of collapsing to 500", async () => {
  const app = appWithClient({
    me: async () => {
      throw new EvoApiError(401, "Unauthorized", "unauthorized", { error: "unauthorized" });
    }
  });
  await withServer(app, async (baseUrl) => {
    const response = await fetch(`${baseUrl}/api/me`, { headers: AUTH });
    assert.equal(response.status, 401);
    assert.deepEqual(await response.json(), { error: "401 Unauthorized: unauthorized" });
  });
});

test("a missing run surfaces as 404", async () => {
  const app = appWithClient({
    getRun: async () => {
      throw new EvoApiError(404, "Not Found", "not found", { error: "not found" });
    }
  });
  await withServer(app, async (baseUrl) => {
    const response = await fetch(`${baseUrl}/api/runs/does-not-exist`, { headers: AUTH });
    assert.equal(response.status, 404);
  });
});

test("non-API errors still map to 500", async () => {
  const app = appWithClient({
    me: async () => {
      throw new Error("boom");
    }
  });
  await withServer(app, async (baseUrl) => {
    const response = await fetch(`${baseUrl}/api/me`, { headers: AUTH });
    assert.equal(response.status, 500);
    assert.deepEqual(await response.json(), { error: "boom" });
  });
});

test("login failures surface the upstream status, not 500", async () => {
  const app = appWithClient({
    login: async () => {
      throw new EvoApiError(401, "Unauthorized", "invalid credentials", { error: "invalid credentials" });
    }
  });
  await withServer(app, async (baseUrl) => {
    const response = await fetch(`${baseUrl}/api/login`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ email: "admin@evo.local", password: "wrong" })
    });
    assert.equal(response.status, 401);
    assert.deepEqual(await response.json(), { error: "401 Unauthorized: invalid credentials" });
  });
});

test("approval routes dispatch approve vs reject with the note", async () => {
  const calls: Array<{ method: string; id: string; note: string }> = [];
  const record = (method: string) => async (id: string, note = ""): Promise<RunEnvelope> => {
    calls.push({ method, id, note });
    return { run: { status: "queued" } } as unknown as RunEnvelope;
  };
  const app = appWithClient({ approve: record("approve"), reject: record("reject") });
  await withServer(app, async (baseUrl) => {
    const approve = await fetch(`${baseUrl}/api/approvals/a1/approve`, {
      method: "POST",
      headers: { ...AUTH, "content-type": "application/json" },
      body: JSON.stringify({ note: "ship it" })
    });
    assert.equal(approve.status, 200);
    const reject = await fetch(`${baseUrl}/api/approvals/a2/reject`, {
      method: "POST",
      headers: { ...AUTH, "content-type": "application/json" },
      body: JSON.stringify({ note: "hold" })
    });
    assert.equal(reject.status, 200);
  });
  assert.deepEqual(calls, [
    { method: "approve", id: "a1", note: "ship it" },
    { method: "reject", id: "a2", note: "hold" }
  ]);
});

test("an unknown approval decision is rejected without touching the control plane", async () => {
  // A typo'd or malformed decision must not silently fall through to reject and
  // terminate the run: it has to fail fast, before any upstream call.
  const calls: string[] = [];
  const record = (method: string) => async (): Promise<RunEnvelope> => {
    calls.push(method);
    return { run: { status: "queued" } } as unknown as RunEnvelope;
  };
  const app = appWithClient({ approve: record("approve"), reject: record("reject") });
  await withServer(app, async (baseUrl) => {
    const response = await fetch(`${baseUrl}/api/approvals/a1/approvve`, {
      method: "POST",
      headers: { ...AUTH, "content-type": "application/json" },
      body: JSON.stringify({ note: "meant to approve" })
    });
    assert.equal(response.status, 400);
    assert.deepEqual(await response.json(), { error: "decision must be approve or reject" });
  });
  assert.deepEqual(calls, []);
});

test("requests without a bearer token are rejected when no service token is set", async () => {
  const savedApp = process.env.EVO_CHATGPT_APP_TOKEN;
  const savedApi = process.env.EVO_API_TOKEN;
  delete process.env.EVO_CHATGPT_APP_TOKEN;
  delete process.env.EVO_API_TOKEN;
  try {
    const app = appWithClient({
      me: async () => {
        throw new Error("handler should not run");
      }
    });
    await withServer(app, async (baseUrl) => {
      const response = await fetch(`${baseUrl}/api/me`);
      assert.equal(response.status, 401);
      assert.deepEqual(await response.json(), { error: "missing bearer token" });
    });
  } finally {
    if (savedApp !== undefined) process.env.EVO_CHATGPT_APP_TOKEN = savedApp;
    if (savedApi !== undefined) process.env.EVO_API_TOKEN = savedApi;
  }
});
