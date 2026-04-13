import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FormEvent, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { apiFetch } from "../api/http";
import type { Organization, OrgFeatures, SignupRequestRow } from "../api/types";

function buildIntendedUrlSaveActivity(org: Organization): string[] {
  const lines: string[] = [
    `[${new Date().toLocaleString()}]`,
    org.intended_public_url
      ? "Saved your team URL."
      : "Cleared your team URL.",
  ];

  lines.push(
    "Only your saved URL was updated. Configure DNS at your domain provider and HTTPS at your server or reverse proxy.",
  );

  if (org.intended_public_url) {
    try {
      const origin = new URL(org.intended_public_url).origin;
      const here = window.location.origin;
      const override = org.public_origin_override?.trim();
      const overrideNorm = override
        ? override.replace(/\/$/, "").toLowerCase()
        : "";
      const intendedNorm = origin.replace(/\/$/, "").toLowerCase();
      const serverAligned =
        overrideNorm !== "" && overrideNorm === intendedNorm;
      if (origin === here) {
        lines.push(`You’re viewing this page from the same address as your team URL.`);
      } else if (serverAligned) {
        lines.push(
          `You’re on ${here} but your team URL is ${origin}. The server is set to accept that team URL for browser access and preview links.`,
        );
      } else {
        lines.push(
          `You’re on ${here} but your team URL is ${origin}. Turn on “Trust this team URL on the server” when you save if you want the server to accept that address.`,
        );
      }
    } catch {
      // ignore malformed URL
    }
  }

  return lines;
}

export default function OrgSettingsPage() {
  const { orgId } = useParams<{ orgId: string }>();
  const qc = useQueryClient();
  if (!orgId) return null;

  const myPermsQ = useQuery({
    queryKey: ["myOrgPermissions", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/me/permissions`,
      );
      if (!res.ok) throw new Error("permissions");
      return (await res.json()) as { permissions: string[] };
    },
  });

  const permSet = myPermsQ.data?.permissions ?? [];
  const canOrgManage = permSet.includes("org.manage");
  const canMembersManage = permSet.includes("org.members.manage");
  const canSpaceMembersManage = permSet.includes("space.members.manage");

  const featuresQ = useQuery({
    queryKey: ["org-features", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/features`);
      if (!res.ok) throw new Error("features");
      const j = (await res.json()) as { features: OrgFeatures };
      return j.features;
    },
  });

  const signupReqQ = useQuery({
    queryKey: ["signup-requests", orgId],
    enabled: !!orgId && myPermsQ.isSuccess && canMembersManage,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/signup-requests`,
      );
      if (!res.ok) throw new Error("signup requests");
      const j = (await res.json()) as { signup_requests: SignupRequestRow[] };
      return j.signup_requests;
    },
  });

  const orgQ = useQuery({
    queryKey: ["org", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}`);
      if (!res.ok) throw new Error("org");
      return (await res.json()) as Organization;
    },
  });

  const [intendedUrl, setIntendedUrl] = useState("");
  useEffect(() => {
    setIntendedUrl(orgQ.data?.intended_public_url ?? "");
  }, [orgQ.data?.intended_public_url]);

  /** When true, PATCH sends sync_runtime_origin so the API updates DB-backed CORS / preview base. */
  const [syncRuntimeOrigin, setSyncRuntimeOrigin] = useState(true);
  const [intendedUrlErr, setIntendedUrlErr] = useState<string | null>(null);
  const [intendedUrlActivity, setIntendedUrlActivity] = useState<{
    at: number;
    lines: string[];
  } | null>(null);

  const patchOrg = useMutation({
    mutationFn: async (body: {
      intended_public_url: string | null;
      sync_runtime_origin?: boolean;
    }) => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}`, {
        method: "PATCH",
        json: body,
      });
      if (!res.ok) {
        const j = (await res.json().catch(() => ({}))) as { error?: string };
        throw new Error(j.error || "patch organization");
      }
      return (await res.json()) as Organization;
    },
    onSuccess: (o) => {
      qc.setQueryData(["org", orgId], o);
      void qc.invalidateQueries({ queryKey: ["orgs"] });
      setIntendedUrlActivity({
        at: Date.now(),
        lines: buildIntendedUrlSaveActivity(o),
      });
    },
  });

  function onSaveIntendedUrl(e: FormEvent) {
    e.preventDefault();
    setIntendedUrlErr(null);
    const trimmed = intendedUrl.trim();
    if (!trimmed) {
      patchOrg.mutate({
        intended_public_url: null,
        sync_runtime_origin: syncRuntimeOrigin,
      });
      return;
    }
    const low = trimmed.toLowerCase();
    if (!low.startsWith("http://") && !low.startsWith("https://")) {
      setIntendedUrlErr(
        "Use a full URL that starts with http:// or https://.",
      );
      return;
    }
    patchOrg.mutate({
      intended_public_url: trimmed,
      sync_runtime_origin: syncRuntimeOrigin,
    });
  }

  const patchFeatures = useMutation({
    mutationFn: async (patch: Partial<OrgFeatures>) => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/features`, {
        method: "PATCH",
        json: patch,
      });
      if (!res.ok) throw new Error("patch features");
      const j = (await res.json()) as { features: OrgFeatures };
      return j.features;
    },
    onSuccess: (features) => {
      qc.setQueryData(["org-features", orgId], features);
    },
  });

  const approveMut = useMutation({
    mutationFn: async (requestId: string) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/signup-requests/${requestId}/approve`,
        { method: "POST" },
      );
      if (!res.ok) throw new Error("approve");
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["signup-requests", orgId] });
    },
  });

  const denyMut = useMutation({
    mutationFn: async (requestId: string) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/signup-requests/${requestId}/deny`,
        { method: "POST" },
      );
      if (!res.ok) throw new Error("deny");
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["signup-requests", orgId] });
    },
  });

  const fe = featuresQ.data;
  const pending = signupReqQ.data ?? [];
  const permsReady = myPermsQ.isSuccess;
  const noSettingsAccess =
    permsReady && !canOrgManage && !canMembersManage;

  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-background">
      <div className="mx-auto max-w-4xl px-4 py-8">
        <header className="border-b border-border pb-6">
          <Link to={`/o/${orgId}`} className="text-xs text-link hover:underline">
            ← Back to spaces
          </Link>
          <p className="mt-2 text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
            Settings
          </p>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight text-foreground">
            Workspace settings
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Manage members, roles, and space access for this workspace.
          </p>
        </header>

        {myPermsQ.isLoading ? (
          <p className="mt-6 text-sm text-muted-foreground">Loading permissions…</p>
        ) : null}
        {myPermsQ.isError ? (
          <p className="mt-6 text-sm text-destructive">
            Could not load your permissions for this workspace.
          </p>
        ) : null}
        {noSettingsAccess ? (
          <p className="mt-6 text-sm text-muted-foreground">
            You don&apos;t have permission to change workspace settings. Ask an administrator
            to grant{" "}
            <span className="font-mono text-xs">org.manage</span> or{" "}
            <span className="font-mono text-xs">org.members.manage</span> on your role.
          </p>
        ) : null}

        {permsReady ? (
        <div className="mt-8 grid grid-cols-1 gap-4 sm:grid-cols-2">
          {canOrgManage ? (
          <div className="rounded-sm border border-border bg-card p-4 sm:col-span-2">
            <div className="text-sm font-semibold text-foreground">Domain &amp; URL</div>
            <p className="mt-1 text-sm text-muted-foreground">
              The address where your team will use Hyperspeed in the browser (for example when
              you&apos;re not on localhost). Save it here, optionally tell the server to trust
              that address. DNS and HTTPS are managed by your host or domain provider.
            </p>
            <p className="mt-2 text-xs text-muted-foreground">
              Current browser origin:{" "}
              <span className="font-mono break-all">{window.location.origin}</span>
            </p>
            {orgQ.isError ? (
              <p className="mt-3 text-sm text-destructive">
                Could not load workspace details. You may need workspace admin permission.
              </p>
            ) : (
              <form className="mt-4 space-y-3" onSubmit={onSaveIntendedUrl}>
                <label className="block text-xs font-medium text-muted-foreground">
                  Team URL
                </label>
                <p className="mt-1 text-xs text-muted-foreground">
                  Enter the full URL your team should use, for example{" "}
                  <span className="font-mono text-[11px]">https://app.example.com</span>.
                </p>
                <input
                  className="w-full max-w-xl rounded-sm border border-input bg-background px-3 py-2 text-sm"
                  placeholder="https://app.example.com"
                  value={intendedUrl}
                  onChange={(e) => {
                    setIntendedUrlActivity(null);
                    setIntendedUrl(e.target.value);
                  }}
                  disabled={patchOrg.isPending || orgQ.isPending}
                  autoComplete="off"
                  spellCheck={false}
                />
                <label className="flex cursor-pointer items-start gap-2 text-sm text-foreground">
                  <input
                    type="checkbox"
                    className="mt-0.5"
                    checked={syncRuntimeOrigin}
                    onChange={(e) => {
                      setSyncRuntimeOrigin(e.target.checked);
                      setIntendedUrlActivity(null);
                    }}
                    disabled={patchOrg.isPending || orgQ.isPending}
                  />
                  <span>
                    Trust this team URL on the server (recommended). Lets people use that
                    address in the browser without extra server configuration.
                  </span>
                </label>
                <div className="flex flex-wrap gap-2">
                  <button
                    type="submit"
                    disabled={patchOrg.isPending || orgQ.isPending}
                    className="rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                  >
                    {patchOrg.isPending ? "Saving…" : "Save URL"}
                  </button>
                  <button
                    type="button"
                    className="rounded-sm border border-border px-3 py-1.5 text-sm disabled:opacity-50"
                    disabled={patchOrg.isPending || orgQ.isPending}
                    onClick={() => {
                      setIntendedUrlErr(null);
                      setIntendedUrlActivity(null);
                      setIntendedUrl("");
                      patchOrg.mutate({
                        intended_public_url: null,
                        sync_runtime_origin: syncRuntimeOrigin,
                      });
                    }}
                  >
                    Clear
                  </button>
                </div>
                {intendedUrlErr ? (
                  <p className="text-sm text-destructive">{intendedUrlErr}</p>
                ) : null}
                {patchOrg.isError ? (
                  <p className="text-sm text-destructive">
                    {patchOrg.error instanceof Error
                      ? patchOrg.error.message
                      : "patch organization"}
                  </p>
                ) : null}
                {intendedUrlActivity ? (
                  <div className="rounded-sm border border-border bg-muted/30 p-3 font-mono text-xs leading-relaxed text-foreground">
                    <div className="mb-2 flex items-center justify-between gap-2">
                      <span className="text-[10px] font-sans font-medium uppercase tracking-wide text-muted-foreground">
                        Last save
                      </span>
                      <button
                        type="button"
                        className="font-sans text-[11px] text-muted-foreground underline decoration-dotted hover:text-foreground"
                        onClick={() => setIntendedUrlActivity(null)}
                      >
                        Dismiss
                      </button>
                    </div>
                    {intendedUrlActivity.lines.map((line, i) => (
                      <div key={`${intendedUrlActivity.at}-${i}`}>{line}</div>
                    ))}
                  </div>
                ) : null}
              </form>
            )}
          </div>
          ) : permsReady && !canOrgManage ? (
            <div className="rounded-sm border border-border bg-card p-4 sm:col-span-2">
              <div className="text-sm font-semibold text-foreground">Domain &amp; URL</div>
              <p className="mt-2 text-sm text-muted-foreground">
                Only workspace administrators with{" "}
                <span className="font-mono text-xs">org.manage</span> can edit the team URL and
                trusted origin for this workspace.
              </p>
            </div>
          ) : null}

          {canMembersManage ? (
          <Link
            to={`/o/${orgId}/roles`}
            className="rounded-sm border border-border bg-card p-4 hover:bg-accent/20"
          >
            <div className="text-sm font-semibold text-foreground">Members & roles</div>
            <div className="mt-1 text-sm text-muted-foreground">
              Create roles and assign them to workspace members.
            </div>
          </Link>
          ) : null}

          {canSpaceMembersManage ? (
          <Link
            to={`/o/${orgId}/settings/spaces`}
            className="rounded-sm border border-border bg-card p-4 hover:bg-accent/20"
          >
            <div className="text-sm font-semibold text-foreground">Spaces management</div>
            <div className="mt-1 text-sm text-muted-foreground">
              Choose which roles can access each space.
            </div>
          </Link>
          ) : null}

          {canMembersManage ? (
          <Link
            to={`/o/${orgId}/settings/service-accounts`}
            className="rounded-sm border border-border bg-card p-4 hover:bg-accent/20 sm:col-span-2"
          >
            <div className="text-sm font-semibold text-foreground">AI staff &amp; MCP</div>
            <div className="mt-1 text-sm text-muted-foreground">
              Service accounts, Cursor &amp; OpenRouter org API keys, agent profiles, and MCP setup.
            </div>
          </Link>
          ) : null}

          {canOrgManage ? (
          <div className="rounded-sm border border-border bg-card p-4 sm:col-span-2">
            <div className="text-sm font-semibold text-foreground">Datasets feature</div>
            <div className="mt-1 text-sm text-muted-foreground">
              Enable tabular datasets (CSV/Parquet) pages and APIs for this workspace.
            </div>
            <label className="mt-3 inline-flex cursor-pointer items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={!!fe?.datasets_enabled}
                disabled={featuresQ.isPending || patchFeatures.isPending}
                onChange={(e) =>
                  patchFeatures.mutate({ datasets_enabled: e.target.checked })
                }
              />
              Enabled
            </label>
          </div>
          ) : null}

          {canOrgManage ? (
          <div className="rounded-sm border border-border bg-card p-4 sm:col-span-2">
            <div className="text-sm font-semibold text-foreground">Open registration</div>
            <div className="mt-1 text-sm text-muted-foreground">
              When enabled, new users can register and appear in the approval queue below.
              When disabled, only invited users can join (existing accounts can still sign in).
            </div>
            <label className="mt-3 inline-flex cursor-pointer items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={fe?.open_signups_enabled !== false}
                disabled={featuresQ.isPending || patchFeatures.isPending}
                onChange={(e) =>
                  patchFeatures.mutate({ open_signups_enabled: e.target.checked })
                }
              />
              Allow open sign-ups
            </label>
          </div>
          ) : null}

          {canMembersManage ? (
          <div className="rounded-sm border border-border bg-card p-4 sm:col-span-2">
            <div className="text-sm font-semibold text-foreground">
              Pending sign-up requests
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              Approve or deny people who registered while open sign-ups were enabled.
            </p>
            {signupReqQ.isLoading ? (
              <p className="mt-3 text-sm text-muted-foreground">Loading…</p>
            ) : signupReqQ.isError ? (
              <p className="mt-3 text-sm text-destructive">
                Could not load requests. You may need member-management permission.
              </p>
            ) : pending.length === 0 ? (
              <p className="mt-3 text-sm text-muted-foreground">No pending requests.</p>
            ) : (
              <ul className="mt-4 space-y-3">
                {pending.map((r) => (
                  <li
                    key={r.id}
                    className="flex flex-wrap items-center justify-between gap-2 rounded-sm border border-border px-3 py-2"
                  >
                    <div>
                      <div className="text-sm font-medium text-foreground">
                        {r.display_name?.trim() || r.email}
                      </div>
                      <div className="font-mono text-xs text-muted-foreground">
                        {r.email}
                      </div>
                    </div>
                    <div className="flex gap-2">
                      <button
                        type="button"
                        className="rounded-sm bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground disabled:opacity-50"
                        disabled={approveMut.isPending || denyMut.isPending}
                        onClick={() => approveMut.mutate(r.id)}
                      >
                        Approve
                      </button>
                      <button
                        type="button"
                        className="rounded-sm border border-border px-3 py-1.5 text-xs disabled:opacity-50"
                        disabled={approveMut.isPending || denyMut.isPending}
                        onClick={() => denyMut.mutate(r.id)}
                      >
                        Deny
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
          ) : null}
        </div>
        ) : null}
      </div>
    </div>
  );
}
