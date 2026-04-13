import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { apiFetch } from "../api/http";
import { coerceOrgPermissionList } from "../api/permissions";
import type { OrgMemberWithUser, RoleWithPermissions, UUID } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { useViewAsRoles } from "../auth/ViewAsRolesContext";

function isOwnerRoleName(name: string): boolean {
  return name.trim().toLowerCase() === "owner";
}

const ALL_PERMS = [
  "org.manage",
  "org.members.manage",
  "space.create",
  "space.delete",
  "space.members.manage",
  "board.read",
  "board.write",
  "tasks.read",
  "tasks.write",
  "chat.read",
  "chat.write",
  "files.read",
  "files.write",
  "files.delete",
  "agent.tools.invoke",
  "datasets.read",
  "datasets.write",
  "terminal.use",
  "ssh_connections.manage",
] as const;

type Perm = (typeof ALL_PERMS)[number];

const PERM_TOOLTIPS: Record<Perm, string> = {
  "org.manage": "Workspace-wide administration (highest-level org control).",
  "org.members.manage": "Invite/remove members and manage membership settings for the workspace.",
  "space.create": "Create new spaces inside the workspace.",
  "space.delete": "Delete spaces inside the workspace.",
  "space.members.manage": "Manage access/membership for spaces. Also grants admin override for space access.",
  "board.read": "View task boards and their contents.",
  "board.write": "Create/edit/delete task boards and columns (board structure).",
  "tasks.read": "View tasks within boards.",
  "tasks.write": "Create/edit/delete tasks and change task fields.",
  "chat.read": "View chat rooms and messages.",
  "chat.write": "Send/edit/delete messages and manage chat interactions you’re allowed to.",
  "files.read": "View file tree and download/read file contents.",
  "files.write": "Create/upload/edit files and folders.",
  "files.delete": "Delete files and folders.",
  "agent.tools.invoke": "Invoke AI agent tools via IDE panel and MCP integrations.",
  "datasets.read": "List and query tabular datasets (preview, filtered reads).",
  "datasets.write": "Upload, complete, or delete datasets in a space.",
  "terminal.use": "Use the web terminal for a space.",
  "ssh_connections.manage": "Create/edit/delete SSH connections for the workspace.",
};

export default function OrgRolesPage() {
  const { orgId } = useParams<{ orgId: string }>();
  const qc = useQueryClient();
  const { state } = useAuth();
  const meId = state.status === "authenticated" ? state.user.id : null;
  const viewAs = useViewAsRoles(orgId);

  const [createName, setCreateName] = useState("");
  const [createPerms, setCreatePerms] = useState<Set<Perm>>(new Set());

  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteToken, setInviteToken] = useState<string | null>(null);

  const [memberRoleDrafts, setMemberRoleDrafts] = useState<Record<string, UUID[]>>(
    {},
  );

  const [editOpen, setEditOpen] = useState(false);
  const [editRoleId, setEditRoleId] = useState<UUID | null>(null);
  const [editName, setEditName] = useState("");
  const [editPerms, setEditPerms] = useState<Set<Perm>>(new Set());

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

  const membersQ = useQuery({
    queryKey: ["orgMembers", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/members`);
      if (!res.ok) throw new Error("members");
      const j = (await res.json()) as { members: OrgMemberWithUser[] };
      return j.members;
    },
  });

  const memberRoleIdsQs = useQueries({
    queries: (membersQ.data ?? []).map((m) => ({
      queryKey: ["memberRoleIds", orgId, m.user_id] as const,
      enabled: !!orgId && !!m.user_id,
      queryFn: async () => {
        const res = await apiFetch(
          `/api/v1/organizations/${orgId}/members/${m.user_id}/roles`,
        );
        if (!res.ok) throw new Error("member roles");
        const j = (await res.json()) as { role_ids: UUID[] };
        return j.role_ids;
      },
    })),
  });

  const memberRoleIdsByUserId = useMemo(() => {
    const m = new Map<string, UUID[]>();
    const members = membersQ.data ?? [];
    for (let i = 0; i < members.length; i++) {
      const uid = members[i]?.user_id;
      const ids = memberRoleIdsQs[i]?.data;
      if (uid && ids) m.set(uid, ids);
    }
    return m;
  }, [membersQ.data, memberRoleIdsQs]);

  const createRole = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/roles`, {
        method: "POST",
        json: { name: createName.trim(), permissions: Array.from(createPerms) },
      });
      if (!res.ok) throw new Error("create role");
      return res.json() as Promise<{ role: RoleWithPermissions }>;
    },
    onSuccess: async () => {
      setCreateName("");
      setCreatePerms(new Set());
      await qc.invalidateQueries({ queryKey: ["roles", orgId] });
    },
  });

  const patchRole = useMutation({
    mutationFn: async () => {
      if (!editRoleId) throw new Error("missing role");
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/roles/${editRoleId}`,
        {
          method: "PATCH",
          json: {
            name: editName.trim(),
            set_permissions: true,
            permissions: Array.from(editPerms),
          },
        },
      );
      if (!res.ok) throw new Error("patch role");
      return res.json() as Promise<{ role: RoleWithPermissions }>;
    },
    onSuccess: async () => {
      setEditOpen(false);
      setEditRoleId(null);
      await qc.invalidateQueries({ queryKey: ["roles", orgId] });
    },
  });

  const deleteRole = useMutation({
    mutationFn: async (roleId: UUID) => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/roles/${roleId}`, {
        method: "DELETE",
      });
      // 404: role already removed (e.g. migration) or stale UI — refresh list.
      if (res.status === 404) {
        await qc.invalidateQueries({ queryKey: ["roles", orgId] });
        return;
      }
      if (!res.ok) throw new Error("delete role");
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["roles", orgId] });
    },
  });

  const setMemberRoles = useMutation({
    mutationFn: async (vars: { userId: UUID; roleIds: UUID[] }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/members/${vars.userId}/roles`,
        { method: "PUT", json: { role_ids: vars.roleIds } },
      );
      if (!res.ok) throw new Error("set member roles");
      return res.json() as Promise<{ ok: boolean }>;
    },
    onSuccess: async (_data, vars) => {
      await qc.invalidateQueries({ queryKey: ["memberRoleIds", orgId, vars.userId] });
      await qc.invalidateQueries({ queryKey: ["myOrgPermissions", orgId] });
    },
  });

  const createInvite = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/invites`, {
        method: "POST",
        json: inviteEmail.trim() ? { email: inviteEmail.trim() } : {},
      });
      if (!res.ok) throw new Error("create invite");
      return res.json() as Promise<{ token: string }>;
    },
    onSuccess: (data) => {
      setInviteToken(data.token);
      setInviteEmail("");
    },
  });

  const removeMember = useMutation({
    mutationFn: async (userId: UUID) => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/members/${userId}`, {
        method: "DELETE",
      });
      if (!res.ok) throw new Error("remove member");
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["orgMembers", orgId] });
      await qc.invalidateQueries({ queryKey: ["memberRoleIds", orgId] });
      await qc.invalidateQueries({ queryKey: ["myOrgPermissions", orgId] });
    },
  });

  const roles = rolesQ.data ?? [];
  const roleIdsForMember = (m: OrgMemberWithUser): UUID[] => {
    // Prefer locally-edited draft (immediate UI response), otherwise use server truth.
    return memberRoleDrafts[m.user_id] ?? memberRoleIdsByUserId.get(m.user_id) ?? [];
  };

  const ownerRoleId = useMemo(() => {
    return roles.find((r) => r.is_system && isOwnerRoleName(r.name))?.id ?? null;
  }, [roles]);

  const myRoleIds = useMemo(() => {
    if (!meId) return [] as UUID[];
    return memberRoleIdsByUserId.get(meId) ?? [];
  }, [meId, memberRoleIdsByUserId]);

  const canUseViewAs = useMemo(() => {
    if (!meId) return false;
    const perms = coerceOrgPermissionList(myPermsQ.data);
    if (perms.includes("org.manage")) return true;
    const legacy = (membersQ.data ?? []).find((m) => m.user_id === meId)?.role;
    if (legacy === "admin") return true;
    if (ownerRoleId && myRoleIds.includes(ownerRoleId)) return true;
    return false;
  }, [meId, myPermsQ.data, membersQ.data, ownerRoleId, myRoleIds]);

  if (!orgId) return null;

  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-background">
      <div className="mx-auto max-w-5xl px-4 py-8">
        <header className="border-b border-border pb-6">
          <Link to={`/o/${orgId}`} className="text-xs text-link hover:underline">
            ← Back to spaces
          </Link>
          <p className="mt-2 text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
            Settings
          </p>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight text-foreground">
            Roles & permissions
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Create custom roles and assign them to workspace members.
          </p>
        </header>

        <section className="mt-8 rounded-sm border border-border bg-card p-4">
          <h2 className="text-xs font-semibold uppercase tracking-[0.15em] text-muted-foreground">
            Roles
          </h2>

          <form
            className="mt-4 space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (!createName.trim()) return;
              createRole.mutate();
            }}
          >
            <div className="flex flex-wrap gap-2">
              <input
                className="min-w-[220px] flex-1 rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                placeholder="New role name"
                value={createName}
                onChange={(e) => setCreateName(e.target.value)}
              />
              <button
                type="submit"
                disabled={createRole.isPending}
                className="rounded-sm bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
              >
                Create role
              </button>
            </div>

            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
              {ALL_PERMS.map((p) => (
                <label
                  key={p}
                  className="flex items-center gap-2 text-sm text-foreground"
                  title={PERM_TOOLTIPS[p]}
                >
                  <input
                    type="checkbox"
                    checked={createPerms.has(p)}
                    onChange={(e) => {
                      setCreatePerms((s) => {
                        const n = new Set(s);
                        if (e.target.checked) n.add(p);
                        else n.delete(p);
                        return n;
                      });
                    }}
                  />
                  <span className="font-mono text-xs" title={PERM_TOOLTIPS[p]}>
                    {p}
                  </span>
                </label>
              ))}
            </div>
          </form>

          <div className="mt-6 space-y-2">
            {roles.map((r) => (
              <div
                key={r.id}
                className="rounded-sm border border-border bg-background px-4 py-3"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <div className="font-medium text-foreground">{r.name}</div>
                      {r.is_system && (
                        <span className="rounded-full border border-border bg-secondary px-2 py-0.5 text-[10px] font-medium text-secondary-foreground">
                          system
                        </span>
                      )}
                    </div>
                    <div className="mt-2 flex flex-wrap gap-1">
                      {(r.permissions ?? []).slice(0, 8).map((p) => (
                        <span
                          key={p}
                          className="rounded-full border border-border bg-secondary px-2 py-0.5 text-[10px] text-secondary-foreground"
                          title={PERM_TOOLTIPS[p as Perm] ?? p}
                        >
                          {p}
                        </span>
                      ))}
                      {(r.permissions?.length ?? 0) > 8 && (
                        <span className="text-[10px] text-muted-foreground">
                          +{(r.permissions?.length ?? 0) - 8} more
                        </span>
                      )}
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      disabled={isOwnerRoleName(r.name)}
                      className="rounded-sm border border-border bg-transparent px-3 py-1.5 text-sm text-foreground hover:bg-accent disabled:opacity-50"
                      onClick={() => {
                        setEditOpen(true);
                        setEditRoleId(r.id);
                        setEditName(r.name);
                        setEditPerms(new Set(r.permissions as Perm[]));
                      }}
                      title={isOwnerRoleName(r.name) ? "Owner role cannot be edited" : undefined}
                    >
                      Edit
                    </button>
                    <button
                      type="button"
                      disabled={isOwnerRoleName(r.name) || deleteRole.isPending}
                      className="rounded-sm border border-border bg-transparent px-3 py-1.5 text-sm text-foreground hover:bg-accent disabled:opacity-50"
                      onClick={() => deleteRole.mutate(r.id)}
                      title={isOwnerRoleName(r.name) ? "Owner cannot be deleted" : "Delete role"}
                    >
                      Delete
                    </button>
                  </div>
                </div>
              </div>
            ))}
            {roles.length === 0 && !rolesQ.isLoading && (
              <p className="text-sm text-muted-foreground">No roles yet.</p>
            )}
          </div>
        </section>

        <section className="mt-6 rounded-sm border border-border bg-card p-4">
          <h2 className="text-xs font-semibold uppercase tracking-[0.15em] text-muted-foreground">
            Members
          </h2>

          {canUseViewAs ? (
            <div className="mt-4 rounded-sm border border-border bg-background p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="min-w-0">
                  <div className="text-sm font-medium text-foreground">View workspace as role(s)</div>
                  <div className="text-xs text-muted-foreground">
                    Simulates access and locks without changing backend permissions.
                  </div>
                </div>
                <label className="flex items-center gap-2 text-xs text-foreground">
                  <input
                    type="checkbox"
                    checked={viewAs.state.enabled}
                    onChange={(e) => {
                      const enabled = e.target.checked;
                      viewAs.set({
                        enabled,
                        roleIds: enabled ? viewAs.state.roleIds : [],
                      });
                    }}
                  />
                  Viewing as roles
                </label>
              </div>

              {viewAs.state.enabled ? (
                <>
                  <div className="mt-3 flex flex-wrap gap-2">
                    <button
                      type="button"
                      className="rounded-sm border border-border bg-transparent px-3 py-1.5 text-xs text-foreground hover:bg-accent"
                      onClick={() => viewAs.set({ enabled: true, roleIds: [] })}
                    >
                      Select none
                    </button>
                    <button
                      type="button"
                      className="rounded-sm border border-border bg-transparent px-3 py-1.5 text-xs text-foreground hover:bg-accent"
                      onClick={() =>
                        viewAs.set({ enabled: true, roleIds: roles.map((r) => r.id) as UUID[] })
                      }
                    >
                      Select all
                    </button>
                    <button
                      type="button"
                      className="rounded-sm border border-border bg-transparent px-3 py-1.5 text-xs text-foreground hover:bg-accent"
                      onClick={() => viewAs.set({ enabled: true, roleIds: myRoleIds })}
                      disabled={!meId}
                      title={!meId ? "Sign in required" : "Use my assigned roles"}
                    >
                      Reset to my roles
                    </button>
                    <button
                      type="button"
                      className="ml-auto rounded-sm border border-border bg-transparent px-3 py-1.5 text-xs text-foreground hover:bg-accent"
                      onClick={() => viewAs.clear()}
                    >
                      Clear
                    </button>
                  </div>

                  <div className="mt-3 flex flex-wrap gap-3">
                    {roles.map((r) => (
                      <label key={r.id} className="flex items-center gap-2 text-xs text-foreground">
                        <input
                          type="checkbox"
                          checked={viewAs.state.roleIds.includes(r.id)}
                          onChange={(e) => {
                            const cur = viewAs.state.roleIds;
                            const next = e.target.checked
                              ? Array.from(new Set([...cur, r.id]))
                              : cur.filter((x) => x !== r.id);
                            viewAs.set({ enabled: true, roleIds: next });
                          }}
                        />
                        {r.name}
                      </label>
                    ))}
                    {roles.length === 0 ? (
                      <div className="text-xs text-muted-foreground">No roles found.</div>
                    ) : null}
                  </div>
                </>
              ) : null}
            </div>
          ) : null}

          <div className="mt-4 space-y-2">
            {(membersQ.data ?? []).map((m) => (
              <div
                key={m.user_id}
                className="rounded-sm border border-border bg-background px-4 py-3"
              >
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="font-medium text-foreground">
                      {m.display_name ?? m.email}
                    </div>
                    <div className="text-xs text-muted-foreground">{m.email}</div>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {roles.map((r) => (
                      <label key={r.id} className="flex items-center gap-2 text-xs">
                        <input
                          type="checkbox"
                          checked={roleIdsForMember(m).includes(r.id)}
                          onChange={(e) => {
                            const cur = roleIdsForMember(m);
                            const next = e.target.checked
                              ? Array.from(new Set([...cur, r.id]))
                              : cur.filter((x) => x !== r.id);
                            setMemberRoleDrafts((d) => ({ ...d, [m.user_id]: next }));
                            setMemberRoles.mutate({ userId: m.user_id, roleIds: next });
                          }}
                        />
                        {r.name}
                      </label>
                    ))}
                    <button
                      type="button"
                      className="rounded-sm border border-border bg-transparent px-3 py-1.5 text-xs text-foreground hover:bg-accent disabled:opacity-50"
                      disabled={m.user_id === meId || removeMember.isPending}
                      title={m.user_id === meId ? "You cannot remove yourself" : "Remove member"}
                      onClick={() => {
                        if (window.confirm(`Remove ${m.display_name ?? m.email} from this organization?`)) {
                          removeMember.mutate(m.user_id);
                        }
                      }}
                    >
                      Remove
                    </button>
                  </div>
                </div>
              </div>
            ))}
            {(membersQ.data?.length ?? 0) === 0 && !membersQ.isLoading && (
              <p className="text-sm text-muted-foreground">No members.</p>
            )}
          </div>
        </section>

        <section className="mt-6 rounded-sm border border-border bg-card p-4">
          <h2 className="text-xs font-semibold uppercase tracking-[0.15em] text-muted-foreground">
            Invites
          </h2>
          <form
            className="mt-4 flex flex-wrap gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              createInvite.mutate();
            }}
          >
            <input
              className="min-w-[260px] flex-1 rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
              placeholder="Email (optional)"
              value={inviteEmail}
              onChange={(e) => setInviteEmail(e.target.value)}
            />
            <button
              type="submit"
              disabled={createInvite.isPending}
              className="rounded-sm bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
            >
              Create invite
            </button>
          </form>
          {inviteToken && (
            <div className="mt-4 rounded-sm border border-border bg-background p-3">
              <p className="text-xs text-muted-foreground">Invite token (share once):</p>
              <code className="mt-2 block break-all rounded-sm bg-muted px-3 py-2 text-xs text-foreground">
                {inviteToken}
              </code>
              <p className="mt-2 text-xs text-muted-foreground">
                Recipient can accept at{" "}
                <Link to="/invite/accept" className="text-link hover:underline">
                  /invite/accept
                </Link>
                .
              </p>
            </div>
          )}
        </section>

        {editOpen && editRoleId && (
          <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/60 sm:items-center">
            <div className="max-h-[90vh] w-full max-w-3xl overflow-y-auto rounded-t-sm border border-border bg-card p-6 sm:rounded-sm">
              <h3 className="text-lg font-semibold text-card-foreground">Edit role</h3>
              <form
                className="mt-4 space-y-4"
                onSubmit={(e) => {
                  e.preventDefault();
                  patchRole.mutate();
                }}
              >
                <div>
                  <label className="text-xs text-muted-foreground">Name</label>
                  <input
                    className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                    value={editName}
                    onChange={(e) => setEditName(e.target.value)}
                  />
                </div>
                <div>
                  <label className="text-xs text-muted-foreground">Permissions</label>
                  <div className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
                    {ALL_PERMS.map((p) => (
                    <label
                      key={p}
                      className="flex items-center gap-2 text-sm text-foreground"
                      title={PERM_TOOLTIPS[p]}
                    >
                        <input
                          type="checkbox"
                          checked={editPerms.has(p)}
                          onChange={(e) => {
                            setEditPerms((s) => {
                              const n = new Set(s);
                              if (e.target.checked) n.add(p);
                              else n.delete(p);
                              return n;
                            });
                          }}
                        />
                        <span className="font-mono text-xs" title={PERM_TOOLTIPS[p]}>
                          {p}
                        </span>
                      </label>
                    ))}
                  </div>
                </div>
                <div className="flex justify-end gap-2">
                  <button
                    type="button"
                    className="rounded-sm border border-border bg-transparent px-4 py-2 text-sm text-foreground hover:bg-accent"
                    onClick={() => {
                      setEditOpen(false);
                      setEditRoleId(null);
                    }}
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    disabled={patchRole.isPending}
                    className="rounded-sm bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
                  >
                    Save
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

