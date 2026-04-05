import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Plus, Settings, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { apiFetch, getAccessToken, wsUrl } from "../api/http";
import type { Project, UUID } from "../api/types";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal as XTerm } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";

type SshConnection = {
  id: UUID;
  organization_id: UUID;
  owner_user_id: UUID;
  name: string;
  host: string;
  port: number;
  username: string;
  auth_method: "key" | "password";
  has_password: boolean;
  has_key: boolean;
  created_at: string;
  updated_at: string;
};

type TermClientToServer =
  | { type: "input"; data: string }
  | { type: "resize"; cols: number; rows: number }
  | { type: "ping" }
  | { type: "auth"; method: "password"; password: string };

type TermServerToClient =
  | { type: "output"; data: string }
  | { type: "error"; message: string }
  | { type: "connected"; connection_id: UUID };

export default function SpaceTerminalPage() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();

  const projectQ = useQuery({
    queryKey: ["project", orgId, projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${projectId}`);
      if (!res.ok) throw new Error("project");
      return res.json() as Promise<Project>;
    },
  });

  const connectionsQ = useQuery({
    queryKey: ["ssh-connections", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/ssh-connections`);
      if (!res.ok) throw new Error("ssh-connections");
      const j = (await res.json()) as { connections: SshConnection[] };
      return j.connections ?? [];
    },
  });

  const [settingsOpen, setSettingsOpen] = useState(false);
  const [activeConnectionId, setActiveConnectionId] = useState<UUID | null>(null);
  const activeConnection = useMemo(
    () => (connectionsQ.data ?? []).find((c) => c.id === activeConnectionId) ?? null,
    [connectionsQ.data, activeConnectionId],
  );

  const termHostRef = useRef<HTMLDivElement | null>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [connState, setConnState] = useState<"disconnected" | "connecting" | "connected" | "error">(
    "disconnected",
  );
  const [connError, setConnError] = useState<string | null>(null);
  const [passwordPromptOpen, setPasswordPromptOpen] = useState(false);
  const [passwordDraft, setPasswordDraft] = useState("");

  function writeSystemLine(line: string) {
    const t = xtermRef.current;
    t?.writeln(`\x1b[90m${line}\x1b[0m`);
  }

  useEffect(() => {
    if (!termHostRef.current) return;
    if (xtermRef.current) return;

    const t = new XTerm({
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, \"Liberation Mono\", \"Courier New\", monospace",
      fontSize: 13,
      cursorBlink: true,
      convertEol: true,
      scrollback: 5000,
      theme: { background: "transparent" },
    });
    const fit = new FitAddon();
    t.loadAddon(fit);

    t.open(termHostRef.current);
    xtermRef.current = t;
    fitRef.current = fit;

    writeSystemLine("Not connected. Open Settings to create/select an SSH connection.");

    const ro = new ResizeObserver(() => {
      try {
        fit.fit();
        const cols = t.cols;
        const rows = t.rows;
        const msg: TermClientToServer = { type: "resize", cols, rows };
        wsRef.current?.send(JSON.stringify(msg));
      } catch {
        // ignore
      }
    });
    ro.observe(termHostRef.current);
    fit.fit();

    const dataDispose = t.onData((data) => {
      const msg: TermClientToServer = { type: "input", data };
      wsRef.current?.send(JSON.stringify(msg));
    });

    return () => {
      dataDispose.dispose();
      ro.disconnect();
      wsRef.current?.close();
      wsRef.current = null;
      t.dispose();
      xtermRef.current = null;
      fitRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [termHostRef.current]);

  function disconnect() {
    wsRef.current?.close();
    wsRef.current = null;
    setConnState("disconnected");
    setConnError(null);
    setPasswordPromptOpen(false);
    setPasswordDraft("");
    writeSystemLine("Disconnected.");
  }

  function connect(connectionId: UUID) {
    const token = getAccessToken();
    if (!orgId || !projectId) return;
    if (!token) {
      setConnState("error");
      setConnError("Not authenticated");
      writeSystemLine("Error: not authenticated.");
      return;
    }

    disconnect();
    setConnState("connecting");
    setConnError(null);
    xtermRef.current?.clear();
    writeSystemLine(`Connecting…`);

    const u = wsUrl(
      `/api/v1/organizations/${orgId}/spaces/${projectId}/terminal/ws?token=${encodeURIComponent(
        token,
      )}&connectionId=${encodeURIComponent(connectionId)}`,
    );
    const ws = new WebSocket(u);
    wsRef.current = ws;

    ws.onopen = () => {
      // Keep as "connecting" until server confirms SSH session.
      setConnState("connecting");
      setConnError(null);

      if (activeConnection?.auth_method === "password" && !activeConnection.has_password) {
        setPasswordDraft("");
        setPasswordPromptOpen(true);
      }
      try {
        fitRef.current?.fit();
        const t = xtermRef.current;
        if (t) {
          const msg: TermClientToServer = { type: "resize", cols: t.cols, rows: t.rows };
          ws.send(JSON.stringify(msg));
        }
      } catch {
        // ignore
      }
    };

    ws.onmessage = (ev) => {
      const t = xtermRef.current;
      if (!t) return;
      let msg: TermServerToClient | null = null;
      try {
        msg = JSON.parse(String(ev.data)) as TermServerToClient;
      } catch {
        t.write(String(ev.data));
        return;
      }
      if (msg.type === "output") {
        t.write(msg.data);
      } else if (msg.type === "connected") {
        setConnState("connected");
        setConnError(null);
      } else if (msg.type === "error") {
        setConnState("error");
        setConnError(msg.message || "Terminal error");
        writeSystemLine(`Error: ${msg.message || "Terminal error"}`);
      }
    };

    ws.onerror = () => {
      setConnState("error");
      setConnError("WebSocket error");
      writeSystemLine("Error: WebSocket error.");
    };

    ws.onclose = () => {
      wsRef.current = null;
      setConnState((prev) => (prev === "error" ? "error" : "disconnected"));
    };
  }

  const createConn = useMutation({
    mutationFn: async (vars: {
      name: string;
      host: string;
      port: number;
      username: string;
      auth_method: "key" | "password";
      private_key?: string | null;
      passphrase?: string | null;
      password?: string | null;
    }) => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/ssh-connections`, {
        method: "POST",
        json: vars,
      });
      if (!res.ok) {
        const e = await res.json().catch(() => ({}));
        throw new Error((e as { error?: string }).error || "Create failed");
      }
      return res.json() as Promise<{ connection: SshConnection }>;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["ssh-connections", orgId] });
    },
  });

  const deleteConn = useMutation({
    mutationFn: async (connectionId: UUID) => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/ssh-connections/${connectionId}`, {
        method: "DELETE",
      });
      if (!res.ok) {
        const e = await res.json().catch(() => ({}));
        throw new Error((e as { error?: string }).error || "Delete failed");
      }
      return true;
    },
    onSuccess: (_ok, connectionId) => {
      if (activeConnectionId === connectionId) {
        setActiveConnectionId(null);
        disconnect();
      }
      void qc.invalidateQueries({ queryKey: ["ssh-connections", orgId] });
    },
  });

  const [draft, setDraft] = useState({
    name: "Server",
    host: "",
    port: "22",
    username: "root",
    auth_method: "key" as "key" | "password",
    private_key: "",
    passphrase: "",
    password: "",
  });

  return (
    <div className="min-h-0 flex-1 overflow-hidden bg-background">
      <div className="flex h-full min-h-0 flex-col">
        <header className="border-b border-border px-4 py-3">
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0">
              <p className="text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
                {projectQ.data?.name ?? "Space"}
              </p>
              <h1 className="mt-1 truncate text-base font-semibold text-foreground">Terminal</h1>
            </div>
            <div className="flex items-center gap-2">
              <button
                type="button"
                className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                onClick={() => void navigate(`/o/${orgId}/p/${projectId}`)}
              >
                <ArrowLeft className="h-4 w-4 opacity-80" />
                Back
              </button>
              {activeConnection ? (
                <button
                  type="button"
                  className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                  onClick={() => {
                    if (connState === "connected" || connState === "connecting") {
                      disconnect();
                      return;
                    }
                    connect(activeConnection.id);
                  }}
                >
                  {connState === "connected" ? "Disconnect" : "Connect"}
                </button>
              ) : null}
              <button
                type="button"
                className="inline-flex items-center gap-2 rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                title="Terminal settings"
                aria-label="Terminal settings"
                onClick={() => {
                  setSettingsOpen(true);
                }}
              >
                <Settings className="h-4 w-4" />
                Settings
              </button>
            </div>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            {activeConnection
              ? `Selected: ${activeConnection.name} (${activeConnection.username}@${activeConnection.host}:${activeConnection.port})`
              : "Select a connection in Settings to start a shell session."}
            {connError ? ` — ${connError}` : ""}
          </p>
        </header>

        <main className="min-h-0 flex-1 overflow-hidden p-4">
          <div className="flex h-full min-h-0 flex-col rounded-sm border border-border bg-card">
            <div className="border-b border-border px-3 py-2 text-xs text-muted-foreground">
              {connState === "connected"
                ? "Connected"
                : connState === "connecting"
                  ? "Connecting…"
                  : connState === "error"
                    ? "Error"
                    : "Not connected"}
            </div>
            <div className="min-h-0 flex-1 overflow-hidden p-3">
              <div ref={termHostRef} className="h-full min-h-0 w-full" />
            </div>
          </div>
        </main>
      </div>

      {settingsOpen ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-2xl rounded-sm border border-border bg-card p-4 shadow-lg">
            <div className="flex items-center justify-between gap-2">
              <div className="text-sm font-semibold text-foreground">Terminal settings</div>
              <button
                type="button"
                className="rounded-sm border border-border px-3 py-1.5 text-sm hover:bg-accent"
                onClick={() => setSettingsOpen(false)}
              >
                Close
              </button>
            </div>

            <div className="mt-4 grid gap-4 md:grid-cols-2">
              <div className="min-h-0">
                <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Connections
                </div>
                <div className="mt-2 space-y-2">
                  {(connectionsQ.data ?? []).map((c) => {
                    const selected = c.id === activeConnectionId;
                    return (
                      <div
                        key={c.id}
                        className={[
                          "flex items-center justify-between gap-2 rounded-sm border border-border px-3 py-2",
                          selected ? "ring-2 ring-ring" : "",
                        ].join(" ")}
                      >
                        <button
                          type="button"
                          className="min-w-0 flex-1 text-left"
                          onClick={() => {
                            setActiveConnectionId(c.id);
                            setSettingsOpen(false);
                          }}
                        >
                          <div className="truncate text-sm font-medium text-foreground">{c.name}</div>
                          <div className="truncate text-xs text-muted-foreground">
                            {c.username}@{c.host}:{c.port}
                          </div>
                        </button>
                        <button
                          type="button"
                          className="rounded-sm border border-border p-2 text-muted-foreground hover:bg-accent hover:text-foreground"
                          title="Delete connection"
                          aria-label="Delete connection"
                          onClick={() => {
                            if (window.confirm("Delete this SSH connection?")) {
                              deleteConn.mutate(c.id);
                            }
                          }}
                          disabled={deleteConn.isPending}
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    );
                  })}
                  {connectionsQ.isPending ? (
                    <div className="text-xs text-muted-foreground">Loading…</div>
                  ) : null}
                  {!connectionsQ.isPending && (connectionsQ.data ?? []).length === 0 ? (
                    <div className="rounded-sm border border-dashed border-border bg-background px-3 py-6 text-center text-xs text-muted-foreground">
                      No connections yet.
                    </div>
                  ) : null}
                </div>
              </div>

              <div>
                <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Add connection
                </div>
                <div className="mt-2 space-y-2">
                  <input
                    className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2"
                    placeholder="Name"
                    value={draft.name}
                    onChange={(e) => setDraft((p) => ({ ...p, name: e.target.value }))}
                  />
                  <div className="grid gap-2 sm:grid-cols-3">
                    <input
                      className="sm:col-span-2 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2"
                      placeholder="Host (e.g. 203.0.113.10)"
                      value={draft.host}
                      onChange={(e) => setDraft((p) => ({ ...p, host: e.target.value }))}
                    />
                    <input
                      className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2"
                      placeholder="Port"
                      value={draft.port}
                      onChange={(e) => setDraft((p) => ({ ...p, port: e.target.value }))}
                    />
                  </div>
                  <input
                    className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2"
                    placeholder="Username"
                    value={draft.username}
                    onChange={(e) => setDraft((p) => ({ ...p, username: e.target.value }))}
                  />
                  <select
                    className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                    value={draft.auth_method}
                    onChange={(e) =>
                      setDraft((p) => ({ ...p, auth_method: e.target.value as "key" | "password" }))
                    }
                    aria-label="Auth method"
                  >
                    <option value="key">SSH key (recommended)</option>
                    <option value="password">Password</option>
                  </select>

                  {draft.auth_method === "key" ? (
                    <>
                      <textarea
                        className="h-36 w-full resize-none rounded-sm border border-input bg-background px-3 py-2 font-mono text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2"
                        placeholder="Paste private key (PEM)"
                        value={draft.private_key}
                        onChange={(e) => setDraft((p) => ({ ...p, private_key: e.target.value }))}
                        spellCheck={false}
                      />
                      <input
                        className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2"
                        placeholder="Passphrase (optional)"
                        value={draft.passphrase}
                        onChange={(e) => setDraft((p) => ({ ...p, passphrase: e.target.value }))}
                      />
                    </>
                  ) : (
                    <>
                      <input
                        className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2"
                        placeholder="Password (optional — leave blank to prompt on connect)"
                        value={draft.password}
                        onChange={(e) => setDraft((p) => ({ ...p, password: e.target.value }))}
                        type="password"
                        autoComplete="off"
                      />
                      <p className="text-xs text-muted-foreground">
                        If left blank, you’ll be prompted each time you connect.
                      </p>
                    </>
                  )}
                  <button
                    type="button"
                    className="inline-flex items-center gap-2 rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                    disabled={
                      createConn.isPending ||
                      !draft.name.trim() ||
                      !draft.host.trim() ||
                      !draft.username.trim() ||
                      (draft.auth_method === "key" ? !draft.private_key.trim() : false)
                    }
                    onClick={async () => {
                      const portNum = Number(draft.port.trim() || "22");
                      await createConn.mutateAsync({
                        name: draft.name.trim(),
                        host: draft.host.trim(),
                        port: Number.isFinite(portNum) && portNum > 0 ? portNum : 22,
                        username: draft.username.trim(),
                        auth_method: draft.auth_method,
                        private_key:
                          draft.auth_method === "key" ? (draft.private_key.trim() ? draft.private_key : null) : null,
                        passphrase:
                          draft.auth_method === "key" ? (draft.passphrase.trim() ? draft.passphrase : null) : null,
                        password:
                          draft.auth_method === "password"
                            ? (draft.password.trim() ? draft.password : null)
                            : null,
                      });
                      setDraft((p) => ({ ...p, private_key: "", passphrase: "", password: "" }));
                      void qc.invalidateQueries({ queryKey: ["ssh-connections", orgId] });
                    }}
                  >
                    <Plus className="h-4 w-4" />
                    Add
                  </button>
                  {createConn.isError ? (
                    <div className="text-xs text-destructive">
                      {(createConn.error as Error).message}
                    </div>
                  ) : null}
                </div>
              </div>
            </div>
          </div>
        </div>
      ) : null}

      {passwordPromptOpen && activeConnection ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-md rounded-sm border border-border bg-card p-4 shadow-lg">
            <div className="text-sm font-semibold text-foreground">Enter password</div>
            <p className="mt-1 text-xs text-muted-foreground">
              Password for {activeConnection.username}@{activeConnection.host}
            </p>
            <input
              autoFocus
              type="password"
              className="mt-3 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
              value={passwordDraft}
              onChange={(e) => setPasswordDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  const ws = wsRef.current;
                  if (!ws) return;
                  const msg: TermClientToServer = {
                    type: "auth",
                    method: "password",
                    password: passwordDraft,
                  };
                  ws.send(JSON.stringify(msg));
                  setPasswordPromptOpen(false);
                }
                if (e.key === "Escape") {
                  disconnect();
                }
              }}
              placeholder="Password"
              autoComplete="off"
            />
            <div className="mt-4 flex justify-end gap-2">
              <button
                type="button"
                className="rounded-sm border border-border px-3 py-2 text-sm hover:bg-accent"
                onClick={() => disconnect()}
              >
                Cancel
              </button>
              <button
                type="button"
                className="rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                disabled={!passwordDraft}
                onClick={() => {
                  const ws = wsRef.current;
                  if (!ws) return;
                  const msg: TermClientToServer = {
                    type: "auth",
                    method: "password",
                    password: passwordDraft,
                  };
                  ws.send(JSON.stringify(msg));
                  setPasswordPromptOpen(false);
                }}
              >
                Continue
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

