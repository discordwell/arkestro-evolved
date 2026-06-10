import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { createMcpExpressApp } from "@modelcontextprotocol/sdk/server/express.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { CallToolRequestSchema, ListToolsRequestSchema } from "@modelcontextprotocol/sdk/types.js";
import { EvoClient } from "@evo/sdk";

export const toolDefinitions = [
  { name: "workspace.list", description: "List workspaces", inputSchema: { type: "object", properties: {} } },
  { name: "environment.list", description: "List environments for a workspace", inputSchema: { type: "object", required: ["workspace_id"], properties: { workspace_id: { type: "string" } } } },
  { name: "tool.list", description: "List tool connections for a workspace", inputSchema: { type: "object", required: ["workspace_id"], properties: { workspace_id: { type: "string" } } } },
  { name: "runbook.list", description: "List runbooks", inputSchema: { type: "object", properties: {} } },
  { name: "runbook.run", description: "Create a run from a runbook", inputSchema: { type: "object", required: ["workspace_id", "runbook_slug"], properties: { workspace_id: { type: "string" }, environment_id: { type: "string" }, runbook_slug: { type: "string" }, context: { type: "object" } } } },
  { name: "run.get", description: "Get a run envelope", inputSchema: { type: "object", required: ["run_id"], properties: { run_id: { type: "string" } } } },
  { name: "run.stream", description: "Poll a run until it completes, fails, is rejected, needs approval, or the timeout elapses; returns the latest run envelope", inputSchema: { type: "object", required: ["run_id"], properties: { run_id: { type: "string" }, interval_ms: { type: "number" }, timeout_ms: { type: "number", description: "Maximum time to poll in milliseconds (default 120000; 0 checks once and returns)" } } } },
  { name: "artifact.list", description: "List artifacts for a run", inputSchema: { type: "object", required: ["run_id"], properties: { run_id: { type: "string" } } } },
  { name: "artifact.get", description: "Get artifact content", inputSchema: { type: "object", required: ["artifact_id"], properties: { artifact_id: { type: "string" } } } },
  { name: "approval.list", description: "List approvals for a workspace", inputSchema: { type: "object", required: ["workspace_id"], properties: { workspace_id: { type: "string" } } } },
  { name: "approval.approve", description: "Approve a pending request", inputSchema: { type: "object", required: ["approval_id"], properties: { approval_id: { type: "string" }, note: { type: "string" } } } },
  { name: "approval.reject", description: "Reject a pending request", inputSchema: { type: "object", required: ["approval_id"], properties: { approval_id: { type: "string" }, note: { type: "string" } } } },
  { name: "audit.query", description: "Query audit events", inputSchema: { type: "object", required: ["workspace_id"], properties: { workspace_id: { type: "string" }, run_id: { type: "string" } } } }
] as const;

type TransportMode = "stdio" | "http";

// Statuses where polling should hand control back to the caller: terminal
// states plus awaiting_approval, which needs an approve/reject decision
// before the run can make further progress.
const SETTLED_RUN_STATUSES = new Set(["completed", "failed", "rejected", "awaiting_approval"]);

export interface PollRunOptions {
  intervalMs?: number;
  timeoutMs?: number;
  sleep?: (ms: number) => Promise<void>;
}

const defaultSleep = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));

function numberOrFallback(value: unknown, fallback: number, minimum: number): number {
  const num = Number(value);
  return Number.isFinite(num) && num >= minimum ? num : fallback;
}

export async function pollRunUntilSettled<T extends { run: { status?: string } }>(
  getRun: () => Promise<T>,
  { intervalMs = 1000, timeoutMs = 120_000, sleep = defaultSleep }: PollRunOptions = {}
): Promise<T> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const envelope = await getRun();
    if (SETTLED_RUN_STATUSES.has(envelope.run.status ?? "") || Date.now() >= deadline) {
      return envelope;
    }
    await sleep(Math.min(intervalMs, Math.max(deadline - Date.now(), 0)));
  }
}

export function transportModeFromArgs(argv: string[], env: NodeJS.ProcessEnv = process.env): TransportMode {
  if (env.EVO_MCP_TRANSPORT === "http" || argv.includes("--transport=http") || argv.includes("http")) {
    return "http";
  }
  return "stdio";
}

export async function executeTool(client: EvoClient, name: string, args: Record<string, unknown>): Promise<unknown> {
  switch (name) {
    case "workspace.list":
      return client.listWorkspaces();
    case "environment.list":
      return client.listEnvironments(String(args.workspace_id));
    case "tool.list":
      return client.listToolConnections(String(args.workspace_id));
    case "runbook.list":
      return client.listRunbooks();
    case "runbook.run":
      return client.createRun({
        workspace_id: String(args.workspace_id),
        environment_id: args.environment_id ? String(args.environment_id) : undefined,
        runbook_slug: String(args.runbook_slug),
        context: (args.context as Record<string, unknown> | undefined) || {}
      });
    case "run.get":
      return client.getRun(String(args.run_id));
    case "run.stream":
      return pollRunUntilSettled(() => client.getRun(String(args.run_id)), {
        intervalMs: numberOrFallback(args.interval_ms, 1000, 1),
        timeoutMs: numberOrFallback(args.timeout_ms, 120_000, 0)
      });
    case "artifact.list":
      return client.listArtifacts(String(args.run_id));
    case "artifact.get":
      return client.getArtifact(String(args.artifact_id));
    case "approval.list":
      return client.listApprovals(String(args.workspace_id));
    case "approval.approve":
      return client.approve(String(args.approval_id), String(args.note || ""));
    case "approval.reject":
      return client.reject(String(args.approval_id), String(args.note || ""));
    case "audit.query":
      return client.listAuditEvents(String(args.workspace_id), args.run_id ? String(args.run_id) : "");
    default:
      throw new Error(`Unknown tool: ${name}`);
  }
}

export function bearerToken(header: string | string[] | undefined): string {
  const raw = Array.isArray(header) ? header[0] : header;
  if (!raw) return "";
  const [scheme, token] = raw.split(" ", 2);
  if (!scheme || !token || scheme.toLowerCase() !== "bearer") {
    return "";
  }
  return token.trim();
}

function clientForRequest(accessToken: string | undefined, surface: string): EvoClient {
  return new EvoClient({
    accessToken: accessToken || process.env.EVO_API_TOKEN,
    actorSurface: surface,
    actorAgent: process.env.EVO_ACTOR_AGENT || "codex"
  });
}

export function createRpcServer(client: EvoClient): Server {
  const server = new Server(
    { name: "evo-control-plane", version: "0.2.0" },
    { capabilities: { tools: {} } }
  );

  server.setRequestHandler(ListToolsRequestSchema, async () => ({ tools: [...toolDefinitions] }));

  server.setRequestHandler(CallToolRequestSchema, async (request) => {
    const args = (request.params.arguments ?? {}) as Record<string, unknown>;
    const result = await executeTool(client, request.params.name, args);
    return {
      content: [
        {
          type: "text",
          text: JSON.stringify(result, null, 2)
        }
      ]
    };
  });

  return server;
}

export async function runStdio(): Promise<void> {
  const server = createRpcServer(clientForRequest(process.env.EVO_API_TOKEN, "mcp-stdio"));
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

export async function runHttp(): Promise<void> {
  const app = createMcpExpressApp({ host: process.env.EVO_MCP_HOST || "127.0.0.1" });
  const port = Number(process.env.PORT || 3301);

  app.get("/health", (_req, res) => {
    res.json({ ok: true, transport: "streamable_http" });
  });

  app.post("/mcp", async (req, res) => {
    const accessToken = bearerToken(req.headers.authorization);
    if (!accessToken && !process.env.EVO_API_TOKEN) {
      res.status(401).json({
        jsonrpc: "2.0",
        error: { code: -32001, message: "Missing bearer token" },
        id: null
      });
      return;
    }

    const server = createRpcServer(clientForRequest(accessToken, "mcp-http"));
    const transport = new StreamableHTTPServerTransport({ sessionIdGenerator: undefined });

    try {
      await server.connect(transport);
      await transport.handleRequest(req, res, req.body);
      res.on("close", () => {
        void transport.close();
        void server.close();
      });
    } catch (error) {
      if (!res.headersSent) {
        res.status(500).json({
          jsonrpc: "2.0",
          error: { code: -32603, message: error instanceof Error ? error.message : "Internal server error" },
          id: null
        });
      }
    }
  });

  app.get("/mcp", (_req, res) => {
    res.status(405).json({
      jsonrpc: "2.0",
      error: { code: -32000, message: "Method not allowed. Use POST /mcp." },
      id: null
    });
  });

  app.delete("/mcp", (_req, res) => {
    res.status(405).json({
      jsonrpc: "2.0",
      error: { code: -32000, message: "Method not allowed. Use POST /mcp." },
      id: null
    });
  });

  app.listen(port, () => {
    console.log(`mcp http listening on http://127.0.0.1:${port}/mcp`);
  });
}

async function main(): Promise<void> {
  const mode = transportModeFromArgs(process.argv.slice(2));
  if (mode === "http") {
    await runHttp();
    return;
  }
  await runStdio();
}

if (process.env.NODE_ENV !== "test") {
  void main().catch((error) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  });
}
