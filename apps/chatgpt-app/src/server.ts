import express from "express";
import { EvoClient } from "@evo/sdk";

const DEFAULT_MCP_URL = process.env.EVO_MCP_HTTP_URL || "http://127.0.0.1:3301/mcp";

export function bearerToken(header: string | undefined): string {
  if (!header) return "";
  const [scheme, token] = header.split(" ", 2);
  if (!scheme || !token || scheme.toLowerCase() !== "bearer") {
    return "";
  }
  return token.trim();
}

function clientForToken(accessToken?: string, surface = "chatgpt-app"): EvoClient {
  return new EvoClient({
    accessToken: accessToken || process.env.EVO_CHATGPT_APP_TOKEN || process.env.EVO_API_TOKEN,
    actorSurface: surface,
    actorAgent: "chatgpt"
  });
}

function requireAuth(req: express.Request, res: express.Response, next: express.NextFunction): void {
  if (bearerToken(req.header("authorization")) || process.env.EVO_CHATGPT_APP_TOKEN || process.env.EVO_API_TOKEN) {
    next();
    return;
  }
  res.status(401).json({ error: "missing bearer token" });
}

export function createApp(): express.Express {
  const app = express();

  app.use(express.json());

  app.get("/health", (_req, res) => {
    res.json({ ok: true, transport: "companion" });
  });

  app.get("/api/connect", (_req, res) => {
    res.json({
      mcp_url: DEFAULT_MCP_URL,
      transport: "streamable_http",
      auth: "bearer",
      control_plane_url: process.env.EVO_API_BASE_URL || "http://127.0.0.1:8080"
    });
  });

  app.post("/api/login", async (req, res, next) => {
    try {
      const session = await clientForToken(undefined, "chatgpt-app-login").login(req.body);
      res.json(session);
    } catch (error) {
      next(error);
    }
  });

  app.use("/api", requireAuth);

  app.get("/api/me", async (req, res, next) => {
    try {
      res.json(await clientForToken(bearerToken(req.header("authorization")), "chatgpt-app").me());
    } catch (error) {
      next(error);
    }
  });

  app.get("/api/overview", async (req, res, next) => {
    try {
      const client = clientForToken(bearerToken(req.header("authorization")), "chatgpt-app");
      const [identity, workspaces, runbooks] = await Promise.all([client.me(), client.listWorkspaces(), client.listRunbooks()]);
      res.json({ identity, workspaces, runbooks });
    } catch (error) {
      next(error);
    }
  });

  app.post("/api/runs", async (req, res, next) => {
    try {
      res.json(await clientForToken(bearerToken(req.header("authorization")), "chatgpt-app").createRun(req.body));
    } catch (error) {
      next(error);
    }
  });

  app.get("/api/runs/:id", async (req, res, next) => {
    try {
      res.json(await clientForToken(bearerToken(req.header("authorization")), "chatgpt-app").getRun(req.params.id));
    } catch (error) {
      next(error);
    }
  });

  app.post("/api/approvals/:id/:decision", async (req, res, next) => {
    try {
      const client = clientForToken(bearerToken(req.header("authorization")), "chatgpt-app");
      const fn = req.params.decision === "approve" ? client.approve.bind(client) : client.reject.bind(client);
      res.json(await fn(req.params.id, req.body?.note || ""));
    } catch (error) {
      next(error);
    }
  });

  app.get("/", (_req, res) => {
    res.type("html").send(`<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Evo ChatGPT Companion</title>
    <style>
      body { font-family: "IBM Plex Sans", "Segoe UI", sans-serif; margin: 0; background: linear-gradient(180deg, #f8fafc 0%, #e0f2fe 100%); color: #111827; }
      main { max-width: 820px; margin: 0 auto; padding: 3rem 1.5rem; }
      section { background: rgba(255,255,255,0.88); border: 1px solid rgba(15,23,42,0.08); border-radius: 24px; padding: 1.5rem; box-shadow: 0 20px 48px rgba(15,23,42,0.08); }
      code { background: #eff6ff; padding: 0.2rem 0.4rem; border-radius: 0.4rem; }
      p { line-height: 1.5; }
    </style>
  </head>
  <body>
    <main>
      <section>
        <p style="text-transform: uppercase; letter-spacing: 0.12em; font-size: 0.75rem; color: #0f766e; margin: 0;">Companion surface</p>
        <h1 style="margin-bottom: 0.5rem;">Evo ChatGPT Companion</h1>
        <p>This host now exposes authenticated control-plane routes plus the remote MCP connection details ChatGPT-compatible clients need.</p>
        <p>Remote MCP endpoint: <code>${DEFAULT_MCP_URL}</code></p>
        <p>Use <code>POST /api/login</code> to obtain a bearer token, then call <code>/api/overview</code>, <code>/api/runs</code>, and <code>/api/approvals/:id/:decision</code> with <code>Authorization: Bearer ...</code>.</p>
        <p>Connection metadata lives at <code>/api/connect</code>.</p>
      </section>
    </main>
  </body>
</html>`);
  });

  app.use((error: Error, _req: express.Request, res: express.Response, _next: express.NextFunction) => {
    res.status(500).json({ error: error.message });
  });

  return app;
}

if (process.env.NODE_ENV !== "test") {
  const port = Number(process.env.PORT || 3200);
  createApp().listen(port, () => {
    console.log(`chatgpt companion listening on http://127.0.0.1:${port}`);
  });
}
