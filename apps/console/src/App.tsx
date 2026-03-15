import { useEffect, useState, type CSSProperties, type FormEvent } from "react";
import { EvoClient, type ApprovalRequest, type AuthSession, type Runbook, type TaskRun, type Workspace } from "@evo/sdk";

const AUTH_STORAGE_KEY = "evo.console.auth";

function loadStoredSession(): AuthSession | null {
  try {
    const raw = window.localStorage.getItem(AUTH_STORAGE_KEY);
    return raw ? (JSON.parse(raw) as AuthSession) : null;
  } catch {
    return null;
  }
}

function saveStoredSession(session: AuthSession | null): void {
  if (!session) {
    window.localStorage.removeItem(AUTH_STORAGE_KEY);
    return;
  }
  window.localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(session));
}

function clientForToken(accessToken?: string): EvoClient {
  return new EvoClient({
    accessToken,
    actorSurface: "console",
    actorAgent: "human"
  });
}

export function App() {
  const [session, setSession] = useState<AuthSession | null>(() => loadStoredSession());
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [runbooks, setRunbooks] = useState<Runbook[]>([]);
  const [runs, setRuns] = useState<TaskRun[]>([]);
  const [approvals, setApprovals] = useState<ApprovalRequest[]>([]);
  const [selectedWorkspace, setSelectedWorkspace] = useState<string>("");
  const [email, setEmail] = useState("admin@evo.local");
  const [password, setPassword] = useState("changeme");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!session?.access_token) return;
    const client = clientForToken(session.access_token);
    void Promise.all([client.listWorkspaces(), client.listRunbooks()]).then(([nextWorkspaces, nextRunbooks]) => {
      setWorkspaces(nextWorkspaces);
      setRunbooks(nextRunbooks);
      if (!selectedWorkspace && nextWorkspaces[0]) {
        setSelectedWorkspace(nextWorkspaces[0].id);
      }
    }).catch((nextError: Error) => {
      setError(nextError.message);
    });
  }, [session?.access_token, selectedWorkspace]);

  useEffect(() => {
    if (!session?.access_token || !selectedWorkspace) return;
    const client = clientForToken(session.access_token);
    void Promise.all([client.listRuns(selectedWorkspace), client.listApprovals(selectedWorkspace)]).then(([nextRuns, nextApprovals]) => {
      setRuns(nextRuns);
      setApprovals(nextApprovals);
    }).catch((nextError: Error) => {
      setError(nextError.message);
    });
  }, [session?.access_token, selectedWorkspace]);

  async function handleLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoading(true);
    setError("");
    try {
      const nextSession = await clientForToken().login({ email, password, label: "console" });
      saveStoredSession(nextSession);
      setSession(nextSession);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : String(nextError));
    } finally {
      setLoading(false);
    }
  }

  async function createDemoWorkspace() {
    if (!session?.access_token) return;
    const client = clientForToken(session.access_token);
    const created = await client.createWorkspace({
      name: "Ops Workspace",
      slug: `ops-${Date.now()}`,
      description: "Default operations workspace"
    });
    setWorkspaces((current) => [...current, created]);
    setSelectedWorkspace(created.id);
  }

  async function runRelease(runbookSlug: string) {
    if (!session?.access_token || !selectedWorkspace) return;
    const client = clientForToken(session.access_token);
    await client.createRun({
      workspace_id: selectedWorkspace,
      runbook_slug: runbookSlug,
      context: { initiated_from: "console" }
    });
    setRuns(await client.listRuns(selectedWorkspace));
  }

  async function approve(approvalId: string) {
    if (!session?.access_token || !selectedWorkspace) return;
    const client = clientForToken(session.access_token);
    await client.approve(approvalId, "Approved from console");
    setApprovals(await client.listApprovals(selectedWorkspace));
    setRuns(await client.listRuns(selectedWorkspace));
  }

  function logout() {
    saveStoredSession(null);
    setSession(null);
    setWorkspaces([]);
    setRunbooks([]);
    setRuns([]);
    setApprovals([]);
    setSelectedWorkspace("");
  }

  if (!session?.access_token) {
    return (
      <main style={loginShell}>
        <section style={loginPanel}>
          <p style={eyebrow}>AI-native control tower</p>
          <h1 style={title}>Sign in to Evo</h1>
          <p style={subtitle}>The console is now backed by the same bearer-authenticated API that powers CLI, MCP, and agent clients.</p>
          <form onSubmit={handleLogin} style={{ display: "grid", gap: "0.9rem" }}>
            <label style={field}>
              <span>Email</span>
              <input value={email} onChange={(event) => setEmail(event.target.value)} style={input} />
            </label>
            <label style={field}>
              <span>Password</span>
              <input type="password" value={password} onChange={(event) => setPassword(event.target.value)} style={input} />
            </label>
            <button disabled={loading} style={primaryButton} type="submit">
              {loading ? "Signing In..." : "Sign In"}
            </button>
          </form>
          {error ? <p style={errorText}>{error}</p> : null}
          <p style={hint}>Default local bootstrap credentials: <code>admin@evo.local</code> / <code>changeme</code></p>
        </section>
      </main>
    );
  }

  return (
    <main style={shell}>
      <header style={header}>
        <div>
          <p style={eyebrow}>Authenticated control tower</p>
          <h1 style={{ margin: 0 }}>Evo Operations Console</h1>
          <p style={subtitle}>Signed in as {session.user.display_name} in {session.org.name}. Token preview: {session.token.token_preview}</p>
        </div>
        <div style={{ display: "flex", gap: "0.75rem" }}>
          <button onClick={createDemoWorkspace} style={secondaryButton}>Create Demo Workspace</button>
          <button onClick={logout} style={secondaryButton}>Sign Out</button>
        </div>
      </header>

      {error ? <p style={errorText}>{error}</p> : null}

      <section style={grid}>
        <article style={card}>
          <h2>Workspaces</h2>
          {workspaces.length === 0 ? <p>No workspaces yet.</p> : null}
          <ul style={list}>
            {workspaces.map((workspace) => (
              <li key={workspace.id}>
                <button onClick={() => setSelectedWorkspace(workspace.id)} style={workspace.id === selectedWorkspace ? selectedButton : textButton}>
                  {workspace.name}
                </button>
              </li>
            ))}
          </ul>
        </article>

        <article style={card}>
          <h2>Runbook Catalog</h2>
          <ul style={list}>
            {runbooks.map((runbook) => (
              <li key={runbook.slug} style={{ marginBottom: "0.9rem" }}>
                <strong>{runbook.title}</strong>
                <div style={{ color: "#4b5563", margin: "0.35rem 0" }}>{runbook.description}</div>
                <button disabled={!selectedWorkspace} onClick={() => runRelease(runbook.slug)} style={primaryButton}>
                  Launch
                </button>
              </li>
            ))}
          </ul>
        </article>

        <article style={card}>
          <h2>Recent Runs</h2>
          <ul style={list}>
            {runs.map((run) => (
              <li key={run.id}>
                <code>{run.runbook_slug}</code> · {run.status}
              </li>
            ))}
          </ul>
        </article>

        <article style={card}>
          <h2>Approvals Inbox</h2>
          <ul style={list}>
            {approvals.map((approval) => (
              <li key={approval.id} style={{ marginBottom: "0.75rem" }}>
                <div>{approval.reason}</div>
                <div style={{ color: "#6b7280", margin: "0.3rem 0" }}>{approval.status}</div>
                {approval.status === "pending" ? (
                  <button onClick={() => approve(approval.id)} style={primaryButton}>Approve</button>
                ) : null}
              </li>
            ))}
          </ul>
        </article>
      </section>
    </main>
  );
}

const shell: CSSProperties = {
  fontFamily: "\"IBM Plex Sans\", \"Segoe UI\", sans-serif",
  margin: "0 auto",
  minHeight: "100vh",
  padding: "2rem",
  color: "#111827",
  background: "linear-gradient(180deg, #f8fafc 0%, #e0f2fe 100%)"
};

const loginShell: CSSProperties = {
  ...shell,
  display: "grid",
  placeItems: "center"
};

const loginPanel: CSSProperties = {
  width: "min(440px, 100%)",
  padding: "2rem",
  borderRadius: "1.5rem",
  background: "rgba(255,255,255,0.88)",
  border: "1px solid rgba(15, 23, 42, 0.08)",
  boxShadow: "0 25px 60px rgba(15, 23, 42, 0.12)"
};

const header: CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  gap: "1rem",
  marginBottom: "1.5rem"
};

const grid: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))",
  gap: "1rem"
};

const card: CSSProperties = {
  padding: "1.2rem",
  borderRadius: "1.25rem",
  background: "rgba(255,255,255,0.86)",
  border: "1px solid rgba(148, 163, 184, 0.35)",
  boxShadow: "0 18px 40px rgba(15, 23, 42, 0.08)"
};

const list: CSSProperties = {
  listStyle: "none",
  padding: 0,
  margin: "0.75rem 0 0"
};

const field: CSSProperties = {
  display: "grid",
  gap: "0.35rem",
  fontSize: "0.95rem"
};

const input: CSSProperties = {
  padding: "0.75rem 0.9rem",
  borderRadius: "0.8rem",
  border: "1px solid #cbd5e1",
  fontSize: "1rem"
};

const primaryButton: CSSProperties = {
  border: 0,
  borderRadius: "999px",
  background: "#0f172a",
  color: "white",
  padding: "0.7rem 1rem",
  cursor: "pointer"
};

const secondaryButton: CSSProperties = {
  ...primaryButton,
  background: "white",
  color: "#0f172a",
  border: "1px solid #cbd5e1"
};

const textButton: CSSProperties = {
  border: 0,
  background: "transparent",
  padding: 0,
  color: "#0f172a",
  cursor: "pointer"
};

const selectedButton: CSSProperties = {
  ...textButton,
  fontWeight: 700
};

const eyebrow: CSSProperties = {
  margin: 0,
  textTransform: "uppercase",
  letterSpacing: "0.12em",
  fontSize: "0.72rem",
  color: "#0f766e"
};

const title: CSSProperties = {
  margin: "0.35rem 0 0.75rem",
  fontSize: "2rem"
};

const subtitle: CSSProperties = {
  margin: "0.35rem 0 0",
  color: "#475569",
  maxWidth: "52rem"
};

const hint: CSSProperties = {
  marginTop: "1rem",
  color: "#64748b",
  fontSize: "0.9rem"
};

const errorText: CSSProperties = {
  color: "#b91c1c"
};
