import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Zap } from "lucide-react";
import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { apiFetch } from "../api/http";
import type { Project, UUID } from "../api/types";

type AutomationKind = "social_post" | "reverse_tunnel" | "scheduled" | "webhook";
type AutomationStatus =
  | "draft"
  | "pending_approval"
  | "active"
  | "paused"
  | "failed"
  | "rejected";

type SpaceAutomation = {
  id: UUID;
  organization_id: UUID;
  space_id: UUID;
  name: string;
  kind: AutomationKind;
  config: Record<string, unknown>;
  status: AutomationStatus;
  created_by_user_id?: UUID;
  created_by_service_account_id?: UUID;
  reviewed_by_user_id?: UUID;
  reviewed_at?: string;
  rejection_reason?: string;
  last_run_at?: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

type SpaceAutomationRun = {
  id: UUID;
  automation_id: UUID;
  started_at: string;
  finished_at?: string;
  success: boolean;
  error_message?: string;
  external_ref?: string;
  created_at: string;
};

export default function SpaceAutomationsPage() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();
  const qc = useQueryClient();
  const [newName, setNewName] = useState("");
  const [newKind, setNewKind] = useState<AutomationKind>("social_post");
  const [newDefaultText, setNewDefaultText] = useState("");
  const [tokenById, setTokenById] = useState<Record<string, string>>({});
  const [runTextById, setRunTextById] = useState<Record<string, string>>({});
  const [selectedId, setSelectedId] = useState<UUID | null>(null);

  const spaceQ = useQuery({
    queryKey: ["project", orgId, projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${projectId}`);
      if (!res.ok) throw new Error("space");
      return res.json() as Promise<Project>;
    },
  });

  const listQ = useQuery({
    queryKey: ["space-automations", orgId, projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/automations`,
      );
      if (!res.ok) throw new Error("automations");
      const j = (await res.json()) as { automations: SpaceAutomation[] };
      return j.automations ?? [];
    },
  });

  const runsQ = useQuery({
    queryKey: ["space-automation-runs", orgId, projectId, selectedId],
    enabled: !!orgId && !!projectId && !!selectedId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/automations/${selectedId}/runs`,
      );
      if (!res.ok) throw new Error("runs");
      const j = (await res.json()) as { runs: SpaceAutomationRun[] };
      return j.runs ?? [];
    },
  });

  const pending = useMemo(
    () => (listQ.data ?? []).filter((a) => a.status === "pending_approval"),
    [listQ.data],
  );
  const social = useMemo(
    () => (listQ.data ?? []).filter((a) => a.kind === "social_post"),
    [listQ.data],
  );

  const createM = useMutation({
    mutationFn: async () => {
      const config: Record<string, unknown> = {};
      if (newDefaultText.trim()) config.default_text = newDefaultText.trim();
      const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${projectId}/automations`, {
        method: "POST",
        json: {
          name: newName.trim(),
          kind: newKind,
          config,
          status: "draft",
        },
      });
      if (!res.ok) throw new Error("create");
      return res.json() as Promise<{ automation: SpaceAutomation }>;
    },
    onSuccess: () => {
      setNewName("");
      setNewDefaultText("");
      void qc.invalidateQueries({ queryKey: ["space-automations", orgId, projectId] });
    },
  });

  const approveM = useMutation({
    mutationFn: async (id: UUID) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/automations/${id}/approve`,
        { method: "POST" },
      );
      if (!res.ok) throw new Error("approve");
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["space-automations", orgId, projectId] }),
  });

  const rejectM = useMutation({
    mutationFn: async (id: UUID) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/automations/${id}/reject`,
        { method: "POST", json: { reason: "Rejected from Automations page" } },
      );
      if (!res.ok) throw new Error("reject");
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["space-automations", orgId, projectId] }),
  });

  const saveTokenM = useMutation({
    mutationFn: async ({ id, token }: { id: UUID; token: string }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/automations/${id}`,
        { method: "PATCH", json: { oauth_token: token } },
      );
      if (!res.ok) throw new Error("patch");
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["space-automations", orgId, projectId] }),
  });

  const activateDraftM = useMutation({
    mutationFn: async ({ id, token }: { id: UUID; token: string }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/automations/${id}`,
        { method: "PATCH", json: { oauth_token: token, status: "active" } },
      );
      if (!res.ok) throw new Error("patch");
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["space-automations", orgId, projectId] }),
  });

  const runM = useMutation({
    mutationFn: async ({ id, text }: { id: UUID; text: string }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/automations/${id}/run`,
        { method: "POST", json: { text } },
      );
      if (!res.ok) throw new Error("run");
      return res.json() as Promise<{ tweet_id?: string; error?: unknown }>;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["space-automations", orgId, projectId] });
      void qc.invalidateQueries({ queryKey: ["space-automation-runs", orgId, projectId, selectedId] });
    },
  });

  const deleteM = useMutation({
    mutationFn: async (id: UUID) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/automations/${id}`,
        { method: "DELETE" },
      );
      if (!res.ok) throw new Error("delete");
    },
    onSuccess: () => {
      setSelectedId(null);
      void qc.invalidateQueries({ queryKey: ["space-automations", orgId, projectId] });
    },
  });

  if (!orgId || !projectId) return null;

  const spaceName = spaceQ.data?.name ?? "Space";

  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-background">
      <div className="mx-auto max-w-4xl px-4 py-8">
        <header className="border-b border-border pb-6">
          <Link
            to={`/o/${orgId}/p/${projectId}`}
            className="inline-flex items-center gap-1 text-xs text-link hover:underline"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back to {spaceName}
          </Link>
          <div className="mt-4 flex items-center gap-2">
            <Zap className="h-7 w-7 text-primary" aria-hidden />
            <div>
              <h1 className="text-2xl font-semibold tracking-tight text-foreground">Automations</h1>
              <p className="mt-1 text-sm text-muted-foreground">
                Build and run workflows for this space — social posting first; tunnels and schedules
                coming next.
              </p>
            </div>
          </div>
        </header>

        <section className="mt-8 rounded-sm border border-border bg-card p-4">
          <h2 className="text-sm font-semibold text-foreground">Pending approval</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            AI staff proposals appear here. Approve to activate (add X credentials before posting).
          </p>
          {listQ.isLoading ? (
            <p className="mt-3 text-sm text-muted-foreground">Loading…</p>
          ) : pending.length === 0 ? (
            <p className="mt-3 text-sm text-muted-foreground">No pending automations.</p>
          ) : (
            <ul className="mt-3 space-y-2">
              {pending.map((a) => (
                <li
                  key={a.id}
                  className="flex flex-wrap items-center justify-between gap-2 rounded-sm border border-border bg-background px-3 py-2 text-sm"
                >
                  <div>
                    <span className="font-medium text-foreground">{a.name}</span>
                    <span className="ml-2 text-xs text-muted-foreground">{a.kind}</span>
                    {a.created_by_service_account_id ? (
                      <span className="ml-2 text-xs text-amber-600 dark:text-amber-400">AI proposal</span>
                    ) : null}
                  </div>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      className="rounded-sm bg-primary px-2 py-1 text-xs font-medium text-primary-foreground"
                      onClick={() => approveM.mutate(a.id)}
                      disabled={approveM.isPending}
                    >
                      Approve
                    </button>
                    <button
                      type="button"
                      className="rounded-sm border border-border px-2 py-1 text-xs"
                      onClick={() => rejectM.mutate(a.id)}
                      disabled={rejectM.isPending}
                    >
                      Reject
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="mt-8">
          <h2 className="text-lg font-semibold text-foreground">Social (X / Twitter)</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Store an OAuth2 bearer token with{" "}
            <code className="rounded bg-muted px-1">tweet.write</code> scope, approve drafts, then run
            or rely on default text from config.
          </p>

          <div className="mt-4 rounded-sm border border-border bg-card p-4">
            <h3 className="text-sm font-medium text-foreground">New automation</h3>
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <label className="block text-xs text-muted-foreground">
                Name
                <input
                  className="mt-1 w-full rounded-sm border border-input bg-background px-2 py-1.5 text-sm"
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder="e.g. Launch posts"
                />
              </label>
              <label className="block text-xs text-muted-foreground">
                Kind
                <select
                  className="mt-1 w-full rounded-sm border border-input bg-background px-2 py-1.5 text-sm"
                  value={newKind}
                  onChange={(e) => setNewKind(e.target.value as AutomationKind)}
                >
                  <option value="social_post">social_post</option>
                  <option value="reverse_tunnel" disabled>
                    reverse_tunnel (soon)
                  </option>
                  <option value="scheduled" disabled>
                    scheduled (soon)
                  </option>
                  <option value="webhook" disabled>
                    webhook (soon)
                  </option>
                </select>
              </label>
            </div>
            <label className="mt-3 block text-xs text-muted-foreground">
              Default post text (optional)
              <textarea
                className="mt-1 min-h-[72px] w-full rounded-sm border border-input bg-background px-2 py-1.5 font-mono text-sm"
                value={newDefaultText}
                onChange={(e) => setNewDefaultText(e.target.value)}
                placeholder="Saved into config.default_text"
              />
            </label>
            <button
              type="button"
              className="mt-3 rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
              disabled={createM.isPending || !newName.trim()}
              onClick={() => createM.mutate()}
            >
              Create draft
            </button>
          </div>

          <ul className="mt-6 space-y-3">
            {social.map((a) => (
              <li key={a.id} className="rounded-sm border border-border bg-card p-4">
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div>
                    <button
                      type="button"
                      className="text-left font-medium text-foreground hover:underline"
                      onClick={() => setSelectedId(a.id === selectedId ? null : a.id)}
                    >
                      {a.name}
                    </button>
                    <span className="ml-2 text-xs text-muted-foreground">{a.status}</span>
                    {typeof a.config?.default_text === "string" && (
                      <p className="mt-1 text-xs text-muted-foreground line-clamp-2">
                        {String(a.config.default_text)}
                      </p>
                    )}
                    {a.last_error ? (
                      <p className="mt-1 text-xs text-red-600">{a.last_error}</p>
                    ) : null}
                  </div>
                  <button
                    type="button"
                    className="text-xs text-red-600 hover:underline"
                    onClick={() => {
                      if (confirm(`Delete “${a.name}”?`)) deleteM.mutate(a.id);
                    }}
                  >
                    Delete
                  </button>
                </div>
                {a.status === "draft" && (
                  <div className="mt-3 space-y-2 border-t border-border pt-3">
                    <label className="block text-xs text-muted-foreground">
                      OAuth2 bearer token (stored encrypted), then activate
                      <input
                        type="password"
                        className="mt-1 w-full max-w-md rounded-sm border border-input bg-background px-2 py-1.5 font-mono text-sm"
                        placeholder="X API bearer token"
                        value={tokenById[a.id] ?? ""}
                        onChange={(e) =>
                          setTokenById((m) => ({ ...m, [a.id]: e.target.value }))
                        }
                      />
                    </label>
                    <button
                      type="button"
                      className="rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                      disabled={activateDraftM.isPending || !(tokenById[a.id]?.trim())}
                      onClick={() =>
                        activateDraftM.mutate({ id: a.id, token: tokenById[a.id]!.trim() })
                      }
                    >
                      Save token and activate
                    </button>
                  </div>
                )}
                {a.status === "active" && (
                  <div className="mt-3 space-y-2 border-t border-border pt-3">
                    <label className="block text-xs text-muted-foreground">
                      Rotate OAuth2 bearer token (optional)
                      <input
                        type="password"
                        className="mt-1 w-full max-w-md rounded-sm border border-input bg-background px-2 py-1.5 font-mono text-sm"
                        placeholder="Paste new token, then Save token"
                        value={tokenById[a.id] ?? ""}
                        onChange={(e) =>
                          setTokenById((m) => ({ ...m, [a.id]: e.target.value }))
                        }
                      />
                    </label>
                    <button
                      type="button"
                      className="rounded-sm border border-border px-2 py-1 text-xs"
                      disabled={saveTokenM.isPending || !(tokenById[a.id]?.trim())}
                      onClick={() =>
                        saveTokenM.mutate({ id: a.id, token: tokenById[a.id]!.trim() })
                      }
                    >
                      Save token
                    </button>
                    <label className="block text-xs text-muted-foreground">
                      Text to post (overrides default for this run)
                      <input
                        className="mt-1 w-full max-w-md rounded-sm border border-input bg-background px-2 py-1.5 text-sm"
                        value={runTextById[a.id] ?? ""}
                        onChange={(e) =>
                          setRunTextById((m) => ({ ...m, [a.id]: e.target.value }))
                        }
                        placeholder="Leave empty to use default_text"
                      />
                    </label>
                    <button
                      type="button"
                      className="rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                      disabled={runM.isPending}
                      onClick={() =>
                        runM.mutate({
                          id: a.id,
                          text: (runTextById[a.id] ?? "").trim(),
                        })
                      }
                    >
                      Post now
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
          {!listQ.isLoading && social.length === 0 ? (
            <p className="mt-4 text-sm text-muted-foreground">No social automations yet.</p>
          ) : null}
        </section>

        <section className="mt-10 rounded-sm border border-dashed border-border bg-muted/20 p-4">
          <h2 className="text-sm font-semibold text-foreground">Connectivity (reverse tunnel)</h2>
          <p className="mt-1 text-sm text-muted-foreground">Coming soon — health checks and tunnel status.</p>
        </section>

        <section className="mt-4 rounded-sm border border-dashed border-border bg-muted/20 p-4">
          <h2 className="text-sm font-semibold text-foreground">Scheduled / webhooks</h2>
          <p className="mt-1 text-sm text-muted-foreground">Coming soon.</p>
        </section>

        {selectedId ? (
          <section className="mt-8 border-t border-border pt-6">
            <h2 className="text-sm font-semibold text-foreground">Recent runs (selected)</h2>
            {runsQ.isLoading ? (
              <p className="mt-2 text-sm text-muted-foreground">Loading runs…</p>
            ) : (
              <ul className="mt-2 space-y-1 font-mono text-xs text-muted-foreground">
                {(runsQ.data ?? []).map((r) => (
                  <li key={r.id}>
                    {r.success ? "ok" : "fail"} — {new Date(r.started_at).toLocaleString()}
                    {r.external_ref ? ` — tweet ${r.external_ref}` : ""}
                    {r.error_message ? ` — ${r.error_message}` : ""}
                  </li>
                ))}
              </ul>
            )}
          </section>
        ) : null}
      </div>
    </div>
  );
}
