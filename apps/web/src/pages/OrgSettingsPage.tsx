import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FormEvent, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { apiFetch } from "../api/http";
import { fetchPublicInstance, type PublicInstance } from "../api/instance";
import type { Organization, OrgFeatures, SignupRequestRow } from "../api/types";
import { HyperspeedTeamUrlInput } from "../components/HyperspeedTeamUrlInput";
import {
  GIFTED_SUBDOMAIN_APEX,
  GIFTED_TEAM_WWW_PREFIX,
  intendedUrlFromTeamSubdomain,
  parseTeamSubdomainFromIntendedUrl,
} from "../constants/giftedDomain";

function buildIntendedUrlSaveActivity(
  org: Organization,
  inst: PublicInstance | undefined,
  opts?: { provisionedThisRequest?: boolean },
): string[] {
  const intendedSub = parseTeamSubdomainFromIntendedUrl(org.intended_public_url);
  const giftedRaw = org.gifted_subdomain_slug?.trim();
  const dnsAligned =
    !!intendedSub &&
    !!giftedRaw &&
    intendedSub.toLowerCase() === giftedRaw.toLowerCase();
  const canAutoDNS = !!inst?.provisioning_enabled;

  const lines: string[] = [
    `[${new Date().toLocaleString()}]`,
    org.intended_public_url
      ? "Saved your team URL."
      : "Cleared your team URL.",
  ];

  if (opts?.provisionedThisRequest) {
    lines.push(
      "A public DNS record was created for this address pointing to the IP you entered.",
    );
    lines.push(
      "HTTPS still depends on how your server or reverse proxy is set up for that hostname.",
    );
  } else if (dnsAligned) {
    lines.push(
      "DNS for this workspace matches this team URL.",
    );
    lines.push(
      "HTTPS still depends on how your server or reverse proxy is set up for that hostname.",
    );
  } else if (canAutoDNS) {
    lines.push(
      "Only your saved URL was updated. To also publish DNS, turn on “Create the DNS record…” below, enter your public IP, and use Save URL & DNS.",
    );
  } else {
    lines.push(
      "Only your saved URL was updated. This server can’t create DNS for you—add a record at your domain host, or ask your administrator to enable automatic team DNS.",
    );
  }

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
      if (intendedSub) {
        const publicUrl = `https://${GIFTED_TEAM_WWW_PREFIX}${intendedSub}.${GIFTED_SUBDOMAIN_APEX}`;
        if (canAutoDNS) {
          lines.push(
            `That hostname (${publicUrl}) must exist in public DNS before it opens in a browser—use Save URL & DNS if you enabled automatic DNS, or add DNS yourself.`,
          );
        } else {
          lines.push(
            `That hostname (${publicUrl}) must exist in public DNS before it opens in a browser.`,
          );
        }
      }
    } catch {
      // ignore malformed URL
    }
  }

  if (inst?.provisioning_enabled) {
    const gifted = org.gifted_subdomain_slug?.trim();
    const base = inst.provisioning_base_domain;
    if (base && gifted) {
      const url = `https://${GIFTED_TEAM_WWW_PREFIX}${gifted}.${base}`;
      if (!dnsAligned) {
        lines.push(`DNS on file for this workspace: ${url}`);
      }
      if (intendedSub && gifted) {
        if (intendedSub.toLowerCase() !== gifted.toLowerCase()) {
          lines.push(
            `Your team name in the URL (${intendedSub}) doesn’t match the DNS name on file (${gifted}).`,
          );
        }
      }
    } else if (!dnsAligned) {
      lines.push(
        "No automatic DNS record for this workspace yet—use Save URL & DNS above if that option is available.",
      );
    }
  }

  return lines;
}

export default function OrgSettingsPage() {
  const { orgId } = useParams<{ orgId: string }>();
  const qc = useQueryClient();
  if (!orgId) return null;

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
    enabled: !!orgId,
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

  const instanceQ = useQuery({
    queryKey: ["public-instance"],
    queryFn: fetchPublicInstance,
  });

  const [teamSubdomain, setTeamSubdomain] = useState("");
  useEffect(() => {
    setTeamSubdomain(
      parseTeamSubdomainFromIntendedUrl(orgQ.data?.intended_public_url),
    );
  }, [orgQ.data?.intended_public_url]);

  const [provisionDns, setProvisionDns] = useState(false);
  /** When true, PATCH sends sync_runtime_origin so the API updates DB-backed CORS / preview base. */
  const [syncRuntimeOrigin, setSyncRuntimeOrigin] = useState(true);
  const [publicIPv4, setPublicIPv4] = useState("");
  const [intendedUrlErr, setIntendedUrlErr] = useState<string | null>(null);
  const [claimErr, setClaimErr] = useState<string | null>(null);
  const [intendedUrlActivity, setIntendedUrlActivity] = useState<{
    at: number;
    lines: string[];
  } | null>(null);

  const patchOrg = useMutation({
    mutationFn: async (body: {
      intended_public_url: string | null;
      provision_gifted_dns?: boolean;
      public_ipv4?: string;
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
    onSuccess: (o, variables) => {
      qc.setQueryData(["org", orgId], o);
      void qc.invalidateQueries({ queryKey: ["orgs"] });
      const inst = qc.getQueryData<PublicInstance>(["public-instance"]);
      const provisionedThisRequest =
        !!variables.provision_gifted_dns && variables.intended_public_url != null;
      setIntendedUrlActivity({
        at: Date.now(),
        lines: buildIntendedUrlSaveActivity(o, inst, {
          provisionedThisRequest,
        }),
      });
    },
  });

  const revokeSubdomain = useMutation({
    mutationFn: async (slug: string) => {
      const res = await apiFetch(
        `/api/v1/provisioning/claim/${encodeURIComponent(slug)}`,
        { method: "DELETE" },
      );
      if (!res.ok) {
        const j = (await res.json().catch(() => ({}))) as { error?: string };
        throw new Error(j.error || "revoke_failed");
      }
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["org", orgId] });
      void qc.invalidateQueries({ queryKey: ["orgs"] });
      setClaimErr(null);
    },
  });

  function mapClaimError(code: string): string {
    switch (code) {
      case "invalid_slug":
        return "Subdomain is invalid or reserved.";
      case "invalid_ipv4":
        return "Enter a valid public IPv4 for DNS.";
      case "slug_taken":
        return "That subdomain is already claimed.";
      case "rate_limited":
        return "Too many provisioning requests. Try again later.";
      case "provision_gifted_dns requires intended_public_url":
        return "Intended URL is required to create DNS.";
      case "public_ipv4 required when provision_gifted_dns is true":
        return "Enter your server’s public IPv4 to create the DNS record.";
      case "intended_public_url must be https://www.{subdomain}.hyperspeedapp.com":
        return "Use the team URL form above (https://www.…hyperspeedapp.com).";
      case "provisioning_unavailable":
        return "Subdomain provisioning is not available.";
      default:
        return code;
    }
  }

  function onSaveIntendedUrl(e: FormEvent) {
    e.preventDefault();
    setIntendedUrlErr(null);
    const t = teamSubdomain.trim();
    if (!t) {
      setProvisionDns(false);
      patchOrg.mutate({
        intended_public_url: null,
        sync_runtime_origin: syncRuntimeOrigin,
      });
      return;
    }
    const full = intendedUrlFromTeamSubdomain(t);
    if (!full) {
      setIntendedUrlErr(
        "Use a valid subdomain: letters, numbers, hyphens (not at the start or end).",
      );
      return;
    }
    const inst = instanceQ.data;
    if (provisionDns && inst?.provisioning_enabled) {
      const ip = publicIPv4.trim();
      if (!ip) {
        setIntendedUrlErr(
          "Enter your server’s public IPv4 to create the DNS record.",
        );
        return;
      }
      patchOrg.mutate({
        intended_public_url: full,
        provision_gifted_dns: true,
        public_ipv4: ip,
        sync_runtime_origin: syncRuntimeOrigin,
      });
      return;
    }
    patchOrg.mutate({
      intended_public_url: full,
      sync_runtime_origin: syncRuntimeOrigin,
    });
  }

  function onRevoke() {
    const slug = orgQ.data?.gifted_subdomain_slug?.trim();
    if (!slug) return;
    if (
      !window.confirm(
        `Remove DNS for ${slug} and clear it from this workspace?`,
      )
    ) {
      return;
    }
    setClaimErr(null);
    revokeSubdomain.mutate(slug, {
      onError: (err) => {
        setClaimErr(
          mapClaimError(
            err instanceof Error ? err.message : "Revoke failed",
          ),
        );
      },
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
  const inst = instanceQ.data;
  const org = orgQ.data;
  const giftedHost =
    inst?.provisioning_base_domain && org?.gifted_subdomain_slug
      ? `https://${GIFTED_TEAM_WWW_PREFIX}${org.gifted_subdomain_slug}.${inst.provisioning_base_domain}`
      : null;

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

        <div className="mt-8 grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="rounded-sm border border-border bg-card p-4 sm:col-span-2">
            <div className="text-sm font-semibold text-foreground">Domain &amp; URL</div>
            <p className="mt-1 text-sm text-muted-foreground">
              The address where your team will use Hyperspeed in the browser (for example when
              you&apos;re not on localhost). Save it here, optionally tell the server to trust
              that address, and—if your host supports it—create the public DNS record in one
              step.
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
                  The label you type becomes{" "}
                  <span className="font-mono text-[11px]">
                    {`https://www.<name>.${GIFTED_SUBDOMAIN_APEX}`}
                  </span>
                  .
                </p>
                <HyperspeedTeamUrlInput
                  value={teamSubdomain}
                  onChange={(v) => {
                    setIntendedUrlActivity(null);
                    setTeamSubdomain(v);
                  }}
                  disabled={patchOrg.isPending || orgQ.isPending}
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
                <div className="space-y-2 pt-1">
                  <div className="text-xs font-medium text-muted-foreground">
                    Automatic DNS (optional)
                  </div>
                  {inst?.provisioning_enabled ? (
                    <>
                      <p className="text-xs text-muted-foreground">
                        If this Hyperspeed server is set up for it, you can publish a public DNS
                        name in one step. The primary button becomes{" "}
                        <strong className="font-medium text-foreground">Save URL &amp; DNS</strong>{" "}
                        when the box below is checked.
                      </p>
                      <label className="flex cursor-pointer items-start gap-2 text-sm text-foreground">
                        <input
                          type="checkbox"
                          className="mt-0.5"
                          checked={provisionDns}
                          onChange={(e) => {
                            setProvisionDns(e.target.checked);
                            setIntendedUrlErr(null);
                          }}
                          disabled={patchOrg.isPending || orgQ.isPending}
                        />
                        <span>
                          Create the DNS record for this address (use your server&apos;s public
                          IPv4—the address the internet uses to reach this app)
                        </span>
                      </label>
                      {provisionDns ? (
                        <input
                          className="w-full max-w-xl rounded-sm border border-input bg-background px-3 py-2 text-sm"
                          placeholder="Public IPv4 (e.g. 203.0.113.10)"
                          value={publicIPv4}
                          onChange={(e) => {
                            setPublicIPv4(e.target.value);
                            setIntendedUrlErr(null);
                          }}
                          autoComplete="off"
                          disabled={patchOrg.isPending || orgQ.isPending}
                        />
                      ) : null}
                    </>
                  ) : (
                    <div className="rounded-sm border border-border bg-muted/20 p-3 text-xs leading-relaxed text-muted-foreground">
                      <p className="text-foreground">
                        <strong className="font-medium text-foreground">Hyperspeed-hosted DNS</strong>{" "}
                        (one-step <strong className="font-medium text-foreground">Save URL &amp; DNS</strong>)
                        appears when this server is linked to Hyperspeed&apos;s provisioning service.
                        Until then you only need <strong className="font-medium text-foreground">Save URL</strong>.
                      </p>
                      <p className="mt-2">
                        You can still save your team URL here. For your own domain, add DNS at your
                        provider; reload this page after your host finishes linking the install.
                      </p>
                    </div>
                  )}
                </div>
                <div className="flex flex-wrap gap-2">
                  <button
                    type="submit"
                    disabled={
                      patchOrg.isPending ||
                      orgQ.isPending ||
                      (!!inst?.provisioning_enabled &&
                        provisionDns &&
                        teamSubdomain.trim() !== "" &&
                        !publicIPv4.trim())
                    }
                    className="rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                  >
                    {patchOrg.isPending
                      ? "Saving…"
                      : inst?.provisioning_enabled && provisionDns
                        ? "Save URL & DNS"
                        : "Save URL"}
                  </button>
                  <button
                    type="button"
                    className="rounded-sm border border-border px-3 py-1.5 text-sm disabled:opacity-50"
                    disabled={patchOrg.isPending || orgQ.isPending}
                    onClick={() => {
                      setIntendedUrlErr(null);
                      setIntendedUrlActivity(null);
                      setProvisionDns(false);
                      setPublicIPv4("");
                      setTeamSubdomain("");
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
                    {mapClaimError(
                      patchOrg.error instanceof Error
                        ? patchOrg.error.message
                        : "patch organization",
                    )}
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

            {inst?.provisioning_enabled ? (
              <div className="mt-6 border-t border-border pt-4">
                <div className="text-sm font-medium text-foreground">
                  DNS status
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  Records are created at{" "}
                  <span className="font-mono">
                    {`https://${GIFTED_TEAM_WWW_PREFIX}<subdomain>.${GIFTED_SUBDOMAIN_APEX}`}
                  </span>{" "}
                  when you use <strong>Save URL &amp; DNS</strong> above (with automatic DNS
                  turned on).
                </p>
                {giftedHost ? (
                  <p className="mt-2 text-sm">
                    <span className="text-muted-foreground">Configured: </span>
                    <span className="font-mono break-all">{giftedHost}</span>
                  </p>
                ) : (
                  <p className="mt-2 text-xs text-muted-foreground">
                    No gifted subdomain recorded for this workspace yet.
                  </p>
                )}
                {org?.gifted_subdomain_slug ? (
                  <div className="mt-3">
                    <button
                      type="button"
                      disabled={revokeSubdomain.isPending}
                      className="rounded-sm border border-destructive/50 px-3 py-1.5 text-sm text-destructive hover:bg-destructive/10 disabled:opacity-50"
                      onClick={onRevoke}
                    >
                      {revokeSubdomain.isPending ? "Revoking…" : "Revoke DNS"}
                    </button>
                  </div>
                ) : null}
                {claimErr ? (
                  <p className="mt-2 text-sm text-destructive">{claimErr}</p>
                ) : null}
                {revokeSubdomain.isError ? (
                  <p className="mt-2 text-sm text-destructive">
                    {mapClaimError(
                      revokeSubdomain.error instanceof Error
                        ? revokeSubdomain.error.message
                        : "Revoke failed",
                    )}
                  </p>
                ) : null}
              </div>
            ) : null}
          </div>

          <Link
            to={`/o/${orgId}/roles`}
            className="rounded-sm border border-border bg-card p-4 hover:bg-accent/20"
          >
            <div className="text-sm font-semibold text-foreground">Members & roles</div>
            <div className="mt-1 text-sm text-muted-foreground">
              Create roles and assign them to workspace members.
            </div>
          </Link>

          <Link
            to={`/o/${orgId}/settings/spaces`}
            className="rounded-sm border border-border bg-card p-4 hover:bg-accent/20"
          >
            <div className="text-sm font-semibold text-foreground">Spaces management</div>
            <div className="mt-1 text-sm text-muted-foreground">
              Choose which roles can access each space.
            </div>
          </Link>

          <Link
            to={`/o/${orgId}/settings/service-accounts`}
            className="rounded-sm border border-border bg-card p-4 hover:bg-accent/20 sm:col-span-2"
          >
            <div className="text-sm font-semibold text-foreground">AI staff &amp; MCP</div>
            <div className="mt-1 text-sm text-muted-foreground">
              Service accounts, Cursor &amp; OpenRouter org API keys, agent profiles, and MCP setup.
            </div>
          </Link>

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
        </div>
      </div>
    </div>
  );
}
