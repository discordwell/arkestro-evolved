#!/usr/bin/env node
import { Command } from "commander";
import { EvoClient, type ArtifactDocument, type RunEnvelope } from "@evo/sdk";
import {
  authFilePath,
  clearStoredAuth,
  parseContext,
  parseIntervalMs,
  readStoredAuth,
  resolveClientOptions,
  watchRunLoop,
  writeStoredAuth
} from "./lib.js";

function output(value: unknown, asJSON: boolean): void {
  if (asJSON) {
    console.log(JSON.stringify(value, null, 2));
    return;
  }
  console.dir(value, { depth: null, colors: true });
}

async function createClient(): Promise<EvoClient> {
  const stored = await readStoredAuth(authFilePath());
  return new EvoClient(resolveClientOptions(process.env, stored));
}

async function watchRun(client: EvoClient, runId: string, intervalMs: number, asJSON: boolean): Promise<void> {
  await watchRunLoop<RunEnvelope>({
    getRun: () => client.getRun(runId),
    emit: (run) => output(run, asJSON),
    intervalMs
  });
}

const program = new Command();
program.name("evo").description("Evo Control Plane CLI");

const auth = program.command("auth").description("Authentication commands");
auth
  .command("status")
  .option("--json", "machine-readable output")
  .action(async (options) => {
    const stored = await readStoredAuth(authFilePath());
    const client = await createClient();
    output({
      baseUrl: client.baseUrl,
      authenticated: Boolean(client.accessToken),
      tokenSource: process.env.EVO_API_TOKEN ? "env" : stored?.accessToken ? "file" : "none",
      authFile: authFilePath(),
      userEmail: stored?.userEmail,
      tokenPreview: stored?.tokenPreview
    }, options.json);
  });
auth
  .command("login")
  .requiredOption("--email <email>")
  .requiredOption("--password <password>")
  .option("--label <label>", "token label", "cli")
  .option("--json", "machine-readable output")
  .action(async (options) => {
    const client = await createClient();
    const session = await client.login({
      email: options.email,
      password: options.password,
      label: options.label
    });
    await writeStoredAuth(authFilePath(), {
      baseUrl: client.baseUrl,
      accessToken: session.access_token,
      userEmail: session.user.email,
      tokenPreview: session.token.token_preview
    });
    output(session, options.json);
  });
auth
  .command("whoami")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).me(), options.json));
auth
  .command("token")
  .option("--label <label>", "token label", "cli")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).createToken({ label: options.label }), options.json));
auth
  .command("logout")
  .option("--json", "machine-readable output")
  .action(async (options) => {
    await clearStoredAuth(authFilePath());
    output({ ok: true, authFile: authFilePath() }, options.json);
  });

const workspace = program.command("workspace").description("Workspace commands");
workspace
  .command("list")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).listWorkspaces(), options.json));
workspace
  .command("create")
  .requiredOption("--name <name>")
  .requiredOption("--slug <slug>")
  .option("--description <description>", "")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).createWorkspace(options), options.json));

const env = program.command("env").description("Environment commands");
env
  .command("list")
  .requiredOption("--workspace-id <workspaceId>")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).listEnvironments(options.workspaceId), options.json));
env
  .command("create")
  .requiredOption("--workspace-id <workspaceId>")
  .requiredOption("--name <name>")
  .requiredOption("--slug <slug>")
  .requiredOption("--kind <kind>")
  .option("--json", "machine-readable output")
  .action(async (options) => output(
    await (await createClient()).createEnvironment({
      workspace_id: options.workspaceId,
      name: options.name,
      slug: options.slug,
      kind: options.kind
    }),
    options.json
  ));

const tool = program.command("tool").description("Tool connection commands");
tool
  .command("list")
  .requiredOption("--workspace-id <workspaceId>")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).listToolConnections(options.workspaceId), options.json));
tool
  .command("create")
  .requiredOption("--workspace-id <workspaceId>")
  .requiredOption("--name <name>")
  .requiredOption("--kind <kind>")
  .option("--environment-id <environmentId>")
  .option("--config <entry...>", "key=value pairs", [])
  .option("--json", "machine-readable output")
  .action(async (options) => output(
    await (await createClient()).createToolConnection({
      workspace_id: options.workspaceId,
      environment_id: options.environmentId,
      name: options.name,
      kind: options.kind,
      config: parseContext(options.config)
    }),
    options.json
  ));

const runbook = program.command("runbook").description("Runbook commands");
runbook
  .command("list")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).listRunbooks(), options.json));

const run = program.command("run").description("Run commands");
run
  .command("list")
  .requiredOption("--workspace-id <workspaceId>")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).listRuns(options.workspaceId), options.json));
run
  .command("create")
  .requiredOption("--workspace-id <workspaceId>")
  .requiredOption("--runbook-slug <runbookSlug>")
  .option("--environment-id <environmentId>")
  .option("--context <entry...>", "key=value pairs", [])
  .option("--json", "machine-readable output")
  .action(async (options) => output(
    await (await createClient()).createRun({
      workspace_id: options.workspaceId,
      environment_id: options.environmentId,
      runbook_slug: options.runbookSlug,
      context: parseContext(options.context)
    }),
    options.json
  ));
run
  .command("get")
  .argument("<runId>")
  .option("--json", "machine-readable output")
  .action(async (runId, options) => output(await (await createClient()).getRun(runId), options.json));
run
  .command("watch")
  .argument("<runId>")
  .option("--interval-ms <ms>", "poll interval", "1000")
  .option("--json", "machine-readable output")
  .action(async (runId, options) => watchRun(await createClient(), runId, parseIntervalMs(options.intervalMs), options.json));

const artifact = program.command("artifact").description("Artifact commands");
artifact
  .command("list")
  .requiredOption("--run-id <runId>")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).listArtifacts(options.runId), options.json));
artifact
  .command("get")
  .argument("<artifactId>")
  .option("--json", "machine-readable output")
  .action(async (artifactId: string, options: { json?: boolean }) => {
    const item = (await (await createClient()).getArtifact(artifactId)) as ArtifactDocument;
    output(item, Boolean(options.json));
  });

const approval = program.command("approval").description("Approval commands");
approval
  .command("list")
  .requiredOption("--workspace-id <workspaceId>")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).listApprovals(options.workspaceId), options.json));
approval
  .command("approve")
  .argument("<approvalId>")
  .option("--note <note>", "")
  .option("--json", "machine-readable output")
  .action(async (approvalId: string, options: { note?: string; json?: boolean }) => {
    const result = (await (await createClient()).approve(approvalId, options.note || "")) as RunEnvelope;
    output(result, Boolean(options.json));
  });
approval
  .command("reject")
  .argument("<approvalId>")
  .option("--note <note>", "")
  .option("--json", "machine-readable output")
  .action(async (approvalId: string, options: { note?: string; json?: boolean }) => {
    const result = (await (await createClient()).reject(approvalId, options.note || "")) as RunEnvelope;
    output(result, Boolean(options.json));
  });

const audit = program.command("audit").description("Audit commands");
audit
  .command("list")
  .requiredOption("--workspace-id <workspaceId>")
  .option("--run-id <runId>")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).listAuditEvents(options.workspaceId, options.runId || ""), options.json));

const docs = program.command("docs").description("Docs commands");
docs
  .command("runbooks")
  .option("--json", "machine-readable output")
  .action(async (options) => output(await (await createClient()).listRunbooks(), options.json));

const mcp = program.command("mcp").description("MCP helper commands");
mcp
  .command("print-config")
  .option("--json", "machine-readable output")
  .action(async (options) => {
    const stored = await readStoredAuth(authFilePath());
    const baseUrl = process.env.EVO_API_BASE_URL || stored?.baseUrl || "http://127.0.0.1:8080";
    const accessToken = process.env.EVO_API_TOKEN || stored?.accessToken || "";
    output({
      stdio: {
        command: "pnpm",
        args: ["--filter", "@evo/mcp", "dev"],
        env: { EVO_API_BASE_URL: baseUrl, EVO_API_TOKEN: accessToken }
      },
      http: {
        url: process.env.EVO_MCP_HTTP_URL || "http://127.0.0.1:3301/mcp",
        headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : {}
      }
    }, options.json);
  });

program.parseAsync(process.argv).catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});
