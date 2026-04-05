import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { apiFetch, integrationQueryError } from "../api/http";
import { DEFAULT_SERVICE_ACCOUNT_PROFILE_MD } from "../constants/defaultServiceAccountProfileMd";
import {
  type ApplyablePresetId,
  getPresetMarkdown,
  inferPresetId,
  PROFILE_PRESET_OPTIONS,
} from "../constants/serviceAccountProfilePresets";
import type {
  Role,
  ServiceAccount,
  ServiceAccountProfileVersion,
  ServiceAccountProvider,
  UUID,
} from "../api/types";

export default function OrgServiceAccountsPage() {
  const { orgId } = useParams<{ orgId: string }>();
  const qc = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState("");
  const [createdToken, setCreatedToken] = useState<string | null>(null);
  const [warnDismiss, setWarnDismiss] = useState(false);
  const [selectedSA, setSelectedSA] = useState<UUID | null>(null);
  const [profileDraft, setProfileDraft] = useState("");
  const [profileTouched, setProfileTouched] = useState(false);
  const [selectedRoleIds, setSelectedRoleIds] = useState<UUID[]>([]);
  const [createProvider, setCreateProvider] = useState<ServiceAccountProvider>("openrouter");
  const [createOrModel, setCreateOrModel] = useState("nvidia/nemotron-3-super-120b-a12b:free");
  const [createRepo, setCreateRepo] = useState("");
  const [createRef, setCreateRef] = useState("");
  const [editProvider, setEditProvider] = useState<ServiceAccountProvider>("openrouter");
  const [editOrModel, setEditOrModel] = useState("");
  const [editRepo, setEditRepo] = useState("");
  const [editRef, setEditRef] = useState("");
  const [cursorKeyDraft, setCursorKeyDraft] = useState("");
  const [openRouterKeyDraft, setOpenRouterKeyDraft] = useState("");
  /** Provider of the service account whose token is in `createdToken` (for post-create UI). */
  const [createdProvider, setCreatedProvider] = useState<ServiceAccountProvider | null>(null);

  const cursorQ = useQuery({
    queryKey: ["org-cursor-integration", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/integrations/cursor`);
      if (!res.ok) throw await integrationQueryError(res, "Cursor integration");
      const j = (await res.json()) as {
        cursor_integration: {
          configured: boolean;
          last_rotated_at?: string;
          hint?: string;
        };
      };
      return j.cursor_integration;
    },
  });

  const putCursorKey = useMutation({
    mutationFn: async (apiKey: string) => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/integrations/cursor`, {
        method: "PUT",
        json: { api_key: apiKey },
      });
      if (!res.ok) {
        const j = await res.json().catch(() => ({}));
        throw new Error((j as { error?: string }).error ?? "save cursor key");
      }
      const j = (await res.json()) as {
        cursor_integration: {
          configured: boolean;
          last_rotated_at?: string;
          hint?: string;
        };
      };
      return j.cursor_integration;
    },
    onSuccess: (meta) => {
      qc.setQueryData(["org-cursor-integration", orgId], meta);
    },
  });

  const deleteCursorKey = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/integrations/cursor`, {
        method: "DELETE",
      });
      if (!res.ok) throw new Error("clear cursor key");
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["org-cursor-integration", orgId] });
    },
  });

  const openRouterQ = useQuery({
    queryKey: ["org-openrouter-integration", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/integrations/openrouter`);
      if (!res.ok) throw await integrationQueryError(res, "OpenRouter integration");
      const j = (await res.json()) as {
        openrouter_integration: {
          configured: boolean;
          last_rotated_at?: string;
          hint?: string;
        };
      };
      return j.openrouter_integration;
    },
  });

  const putOpenRouterKey = useMutation({
    mutationFn: async (apiKey: string) => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/integrations/openrouter`, {
        method: "PUT",
        json: { api_key: apiKey },
      });
      if (!res.ok) {
        const j = await res.json().catch(() => ({}));
        throw new Error((j as { error?: string }).error ?? "save openrouter key");
      }
      const j = (await res.json()) as {
        openrouter_integration: {
          configured: boolean;
          last_rotated_at?: string;
          hint?: string;
        };
      };
      return j.openrouter_integration;
    },
    onSuccess: (meta) => {
      qc.setQueryData(["org-openrouter-integration", orgId], meta);
    },
  });

  const deleteOpenRouterKey = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/integrations/openrouter`, {
        method: "DELETE",
      });
      if (!res.ok) throw new Error("clear openrouter key");
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["org-openrouter-integration", orgId] });
    },
  });

  const listQ = useQuery({
    queryKey: ["service-accounts", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/service-accounts`);
      if (!res.ok) throw new Error("list");
      const j = (await res.json()) as { service_accounts: ServiceAccount[] };
      return j.service_accounts;
    },
  });

  const rolesQ = useQuery({
    queryKey: ["org-roles", orgId],
    enabled: !!orgId && createOpen,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/roles`);
      if (!res.ok) throw new Error("roles");
      const j = (await res.json()) as { roles: Role[] };
      return j.roles;
    },
  });

  const profileQ = useQuery({
    queryKey: ["sa-profile", orgId, selectedSA],
    enabled: !!orgId && !!selectedSA,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/service-accounts/${selectedSA}/profile`,
      );
      if (!res.ok) throw new Error("profile");
      return res.json() as Promise<{
        profile: ServiceAccountProfileVersion | null;
        /** Shown when `profile` is null (no saved version yet); same as server default template. */
        default_content_md?: string;
      }>;
    },
  });

  const versionsQ = useQuery({
    queryKey: ["sa-profile-versions", orgId, selectedSA],
    enabled: !!orgId && !!selectedSA,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/service-accounts/${selectedSA}/profile/versions`,
      );
      if (!res.ok) throw new Error("versions");
      return res.json() as Promise<{ versions: ServiceAccountProfileVersion[] }>;
    },
  });

  /** Saved profile text, or default template when latest version is blank (e.g. empty v1). */
  const profileDisplayMd = useMemo(() => {
    const p = profileQ.data;
    if (!p) return "";
    const saved = p.profile?.content_md ?? "";
    const def = (p.default_content_md ?? "").trim() || DEFAULT_SERVICE_ACCOUNT_PROFILE_MD;
    return saved.trim() !== "" ? saved : def;
  }, [profileQ.data]);

  const profileEditorSnapshot = profileTouched ? profileDraft : profileDisplayMd;
  const profilePresetSelectValue = useMemo(
    () => inferPresetId(profileEditorSnapshot),
    [profileEditorSnapshot],
  );

  useEffect(() => {
    setProfileTouched(false);
  }, [selectedSA]);

  useEffect(() => {
    if (profileTouched || !profileQ.data) return;
    setProfileDraft(profileDisplayMd);
  }, [profileDisplayMd, profileTouched, selectedSA, profileQ.data]);

  const selectedRow = useMemo(
    () => (listQ.data ?? []).find((s) => s.id === selectedSA) ?? null,
    [listQ.data, selectedSA],
  );

  useEffect(() => {
    if (!selectedRow) return;
    setEditProvider(selectedRow.provider ?? "openrouter");
    setEditOrModel(selectedRow.openrouter_model?.trim() || "nvidia/nemotron-3-super-120b-a12b:free");
    setEditRepo(selectedRow.cursor_default_repo_url?.trim() || "");
    setEditRef(selectedRow.cursor_default_ref?.trim() || "");
  }, [selectedRow]);

  const createM = useMutation({
    mutationFn: async (vars: {
      name: string;
      role_ids: UUID[];
      provider: ServiceAccountProvider;
      openrouter_model?: string;
      cursor_default_repo_url?: string;
      cursor_default_ref?: string;
    }) => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/service-accounts`, {
        method: "POST",
        json: {
          name: vars.name,
          role_ids: vars.role_ids,
          provider: vars.provider,
          openrouter_model: vars.openrouter_model,
          cursor_default_repo_url: vars.cursor_default_repo_url,
          cursor_default_ref: vars.cursor_default_ref,
        },
      });
      if (!res.ok) throw new Error("create");
      return res.json() as Promise<{ service_account: ServiceAccount; token: string }>;
    },
    onSuccess: (data, vars) => {
      setCreatedToken(data.token);
      setCreatedProvider(vars.provider);
      setWarnDismiss(false);
      setCreateOpen(false);
      setNewName("");
      setSelectedRoleIds([]);
      setCreateProvider("openrouter");
      setCreateOrModel("nvidia/nemotron-3-super-120b-a12b:free");
      setCreateRepo("");
      setCreateRef("");
      void qc.invalidateQueries({ queryKey: ["service-accounts", orgId] });
    },
  });

  const patchM = useMutation({
    mutationFn: async (vars: {
      serviceAccountId: UUID;
      provider: ServiceAccountProvider;
      openrouter_model?: string;
      cursor_default_repo_url?: string;
      cursor_default_ref?: string;
    }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/service-accounts/${vars.serviceAccountId}`,
        {
          method: "PATCH",
          json: {
            provider: vars.provider,
            openrouter_model: vars.openrouter_model,
            cursor_default_repo_url: vars.cursor_default_repo_url,
            cursor_default_ref: vars.cursor_default_ref,
          },
        },
      );
      if (!res.ok) throw new Error("patch");
      return res.json() as Promise<{ service_account: ServiceAccount }>;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["service-accounts", orgId] });
    },
  });

  const deleteM = useMutation({
    mutationFn: async (id: UUID) => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/service-accounts/${id}`, {
        method: "DELETE",
      });
      if (!res.ok) throw new Error("delete");
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["service-accounts", orgId] });
      setSelectedSA(null);
    },
  });

  const saveProfileM = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/service-accounts/${selectedSA}/profile`,
        { method: "PATCH", json: { content_md: profileDraft } },
      );
      if (!res.ok) throw new Error("save");
      return res.json() as Promise<{ profile: ServiceAccountProfileVersion }>;
    },
    onSuccess: () => {
      setProfileTouched(false);
      void qc.invalidateQueries({ queryKey: ["sa-profile", orgId, selectedSA] });
      void qc.invalidateQueries({ queryKey: ["sa-profile-versions", orgId, selectedSA] });
    },
  });

  if (!orgId) return null;

  const apiBase = import.meta.env.VITE_API_URL || "http://localhost:8080";

  const mcpTemplatePlaceholder = useMemo(
    () =>
      JSON.stringify(
        {
          mcpServers: {
            hyperspeed: {
              command: "mcp-hyperspeed",
              args: [] as string[],
              env: {
                HYPERSPEED_API_URL: apiBase,
                HYPERSPEED_TOKEN: "<paste token from new service account>",
                HYPERSPEED_ORG_ID: orgId,
              },
            },
          },
        },
        null,
        2,
      ),
    [apiBase, orgId],
  );

  const envBlock = createdToken
    ? `HYPERSPEED_API_URL=${apiBase}\nHYPERSPEED_TOKEN=${createdToken}\nHYPERSPEED_ORG_ID=${orgId}`
    : "";

  const cursorMcpJson = createdToken
    ? JSON.stringify(
        {
          mcpServers: {
            hyperspeed: {
              command: "mcp-hyperspeed",
              args: [] as string[],
              env: {
                HYPERSPEED_API_URL: apiBase,
                HYPERSPEED_TOKEN: createdToken,
                HYPERSPEED_ORG_ID: orgId,
              },
            },
          },
        },
        null,
        2,
      )
    : "";

  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-background">
      <div className="mx-auto max-w-4xl px-4 py-8">
        <header className="border-b border-border pb-6">
          <Link to={`/o/${orgId}/settings`} className="text-xs text-link hover:underline">
            ← Back to settings
          </Link>
          <p className="mt-2 text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
            Settings
          </p>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight text-foreground">
            AI staff &amp; MCP
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Org-wide API keys for chat AI, service account tokens, and Cursor MCP. Tokens are shown only once — copy
            them immediately.
          </p>
        </header>

        <div className="mt-8 space-y-4">
          <div className="rounded-sm border border-border bg-card p-4">
            <div className="text-sm font-semibold text-foreground">Cursor integration</div>
            <div className="mt-1 text-sm text-muted-foreground">
              Org-wide API key for Cursor-backed AI replies in chat. The secret is encrypted and never shown again after
              saving; only a short hint is displayed.
            </div>
            {cursorQ.isError ? (
              <p className="mt-2 text-sm text-destructive">
                Could not load Cursor integration settings.{" "}
                {(cursorQ.error as Error)?.message ? (
                  <span className="font-mono text-xs">{(cursorQ.error as Error).message}</span>
                ) : null}
              </p>
            ) : null}
            {cursorQ.data ? (
              <div className="mt-3 space-y-2 text-sm">
                <p className="text-foreground">
                  Status:{" "}
                  <span className="font-medium">
                    {cursorQ.data.configured ? "Configured" : "Not configured"}
                  </span>
                  {cursorQ.data.hint ? (
                    <span className="text-muted-foreground"> ({cursorQ.data.hint})</span>
                  ) : null}
                </p>
                {cursorQ.data.last_rotated_at ? (
                  <p className="text-xs text-muted-foreground">
                    Last updated: {new Date(cursorQ.data.last_rotated_at).toLocaleString()}
                  </p>
                ) : null}
                <label className="mt-2 block text-xs font-medium text-muted-foreground">
                  New API key (replaces existing)
                </label>
                <input
                  type="password"
                  autoComplete="off"
                  className="mt-1 w-full max-w-md rounded-sm border border-border bg-background px-2 py-1.5 text-sm text-foreground"
                  placeholder="Paste Cursor API key"
                  value={cursorKeyDraft}
                  onChange={(e) => setCursorKeyDraft(e.target.value)}
                />
                <div className="flex flex-wrap gap-2 pt-1">
                  <button
                    type="button"
                    className="rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                    disabled={
                      putCursorKey.isPending || !cursorKeyDraft.trim() || cursorQ.isPending
                    }
                    onClick={() => {
                      putCursorKey.mutate(cursorKeyDraft.trim(), {
                        onSuccess: () => setCursorKeyDraft(""),
                      });
                    }}
                  >
                    Save key
                  </button>
                  <button
                    type="button"
                    className="rounded-sm border border-border px-3 py-1.5 text-sm font-medium text-foreground disabled:opacity-50"
                    disabled={deleteCursorKey.isPending || !cursorQ.data.configured || cursorQ.isPending}
                    onClick={() => {
                      if (
                        !confirm(
                          "Remove the Cursor API key? Chat AI mentions will stop using Cursor until a new key is saved.",
                        )
                      ) {
                        return;
                      }
                      deleteCursorKey.mutate();
                    }}
                  >
                    Remove key
                  </button>
                </div>
                {putCursorKey.isError ? (
                  <p className="text-sm text-destructive">{(putCursorKey.error as Error).message}</p>
                ) : null}
              </div>
            ) : null}
          </div>

          <div className="rounded-sm border border-border bg-card p-4">
            <div className="text-sm font-semibold text-foreground">OpenRouter integration</div>
            <div className="mt-1 text-sm text-muted-foreground">
              Org-wide API key for OpenRouter-backed AI staff in chat (chat completions). Keys are created in the{" "}
              <a
                className="text-link hover:underline"
                href="https://openrouter.ai/"
                target="_blank"
                rel="noreferrer"
              >
                OpenRouter dashboard
              </a>
              . The secret is encrypted and never shown again after saving; only a short hint is displayed.
            </div>
            {openRouterQ.isError ? (
              <p className="mt-2 text-sm text-destructive">
                Could not load OpenRouter integration settings.{" "}
                {(openRouterQ.error as Error)?.message ? (
                  <span className="font-mono text-xs">{(openRouterQ.error as Error).message}</span>
                ) : null}
              </p>
            ) : null}
            {openRouterQ.data ? (
              <div className="mt-3 space-y-2 text-sm">
                <p className="text-foreground">
                  Status:{" "}
                  <span className="font-medium">
                    {openRouterQ.data.configured ? "Configured" : "Not configured"}
                  </span>
                  {openRouterQ.data.hint ? (
                    <span className="text-muted-foreground"> ({openRouterQ.data.hint})</span>
                  ) : null}
                </p>
                {openRouterQ.data.last_rotated_at ? (
                  <p className="text-xs text-muted-foreground">
                    Last updated: {new Date(openRouterQ.data.last_rotated_at).toLocaleString()}
                  </p>
                ) : null}
                <label className="mt-2 block text-xs font-medium text-muted-foreground">
                  New API key (replaces existing)
                </label>
                <input
                  type="password"
                  autoComplete="off"
                  className="mt-1 w-full max-w-md rounded-sm border border-border bg-background px-2 py-1.5 text-sm text-foreground"
                  placeholder="Paste OpenRouter API key"
                  value={openRouterKeyDraft}
                  onChange={(e) => setOpenRouterKeyDraft(e.target.value)}
                />
                <div className="flex flex-wrap gap-2 pt-1">
                  <button
                    type="button"
                    className="rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                    disabled={
                      putOpenRouterKey.isPending || !openRouterKeyDraft.trim() || openRouterQ.isPending
                    }
                    onClick={() => {
                      putOpenRouterKey.mutate(openRouterKeyDraft.trim(), {
                        onSuccess: () => setOpenRouterKeyDraft(""),
                      });
                    }}
                  >
                    Save key
                  </button>
                  <button
                    type="button"
                    className="rounded-sm border border-border px-3 py-1.5 text-sm font-medium text-foreground disabled:opacity-50"
                    disabled={
                      deleteOpenRouterKey.isPending || !openRouterQ.data.configured || openRouterQ.isPending
                    }
                    onClick={() => {
                      if (
                        !confirm(
                          "Remove the OpenRouter API key? OpenRouter-backed AI staff will stop until a new key is saved.",
                        )
                      ) {
                        return;
                      }
                      deleteOpenRouterKey.mutate();
                    }}
                  >
                    Remove key
                  </button>
                </div>
                {putOpenRouterKey.isError ? (
                  <p className="text-sm text-destructive">{(putOpenRouterKey.error as Error).message}</p>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>

        <div className="mt-8 rounded-sm border border-border bg-card p-4">
          <h2 className="text-sm font-semibold text-foreground">Cursor MCP</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Optional IDE integration: run Hyperspeed tools from Cursor using the{" "}
            <code className="font-mono text-xs">mcp-hyperspeed</code> server. Create a service account below, copy its
            token when shown once, then paste the token into the config here and add it under{" "}
            <span className="font-medium text-foreground">Cursor → Settings → MCP</span>. This is separate from{" "}
            <span className="font-medium text-foreground">OpenRouter</span> or <span className="font-medium text-foreground">Cursor Cloud Agents</span> used for <em>chat</em> above.
          </p>
          <pre className="mt-3 max-h-56 overflow-auto rounded-sm border border-border bg-background p-3 font-mono text-xs text-muted-foreground">
            {mcpTemplatePlaceholder}
          </pre>
          <div className="mt-2 flex flex-wrap gap-2">
            <button
              type="button"
              className="rounded-sm border border-border px-3 py-1.5 text-sm hover:bg-accent"
              onClick={() => void navigator.clipboard.writeText(mcpTemplatePlaceholder)}
            >
              Copy template
            </button>
          </div>
          <p className="mt-3 text-xs text-muted-foreground">
            Build or install <code className="font-mono">mcp-hyperspeed</code> from{" "}
            <code className="font-mono">apps/api/cmd/mcp-hyperspeed</code> and put it on your PATH, or set{" "}
            <code className="font-mono">command</code> to the full path in the JSON.
          </p>
        </div>

        {createdToken && !warnDismiss && createdProvider === "openrouter" ? (
          <div
            role="alert"
            className="mt-6 rounded-sm border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-foreground"
          >
            <p className="font-semibold">Copy your service account token now</p>
            <p className="mt-1 text-muted-foreground">
              This token is for the Hyperspeed API (tools, MCP, automation). Chat for this staff member uses the org{" "}
              <strong>OpenRouter</strong> key in the section above — not this token. You will not see this token again.
            </p>
            <pre className="mt-3 overflow-x-auto rounded-sm bg-background p-3 font-mono text-xs text-foreground">
              {createdToken}
            </pre>
            <div className="mt-3 flex flex-wrap gap-2">
              <button
                type="button"
                className="inline-flex items-center gap-2 rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                onClick={() => void navigator.clipboard.writeText(createdToken)}
              >
                <Copy className="h-4 w-4" />
                Copy token
              </button>
              <button
                type="button"
                className="rounded-sm border border-border px-3 py-2 text-sm hover:bg-accent"
                onClick={() => void navigator.clipboard.writeText(envBlock)}
              >
                Copy env block
              </button>
              <button
                type="button"
                className="ml-auto text-sm text-muted-foreground underline"
                onClick={() => {
                  setWarnDismiss(true);
                  setCreatedToken(null);
                  setCreatedProvider(null);
                }}
              >
                I’ve copied it — dismiss
              </button>
            </div>
            <p className="mt-3 text-xs text-muted-foreground">
              For Cursor MCP, use the <strong>Cursor MCP</strong> section above with this token in{" "}
              <code className="font-mono">HYPERSPEED_TOKEN</code>.
            </p>
          </div>
        ) : null}

        {createdToken && !warnDismiss && createdProvider === null ? (
          <div
            role="alert"
            className="mt-6 rounded-sm border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-foreground"
          >
            <p className="font-semibold">Copy your service account token now</p>
            <p className="mt-1 text-muted-foreground">
              You will not see this token again. For Cursor MCP, use the <strong>Cursor MCP</strong> section above.
            </p>
            <pre className="mt-3 overflow-x-auto rounded-sm bg-background p-3 font-mono text-xs text-foreground">
              {createdToken}
            </pre>
            <div className="mt-3 flex flex-wrap gap-2">
              <button
                type="button"
                className="inline-flex items-center gap-2 rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                onClick={() => void navigator.clipboard.writeText(createdToken)}
              >
                <Copy className="h-4 w-4" />
                Copy token
              </button>
              <button
                type="button"
                className="rounded-sm border border-border px-3 py-2 text-sm hover:bg-accent"
                onClick={() => void navigator.clipboard.writeText(envBlock)}
              >
                Copy env block
              </button>
              <button
                type="button"
                className="ml-auto text-sm text-muted-foreground underline"
                onClick={() => {
                  setWarnDismiss(true);
                  setCreatedToken(null);
                  setCreatedProvider(null);
                }}
              >
                I’ve copied it — dismiss
              </button>
            </div>
          </div>
        ) : null}

        {createdToken && !warnDismiss && createdProvider === "cursor" ? (
          <div
            role="alert"
            className="mt-6 rounded-sm border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-foreground"
          >
            <p className="font-semibold">Copy your service account token now</p>
            <p className="mt-1 text-muted-foreground">
              Paste it into Cursor → Settings → MCP (see JSON below). You will not see this token again.
            </p>
            <pre className="mt-3 overflow-x-auto rounded-sm bg-background p-3 font-mono text-xs text-foreground">
              {createdToken}
            </pre>
            <div className="mt-3 flex flex-wrap gap-2">
              <button
                type="button"
                className="inline-flex items-center gap-2 rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                onClick={() => void navigator.clipboard.writeText(createdToken)}
              >
                <Copy className="h-4 w-4" />
                Copy token
              </button>
              <button
                type="button"
                className="rounded-sm border border-border px-3 py-2 text-sm hover:bg-accent"
                onClick={() => void navigator.clipboard.writeText(envBlock)}
              >
                Copy env block
              </button>
              <button
                type="button"
                className="rounded-sm border border-border px-3 py-2 text-sm hover:bg-accent"
                onClick={() => void navigator.clipboard.writeText(cursorMcpJson)}
              >
                Copy Cursor MCP JSON
              </button>
              <button
                type="button"
                className="ml-auto text-sm text-muted-foreground underline"
                onClick={() => {
                  setWarnDismiss(true);
                  setCreatedToken(null);
                  setCreatedProvider(null);
                }}
              >
                I’ve copied it — dismiss
              </button>
            </div>
            <pre className="mt-4 max-h-48 overflow-auto rounded-sm border border-border bg-card p-3 font-mono text-xs text-muted-foreground">
              {cursorMcpJson}
            </pre>
            <p className="mt-2 text-xs text-muted-foreground">
              Build or install the <code className="font-mono">mcp-hyperspeed</code> binary from{" "}
              <code className="font-mono">apps/api/cmd/mcp-hyperspeed</code> and ensure it is on your PATH, or set{" "}
              <code className="font-mono">command</code> to the full path.
            </p>
          </div>
        ) : null}

        <div className="mt-8 flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-semibold text-foreground">Accounts</h2>
          <button
            type="button"
            className="rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            onClick={() => setCreateOpen(true)}
          >
            New service account
          </button>
        </div>

        {createOpen ? (
          <div className="mt-4 rounded-sm border border-border bg-card p-4">
            <h3 className="text-sm font-semibold text-foreground">Create</h3>
            <input
              className="mt-2 w-full max-w-md rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
              placeholder="Name (e.g. cursor-agent)"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
            />
            <label className="mt-3 block text-xs font-medium text-muted-foreground">Provider</label>
            <select
              className="mt-1 w-full max-w-md rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
              value={createProvider}
              onChange={(e) => setCreateProvider(e.target.value as ServiceAccountProvider)}
            >
              <option value="openrouter">OpenRouter (chat completions)</option>
              <option value="cursor">Cursor Cloud Agents</option>
            </select>
            {createProvider === "openrouter" ? (
              <div className="mt-3">
                <label className="text-xs font-medium text-muted-foreground">OpenRouter model id</label>
                <input
                  className="mt-1 w-full max-w-md rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                  placeholder="e.g. nvidia/nemotron-3-super-120b-a12b:free"
                  value={createOrModel}
                  onChange={(e) => setCreateOrModel(e.target.value)}
                />
                <p className="mt-1 text-xs text-muted-foreground">Powered by OpenRouter</p>
              </div>
            ) : (
              <div className="mt-3 space-y-2">
                <div>
                  <label className="text-xs font-medium text-muted-foreground">Default Git repo (HTTPS, optional)</label>
                  <input
                    className="mt-1 w-full max-w-md rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                    placeholder="https://github.com/org/repo — or leave empty and use IDE Source Control per space"
                    value={createRepo}
                    onChange={(e) => setCreateRepo(e.target.value)}
                  />
                </div>
                <div>
                  <label className="text-xs font-medium text-muted-foreground">Git ref (optional)</label>
                  <input
                    className="mt-1 w-full max-w-md rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                    placeholder="main"
                    value={createRef}
                    onChange={(e) => setCreateRef(e.target.value)}
                  />
                </div>
                <p className="text-xs text-muted-foreground">Powered by Cursor Cloud Agents</p>
              </div>
            )}
            <div className="mt-3 flex flex-wrap gap-2">
              <span className="text-xs text-muted-foreground">Roles (optional):</span>
              {rolesQ.data?.map((r) => (
                <label key={r.id} className="flex items-center gap-1 text-sm">
                  <input
                    type="checkbox"
                    checked={selectedRoleIds.includes(r.id)}
                    onChange={(e) => {
                      setSelectedRoleIds((prev) =>
                        e.target.checked ? [...prev, r.id] : prev.filter((x) => x !== r.id),
                      );
                    }}
                  />
                  {r.name}
                </label>
              ))}
            </div>
            <div className="mt-4 flex gap-2">
              <button
                type="button"
                className="rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                disabled={!newName.trim() || createM.isPending}
                onClick={() =>
                  createM.mutate({
                    name: newName.trim(),
                    role_ids: selectedRoleIds,
                    provider: createProvider,
                    ...(createProvider === "openrouter"
                      ? { openrouter_model: createOrModel.trim() || undefined }
                      : {
                          cursor_default_repo_url: createRepo.trim() || undefined,
                          cursor_default_ref: createRef.trim() || undefined,
                        }),
                  })
                }
              >
                Create
              </button>
              <button
                type="button"
                className="rounded-sm border border-border px-3 py-2 text-sm hover:bg-accent"
                onClick={() => setCreateOpen(false)}
              >
                Cancel
              </button>
            </div>
          </div>
        ) : null}

        <ul className="mt-4 divide-y divide-border rounded-sm border border-border">
          {(listQ.data ?? []).map((sa) => (
            <li
              key={sa.id}
              className="flex flex-wrap items-center justify-between gap-3 px-4 py-3 hover:bg-accent/50"
            >
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium text-foreground">{sa.name}</span>
                  <span className="rounded-sm bg-accent px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                    {(sa.provider ?? "openrouter") === "cursor" ? "Cursor" : "OpenRouter"}
                  </span>
                </div>
                <div className="font-mono text-xs text-muted-foreground">{sa.id}</div>
              </div>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  className="rounded-sm border border-border px-3 py-1.5 text-sm hover:bg-accent"
                  onClick={() => {
                    setSelectedSA(sa.id);
                    setProfileTouched(false);
                    setProfileDraft("");
                  }}
                >
                  Profile
                </button>
                <button
                  type="button"
                  className="rounded-sm border border-border p-2 text-red-600 hover:bg-accent"
                  title="Delete"
                  onClick={() => {
                    if (confirm(`Delete service account “${sa.name}”?`)) deleteM.mutate(sa.id);
                  }}
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            </li>
          ))}
        </ul>

        {selectedSA ? (
          <div className="mt-10 border-t border-border pt-8">
            <h2 className="text-lg font-semibold text-foreground">AI backend</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Chat mentions route to OpenRouter or Cursor Cloud Agents depending on provider.
            </p>
            <div className="mt-4 max-w-xl space-y-3 rounded-sm border border-border bg-card p-4">
              <label className="block text-xs font-medium text-muted-foreground">Provider</label>
              <select
                className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                value={editProvider}
                onChange={(e) => setEditProvider(e.target.value as ServiceAccountProvider)}
              >
                <option value="openrouter">OpenRouter</option>
                <option value="cursor">Cursor Cloud Agents</option>
              </select>
              {editProvider === "openrouter" ? (
                <div>
                  <label className="text-xs font-medium text-muted-foreground">Model id</label>
                  <input
                    className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                    value={editOrModel}
                    onChange={(e) => setEditOrModel(e.target.value)}
                  />
                </div>
              ) : (
                <>
                  <div>
                    <label className="text-xs font-medium text-muted-foreground">Default repo URL (optional if space has IDE Git)</label>
                    <input
                      className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                      placeholder="Empty = use this space’s Git remote from Source Control"
                      value={editRepo}
                      onChange={(e) => setEditRepo(e.target.value)}
                    />
                  </div>
                  <div>
                    <label className="text-xs font-medium text-muted-foreground">Git ref</label>
                    <input
                      className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                      value={editRef}
                      onChange={(e) => setEditRef(e.target.value)}
                    />
                  </div>
                </>
              )}
              <button
                type="button"
                className="rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                disabled={patchM.isPending || !selectedSA}
                onClick={() => {
                  if (!selectedSA) return;
                  if (editProvider === "openrouter") {
                    patchM.mutate({
                      serviceAccountId: selectedSA,
                      provider: "openrouter",
                      openrouter_model: editOrModel.trim(),
                    });
                  } else {
                    patchM.mutate({
                      serviceAccountId: selectedSA,
                      provider: "cursor",
                      cursor_default_repo_url: editRepo.trim(),
                      cursor_default_ref: editRef.trim() || undefined,
                    });
                  }
                }}
              >
                Save backend settings
              </button>
            </div>

            <h2 className="mt-10 text-lg font-semibold text-foreground">Agent profile (Markdown)</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Versioned instructions for this AI staff. Saving appends a new version.
            </p>
            {profileQ.isPending && (
              <p className="mt-2 text-xs text-muted-foreground" aria-live="polite">
                Loading profile…
              </p>
            )}
            {profileQ.isError && (
              <p className="mt-2 text-xs text-red-600" role="alert">
                Could not load profile: {(profileQ.error as Error)?.message ?? "error"}
              </p>
            )}
            <label className="mt-4 block text-xs font-medium text-muted-foreground" htmlFor="sa-profile-preset">
              Profile template
            </label>
            <select
              id="sa-profile-preset"
              className="mt-1 w-full max-w-md rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground disabled:opacity-60"
              value={profilePresetSelectValue}
              onChange={(e) => {
                const v = e.target.value;
                if (v === "custom") return;
                setProfileDraft(getPresetMarkdown(v as ApplyablePresetId));
                setProfileTouched(true);
              }}
              disabled={profileQ.isPending && !profileQ.data}
            >
              {PROFILE_PRESET_OPTIONS.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.label}
                </option>
              ))}
              <option value="custom" disabled>
                Custom (edited)
              </option>
            </select>
            <p className="mt-1 max-w-xl text-xs text-muted-foreground">
              Choosing a template replaces the editor contents. You can edit freely before saving.
            </p>
            <textarea
              className="mt-4 min-h-[200px] w-full rounded-sm border border-input bg-background p-3 font-mono text-sm text-foreground disabled:opacity-60"
              value={profileTouched ? profileDraft : profileDisplayMd}
              onChange={(e) => {
                setProfileTouched(true);
                setProfileDraft(e.target.value);
              }}
              placeholder={
                profileQ.isPending && !profileQ.data
                  ? "Loading profile…"
                  : "Describe specialization, tools usage, and boundaries…"
              }
              disabled={profileQ.isPending && !profileQ.data}
            />
            <button
              type="button"
              className="mt-3 rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
              disabled={saveProfileM.isPending || (profileQ.isPending && !profileQ.data)}
              onClick={() => saveProfileM.mutate()}
            >
              Save new version
            </button>
            <div className="mt-6">
              <h3 className="text-sm font-semibold text-foreground">Recent versions</h3>
              <ul className="mt-2 text-xs text-muted-foreground">
                {(versionsQ.data?.versions ?? []).slice(0, 10).map((v) => (
                  <li key={v.id}>
                    v{v.version} — {new Date(v.created_at).toLocaleString()}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
}
