import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";
import { Link, useParams } from "react-router-dom";
import { apiFetch } from "../api/http";
import type { Project, RoleWithPermissions, UUID } from "../api/types";

type AccessResp = { role_ids: UUID[] };

export default function OrgSpacesAccessPage() {
  const { orgId } = useParams<{ orgId: string }>();
  const qc = useQueryClient();

  const spacesQ = useQuery({
    queryKey: ["projects", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces`);
      if (res.status === 403) {
        return { spaces: [] as Project[], forbidden: true as const };
      }
      if (!res.ok) throw new Error("projects");
      const j = (await res.json()) as { spaces: Project[] };
      return { spaces: j.spaces, forbidden: false as const };
    },
  });

  const rolesQ = useQuery({
    queryKey: ["roles", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/roles`);
      if (!res.ok) throw new Error("roles");
      const j = (await res.json()) as { roles: RoleWithPermissions[] };
      return j.roles;
    },
  });

  const accessQs = useQueries({
    queries: (spacesQ.data?.spaces ?? []).map((p) => ({
      queryKey: ["space-access", orgId, p.id] as const,
      enabled: !!orgId && !!p.id,
      queryFn: async () => {
        const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${p.id}/access`);
        if (!res.ok) throw new Error("access");
        return res.json() as Promise<AccessResp>;
      },
    })),
  });

  const accessByProjectId = useMemo(() => {
    const m = new Map<UUID, UUID[]>();
    const spaces = spacesQ.data?.spaces ?? [];
    for (let i = 0; i < spaces.length; i++) {
      const pid = spaces[i]?.id;
      const data = accessQs[i]?.data;
      if (pid && data) {
        m.set(pid, data.role_ids ?? []);
      }
    }
    return m;
  }, [spacesQ.data, accessQs]);

  const putAccess = useMutation({
    mutationFn: async (vars: { projectId: UUID; roleIds: UUID[] }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${vars.projectId}/access`,
        { method: "PUT", json: { role_ids: vars.roleIds } },
      );
      if (!res.ok) throw new Error("put access");
      return res.json() as Promise<AccessResp>;
    },
    onSuccess: async (_data, vars) => {
      await qc.invalidateQueries({ queryKey: ["space-access", orgId, vars.projectId] });
      await qc.invalidateQueries({ queryKey: ["projects-access-summary", orgId] });
    },
  });

  if (!orgId) return null;

  const roles = rolesQ.data ?? [];
  const spaces = spacesQ.data?.spaces ?? [];
  const spacesForbidden = spacesQ.data?.forbidden === true;

  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-background">
      <div className="mx-auto max-w-5xl px-4 py-8">
        <header className="border-b border-border pb-6">
          <Link to={`/o/${orgId}/settings`} className="text-xs text-link hover:underline">
            ← Back to settings
          </Link>
          <p className="mt-2 text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
            Settings
          </p>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight text-foreground">
            Spaces management
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Choose which roles can access each space. If a space has no roles selected, any member
            who has workspace access (for example <span className="font-mono text-xs">board.read</span>)
            may open it.
          </p>
        </header>

        {spacesForbidden ? (
          <p className="mt-8 text-sm text-muted-foreground">
            You can&apos;t load spaces until an owner assigns you a role that includes workspace
            access.
          </p>
        ) : null}

        <div className="mt-8 space-y-3">
          {!spacesForbidden &&
            spaces.map((p) => {
            const selected = new Set(accessByProjectId.get(p.id) ?? []);
            return (
              <div key={p.id} className="rounded-sm border border-border bg-card p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="text-sm font-semibold text-foreground">#{p.name}</div>
                    {p.description ? (
                      <div className="mt-1 text-sm text-muted-foreground">{p.description}</div>
                    ) : null}
                    <div className="mt-2 text-xs text-muted-foreground">
                      Allowed roles:{" "}
                      {selected.size === 0
                        ? "Anyone with workspace access (default)"
                        : Array.from(selected)
                            .map((rid) => roles.find((r) => r.id === rid)?.name ?? "…")
                            .join(", ")}
                    </div>
                  </div>
                  <Link
                    to={`/o/${orgId}/p/${p.id}`}
                    className="rounded-sm border border-border bg-transparent px-3 py-1.5 text-sm text-foreground hover:bg-accent"
                  >
                    Open space
                  </Link>
                </div>

                <div className="mt-4 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
                  {roles.map((r) => (
                    <label key={r.id} className="flex items-center gap-2 text-sm text-foreground">
                      <input
                        type="checkbox"
                        checked={selected.has(r.id)}
                        onChange={(e) => {
                          const next = new Set(selected);
                          if (e.target.checked) next.add(r.id);
                          else next.delete(r.id);
                          putAccess.mutate({ projectId: p.id, roleIds: Array.from(next) });
                        }}
                        disabled={putAccess.isPending}
                      />
                      <span className="truncate">{r.name}</span>
                    </label>
                  ))}
                  {roles.length === 0 && !rolesQ.isLoading ? (
                    <div className="text-sm text-muted-foreground">No roles yet.</div>
                  ) : null}
                </div>
              </div>
            );
          })}

          {!spacesForbidden && spaces.length === 0 && !spacesQ.isLoading ? (
            <div className="rounded-sm border border-dashed border-border bg-card px-3 py-10 text-center text-sm text-muted-foreground">
              No spaces yet.
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}

