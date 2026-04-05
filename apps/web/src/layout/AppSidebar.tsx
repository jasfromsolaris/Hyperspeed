import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BookOpen,
  ChevronRight,
  Code2,
  FileText,
  Folder,
  Hash,
  Inbox,
  LayoutDashboard,
  LayoutGrid,
  MessageCircle,
  Plus,
  Search,
  Settings,
  Table2,
  Terminal,
  Lock,
  Zap,
} from "lucide-react";
import { type FormEvent, type MouseEvent, useEffect, useMemo, useState } from "react";
import { NavLink, useLocation, useNavigate, useParams } from "react-router-dom";
import { apiFetch } from "../api/http";
import { fetchOrganizationsList } from "../api/orgs";
import type { Board, ChatRoom, FileNode, OrgFeatures, Project } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { useViewAsRolesController } from "../auth/ViewAsRolesContext";
import { orgDotStyle } from "../lib/orgColor";

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  [
    "flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm transition-colors",
    isActive
      ? "bg-accent/60 text-accent-foreground border-l-2 border-primary pl-[6px]"
      : "text-muted-foreground hover:bg-accent/40 hover:text-foreground border-l-2 border-transparent pl-[6px]",
  ].join(" ");

/** Normalize list response or cached query data; never call .flatMap on raw values. */
function asSpaceList(data: unknown): Project[] {
  if (Array.isArray(data)) {
    return data;
  }
  if (data && typeof data === "object") {
    const o = data as Record<string, unknown>;
    if (Array.isArray(o.spaces)) {
      return o.spaces as Project[];
    }
    if (Array.isArray(o.projects)) {
      return o.projects as Project[];
    }
  }
  return [];
}

type CtxMenu = { x: number; y: number; orgId: string };
type SpaceAddMenu = { x: number; y: number; orgId: string; spaceId: string };
type PageMenu =
  | {
      x: number;
      y: number;
      orgId: string;
      spaceId: string;
      kind: "board";
      pageId: string;
      label: string;
    }
  | {
      x: number;
      y: number;
      orgId: string;
      spaceId: string;
      kind: "chat";
      pageId: string;
      label: string;
    }
  | {
      x: number;
      y: number;
      orgId: string;
      spaceId: string;
      kind: "files";
      label: string;
    }
  | {
      x: number;
      y: number;
      orgId: string;
      spaceId: string;
      kind: "ide";
      label: string;
    }
  | {
      x: number;
      y: number;
      orgId: string;
      spaceId: string;
      kind: "terminal";
      label: string;
    }
  | {
      x: number;
      y: number;
      orgId: string;
      spaceId: string;
      kind: "automations";
      label: string;
    }
  | {
      x: number;
      y: number;
      orgId: string;
      spaceId: string;
      kind: "datasets";
      label: string;
    };

const dndNodeMime = "application/x-hyperspeed-node";

function FilesTree(props: {
  orgId: string;
  spaceId: string;
  nodesByParent: Map<string, FileNode[]> | undefined;
  isFolderOpen: (folderId: string) => boolean;
  onToggleFolder: (folderId: string) => void;
  onMoveNode: (vars: { nodeId: string; parentId: string | null }) => void;
  parentKey?: string;
  depth?: number;
}) {
  const {
    orgId,
    spaceId,
    nodesByParent,
    isFolderOpen,
    onToggleFolder,
    onMoveNode,
    parentKey = "root",
    depth = 0,
  } = props;

  const nodes = nodesByParent?.get(parentKey) ?? [];
  const pad = Math.min(depth, 6) * 12;

  function setDraggedNode(e: React.DragEvent, n: FileNode) {
    e.dataTransfer.effectAllowed = "move";
    const payload = JSON.stringify({ id: n.id, kind: n.kind });
    // Some browsers don't reliably round-trip custom MIME types; keep a text fallback.
    e.dataTransfer.setData(dndNodeMime, payload);
    e.dataTransfer.setData("text/plain", payload);
  }

  function getDraggedNode(e: React.DragEvent): { id: string; kind: FileNode["kind"] } | null {
    const raw = e.dataTransfer.getData(dndNodeMime) || e.dataTransfer.getData("text/plain");
    if (!raw) return null;
    try {
      const j = JSON.parse(raw) as { id?: string; kind?: FileNode["kind"] };
      if (!j?.id || (j.kind !== "file" && j.kind !== "folder")) return null;
      return { id: j.id, kind: j.kind };
    } catch {
      return null;
    }
  }

  return (
    <ul className="mt-0.5 space-y-0.5" style={{ paddingLeft: pad }}>
      {nodes.slice(0, 50).map((n) => {
        const to =
          n.kind === "folder"
            ? `/o/${orgId}/p/${spaceId}/files?parentId=${n.id}`
            : (() => {
                const qs = new URLSearchParams();
                qs.set("fileId", n.id);
                if (n.parent_id) qs.set("parentId", n.parent_id);
                return `/o/${orgId}/p/${spaceId}/files?${qs.toString()}`;
              })();
        const open = n.kind === "folder" ? isFolderOpen(n.id) : false;
        return (
          <li key={`${parentKey}-${n.id}`}>
            <div
              className="flex items-center gap-1"
              draggable
              onDragStart={(e) => setDraggedNode(e, n)}
              onDragOver={(e) => {
                if (n.kind !== "folder") return;
                // Don't call getData() here—some browsers keep it empty until drop.
                if (!e.dataTransfer.types.includes(dndNodeMime)) return;
                e.preventDefault();
                e.dataTransfer.dropEffect = "move";
              }}
              onDrop={(e) => {
                if (n.kind !== "folder") return;
                const dragged = getDraggedNode(e);
                if (!dragged) return;
                if (dragged.id === n.id) return;
                e.preventDefault();
                e.stopPropagation();
                onMoveNode({ nodeId: dragged.id, parentId: n.id });
              }}
            >
              {n.kind === "folder" ? (
                <button
                  type="button"
                  className="rounded-sm p-1 text-muted-foreground hover:bg-accent/30 hover:text-foreground"
                  title={open ? "Collapse" : "Expand"}
                  aria-label={open ? "Collapse folder" : "Expand folder"}
                  onClick={() => onToggleFolder(n.id)}
                  onContextMenu={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    onToggleFolder(n.id);
                  }}
                >
                  <ChevronRight
                    className={[
                      "h-3.5 w-3.5 opacity-70 transition-transform",
                      open ? "rotate-90" : "",
                    ].join(" ")}
                  />
                </button>
              ) : (
                <span className="w-[26px]" />
              )}

              <NavLink
                to={to}
                className={({ isActive }) =>
                  [
                    "flex min-w-0 flex-1 items-center gap-1.5 rounded-sm px-2 py-1 text-sm",
                    isActive
                      ? "bg-accent/50 text-foreground"
                      : "text-muted-foreground hover:bg-accent/30 hover:text-foreground",
                  ].join(" ")
                }
                onContextMenu={(e) => {
                  if (n.kind !== "folder") {
                    return;
                  }
                  e.preventDefault();
                  e.stopPropagation();
                  onToggleFolder(n.id);
                }}
              >
                {n.kind === "folder" ? (
                  <Folder className="h-3.5 w-3.5 shrink-0 opacity-70" />
                ) : (
                  <FileText className="h-3.5 w-3.5 shrink-0 opacity-70" />
                )}
                <span className="min-w-0 truncate">{n.name}</span>
              </NavLink>
            </div>

            {n.kind === "folder" && open ? (
              <FilesTree
                orgId={orgId}
                spaceId={spaceId}
                nodesByParent={nodesByParent}
                isFolderOpen={isFolderOpen}
                onToggleFolder={onToggleFolder}
                onMoveNode={onMoveNode}
                parentKey={n.id}
                depth={depth + 1}
              />
            ) : null}
          </li>
        );
      })}
    </ul>
  );
}

export function AppSidebar() {
  const qc = useQueryClient();
  const { state, logout } = useAuth();
  const viewAsCtrl = useViewAsRolesController();
  const navigate = useNavigate();
  const location = useLocation();
  const { orgId } = useParams<{
    orgId?: string;
  }>();

  const email = state.status === "authenticated" ? state.user.email : "";
  const initial = email ? email[0]!.toUpperCase() : "?";

  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  /** When true, the space's page branch (boards, chats, files) is hidden. */
  const [spaceCollapsed, setSpaceCollapsed] = useState<Record<string, boolean>>({});
  /** When true, the Files subtree within a space is hidden. */
  const [filesBranchCollapsed, setFilesBranchCollapsed] = useState<Record<string, boolean>>({});
  /** Per-space folder expansion state inside Files tree. */
  const [filesFolderExpanded, setFilesFolderExpanded] = useState<
    Record<string, Record<string, boolean>>
  >({});
  const [ctxMenu, setCtxMenu] = useState<CtxMenu | null>(null);
  const [spaceAddMenu, setSpaceAddMenu] = useState<SpaceAddMenu | null>(null);
  const [pageMenu, setPageMenu] = useState<PageMenu | null>(null);
  const [createModalOrgId, setCreateModalOrgId] = useState<string | null>(null);
  const [createName, setCreateName] = useState("");

  const moveNode = useMutation({
    mutationFn: async (vars: { orgId: string; spaceId: string; nodeId: string; parentId: string | null }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${vars.orgId}/spaces/${vars.spaceId}/files/${vars.nodeId}`,
        { method: "PATCH", json: { parent_id: vars.parentId } },
      );
      if (!res.ok) {
        throw new Error("move");
      }
      return res.json() as Promise<{ ok: boolean }>;
    },
    onSuccess: (_ok, vars) => {
      void qc.invalidateQueries({ queryKey: ["file-tree", vars.spaceId] });
      void qc.invalidateQueries({ queryKey: ["file-nodes", vars.spaceId], exact: false });
    },
  });

  function getDraggedNode(e: React.DragEvent): { id: string; kind: FileNode["kind"] } | null {
    const raw = e.dataTransfer.getData(dndNodeMime) || e.dataTransfer.getData("text/plain");
    if (!raw) return null;
    try {
      const j = JSON.parse(raw) as { id?: string; kind?: FileNode["kind"] };
      if (!j?.id || (j.kind !== "file" && j.kind !== "folder")) return null;
      return { id: j.id, kind: j.kind };
    } catch {
      return null;
    }
  }

  useEffect(() => {
    if (orgId) {
      setExpanded((prev) => ({ ...prev, [orgId]: true }));
    }
  }, [orgId]);

  /** Expand the space branch when the route lands on any page under that space. */
  useEffect(() => {
    const m = location.pathname.match(/^\/o\/[^/]+\/p\/([^/]+)/);
    if (!m?.[1]) {
      return;
    }
    const sid = m[1];
    setSpaceCollapsed((prev) => ({ ...prev, [sid]: false }));
  }, [location.pathname]);

  function toggleSpaceBranch(spaceId: string) {
    setSpaceCollapsed((prev) => ({
      ...prev,
      [spaceId]: !(prev[spaceId] === true),
    }));
  }

  function isSpaceBranchCollapsed(spaceId: string) {
    return spaceCollapsed[spaceId] === true;
  }

  function toggleFilesBranch(spaceId: string) {
    setFilesBranchCollapsed((prev) => ({
      ...prev,
      [spaceId]: !(prev[spaceId] === true),
    }));
  }

  function isFilesBranchCollapsed(spaceId: string) {
    return filesBranchCollapsed[spaceId] === true;
  }

  function toggleFilesFolder(spaceId: string, folderId: string) {
    setFilesFolderExpanded((prev) => {
      const cur = prev[spaceId] ?? {};
      return { ...prev, [spaceId]: { ...cur, [folderId]: !(cur[folderId] === true) } };
    });
  }

  function isFilesFolderExpanded(spaceId: string, folderId: string) {
    return filesFolderExpanded[spaceId]?.[folderId] === true;
  }

  const filesEnabledKey = (spaceId: string) => `hs_space_files_enabled_${spaceId}`;
  function isFilesEnabled(spaceId: string) {
    return window.localStorage.getItem(filesEnabledKey(spaceId)) === "1";
  }
  function setFilesEnabled(spaceId: string) {
    window.localStorage.setItem(filesEnabledKey(spaceId), "1");
  }
  function clearFilesEnabled(spaceId: string) {
    window.localStorage.removeItem(filesEnabledKey(spaceId));
  }

  const ideEnabledKey = (spaceId: string) => `hs_space_ide_enabled_${spaceId}`;
  function isIdeEnabled(spaceId: string) {
    return window.localStorage.getItem(ideEnabledKey(spaceId)) === "1";
  }
  function setIdeEnabled(spaceId: string) {
    window.localStorage.setItem(ideEnabledKey(spaceId), "1");
  }
  function clearIdeEnabled(spaceId: string) {
    window.localStorage.removeItem(ideEnabledKey(spaceId));
  }

  const terminalEnabledKey = (spaceId: string) => `hs_space_terminal_enabled_${spaceId}`;
  function isTerminalEnabled(spaceId: string) {
    return window.localStorage.getItem(terminalEnabledKey(spaceId)) === "1";
  }
  function setTerminalEnabled(spaceId: string) {
    window.localStorage.setItem(terminalEnabledKey(spaceId), "1");
  }
  function clearTerminalEnabled(spaceId: string) {
    window.localStorage.removeItem(terminalEnabledKey(spaceId));
  }

  const automationsEnabledKey = (spaceId: string) =>
    `hs_space_automations_enabled_${spaceId}`;
  function isAutomationsEnabled(spaceId: string) {
    return window.localStorage.getItem(automationsEnabledKey(spaceId)) === "1";
  }
  function setAutomationsEnabled(spaceId: string) {
    window.localStorage.setItem(automationsEnabledKey(spaceId), "1");
  }
  function clearAutomationsEnabled(spaceId: string) {
    window.localStorage.removeItem(automationsEnabledKey(spaceId));
  }

  const datasetsPageEnabledKey = (spaceId: string) =>
    `hs_space_datasets_page_enabled_${spaceId}`;
  function isDatasetsPageEnabled(spaceId: string) {
    return window.localStorage.getItem(datasetsPageEnabledKey(spaceId)) === "1";
  }
  function setDatasetsPageEnabled(spaceId: string) {
    window.localStorage.setItem(datasetsPageEnabledKey(spaceId), "1");
  }
  function clearDatasetsPageEnabled(spaceId: string) {
    window.localStorage.removeItem(datasetsPageEnabledKey(spaceId));
  }

  useEffect(() => {
    if (!ctxMenu) {
      return;
    }
    let removeListeners: (() => void) | undefined;
    const id = window.setTimeout(() => {
      function close() {
        setCtxMenu(null);
      }
      document.addEventListener("click", close);
      document.addEventListener("scroll", close, true);
      removeListeners = () => {
        document.removeEventListener("click", close);
        document.removeEventListener("scroll", close, true);
      };
    }, 100);
    return () => {
      window.clearTimeout(id);
      removeListeners?.();
    };
  }, [ctxMenu]);

  useEffect(() => {
    if (!spaceAddMenu) {
      return;
    }
    let removeListeners: (() => void) | undefined;
    const id = window.setTimeout(() => {
      function close() {
        setSpaceAddMenu(null);
      }
      document.addEventListener("click", close);
      document.addEventListener("scroll", close, true);
      removeListeners = () => {
        document.removeEventListener("click", close);
        document.removeEventListener("scroll", close, true);
      };
    }, 100);
    return () => {
      window.clearTimeout(id);
      removeListeners?.();
    };
  }, [spaceAddMenu]);

  useEffect(() => {
    if (!pageMenu) {
      return;
    }
    let removeListeners: (() => void) | undefined;
    const id = window.setTimeout(() => {
      function close() {
        setPageMenu(null);
      }
      document.addEventListener("click", close);
      document.addEventListener("scroll", close, true);
      removeListeners = () => {
        document.removeEventListener("click", close);
        document.removeEventListener("scroll", close, true);
      };
    }, 100);
    return () => {
      window.clearTimeout(id);
      removeListeners?.();
    };
  }, [pageMenu]);

  useEffect(() => {
    if (!createModalOrgId) {
      return;
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        setCreateModalOrgId(null);
        setCreateName("");
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [createModalOrgId]);

  const orgsQ = useQuery({
    queryKey: ["orgs"],
    queryFn: fetchOrganizationsList,
  });

  const orgs = orgsQ.data?.organizations ?? [];
  const firstOrgId = orgs[0]?.id;
  const meId = state.status === "authenticated" ? state.user.id : null;

  const orgFeaturesQs = useQueries({
    queries: orgs.map((o) => ({
      queryKey: ["org-features", o.id] as const,
      queryFn: async () => {
        const res = await apiFetch(`/api/v1/organizations/${o.id}/features`);
        if (!res.ok) throw new Error("org features");
        const j = (await res.json()) as { features: OrgFeatures };
        return j.features;
      },
    })),
  });
  const datasetsEnabledByOrgId = useMemo(() => {
    const m = new Map<string, boolean>();
    for (let i = 0; i < orgs.length; i++) {
      const org = orgs[i];
      if (!org) continue;
      m.set(org.id, !!orgFeaturesQs[i]?.data?.datasets_enabled);
    }
    return m;
  }, [orgs, orgFeaturesQs]);

  const inboxUnreadQs = useQueries({
    queries: orgs.map((o) => ({
      queryKey: ["notifications-unread", o.id] as const,
      enabled: state.status === "authenticated" && !!o.id,
      queryFn: async () => {
        const res = await apiFetch(
          `/api/v1/me/notifications?org_id=${encodeURIComponent(o.id)}&limit=1`,
        );
        if (!res.ok) throw new Error("notifications");
        const j = (await res.json()) as { unread_count?: number };
        return j.unread_count ?? 0;
      },
      staleTime: 0,
    })),
  });

  const inboxUnreadTotal = useMemo(() => {
    let n = 0;
    for (const q of inboxUnreadQs) {
      const v = q.data;
      if (typeof v === "number") n += v;
    }
    return n;
  }, [inboxUnreadQs]);

  const orgMembersQs = useQueries({
    queries: orgs.map((o) => ({
      queryKey: ["orgMembers", o.id] as const,
      enabled: !!o.id && !!meId,
      queryFn: async () => {
        const res = await apiFetch(`/api/v1/organizations/${o.id}/members`);
        if (!res.ok) throw new Error("members");
        const j = (await res.json()) as {
          members: { user_id: string; role: "admin" | "member" }[];
        };
        return j.members;
      },
    })),
  });

  const isOrgAdmin = useMemo(() => {
    const m = new Map<string, boolean>();
    for (let i = 0; i < orgs.length; i++) {
      const oid = orgs[i]?.id;
      const members = orgMembersQs[i]?.data;
      if (!oid || !meId || !members) continue;
      const mine = members.find((x) => x.user_id === meId);
      m.set(oid, mine?.role === "admin");
    }
    return m;
  }, [orgs, orgMembersQs, meId]);

  const accessSummaryQs = useQueries({
    queries: orgs.map((o) => ({
      queryKey: ["projects-access-summary", o.id] as const,
      enabled: !!o.id && !!meId,
      queryFn: async () => {
        const res = await apiFetch(`/api/v1/organizations/${o.id}/spaces/access-summary`);
        if (res.status === 403) {
          return { denied: true as const, projects: [] as { project_id: string; can_access: boolean }[] };
        }
        if (!res.ok) throw new Error("access summary");
        const j = (await res.json()) as {
          projects: { project_id: string; can_access: boolean }[];
        };
        return { denied: false as const, projects: j.projects };
      },
    })),
  });

  const canAccessBySpaceId = useMemo(() => {
    const m = new Map<string, boolean>();
    for (let i = 0; i < orgs.length; i++) {
      const row = accessSummaryQs[i]?.data;
      if (!row || row.denied) continue;
      for (const p of row.projects) {
        m.set(p.project_id, p.can_access);
      }
    }
    return m;
  }, [orgs, accessSummaryQs]);

  const accessSummaryDeniedByOrgId = useMemo(() => {
    const m = new Map<string, boolean>();
    for (let i = 0; i < orgs.length; i++) {
      const oid = orgs[i]?.id;
      const row = accessSummaryQs[i]?.data;
      if (oid && row?.denied) m.set(oid, true);
    }
    return m;
  }, [orgs, accessSummaryQs]);

  useEffect(() => {
    const list = orgsQ.data?.organizations;
    if (!list?.length) {
      return;
    }
    setExpanded((prev) => {
      if (Object.keys(prev).length > 0) {
        return prev;
      }
      return { [list[0]!.id]: true };
    });
  }, [orgsQ.data]);

  const spaceQueries = useQueries({
    queries: orgs.map((o) => ({
      queryKey: ["projects", o.id] as const,
      queryFn: async () => {
        const res = await apiFetch(`/api/v1/organizations/${o.id}/spaces`);
        if (res.status === 403) {
          return [] as Project[];
        }
        if (!res.ok) {
          throw new Error("projects");
        }
        return asSpaceList(await res.json());
      },
      enabled: orgs.length > 0,
    })),
  });

  const viewAsByOrgId = useMemo(() => {
    const m = new Map<string, { enabled: boolean; roleIds: string[] }>();
    for (const o of orgs) {
      m.set(o.id, viewAsCtrl.getState(o.id));
    }
    return m;
  }, [orgs, viewAsCtrl]);

  type SpaceAccessRequest = { orgId: string; spaceId: string };
  const spaceAccessRequests = useMemo((): SpaceAccessRequest[] => {
    return orgs.flatMap((o, i) => {
      const v = viewAsByOrgId.get(o.id);
      if (!v?.enabled) return [];
      const spaces = asSpaceList(spaceQueries[i]?.data);
      return spaces.map((s) => ({ orgId: o.id, spaceId: s.id }));
    });
  }, [orgs, spaceQueries, viewAsByOrgId]);

  const spaceAccessQs = useQueries({
    queries: spaceAccessRequests.map((req) => ({
      queryKey: ["space-access-roles", req.orgId, req.spaceId] as const,
      queryFn: async () => {
        const res = await apiFetch(
          `/api/v1/organizations/${req.orgId}/spaces/${req.spaceId}/access`,
        );
        if (!res.ok) throw new Error("space access");
        const j = (await res.json()) as { role_ids: string[] };
        return j.role_ids ?? [];
      },
    })),
  });

  const allowedRoleIdsBySpaceId = useMemo(() => {
    const m = new Map<string, string[]>();
    for (let i = 0; i < spaceAccessRequests.length; i++) {
      const req = spaceAccessRequests[i];
      const ids = spaceAccessQs[i]?.data;
      if (req && ids) {
        m.set(req.spaceId, ids);
      }
    }
    return m;
  }, [spaceAccessRequests, spaceAccessQs]);

  const canAccessSpace = (orgId: string, spaceId: string) => {
    const v = viewAsByOrgId.get(orgId);
    if (!v?.enabled) {
      return canAccessBySpaceId.get(spaceId) !== false;
    }
    const allowed = allowedRoleIdsBySpaceId.get(spaceId);
    if (!allowed) {
      return false;
    }
    if (allowed.length === 0) return true;
    for (const rid of v.roleIds) {
      if (allowed.includes(rid)) return true;
    }
    return false;
  };

  type BranchRequest =
    | { orgId: string; spaceId: string; kind: "boards" }
    | { orgId: string; spaceId: string; kind: "chat-rooms" }
    | { orgId: string; spaceId: string; kind: "file-tree" };

  const branchRequests = useMemo((): BranchRequest[] => {
    return orgs.flatMap((o, i) => {
      const spaces = asSpaceList(spaceQueries[i]?.data);
      if (!expanded[o.id]) {
        return [];
      }
      return spaces.flatMap((r) => {
        if (!canAccessSpace(o.id, r.id)) {
          return [];
        }
        if (spaceCollapsed[r.id] === true) {
          return [];
        }
        const out: BranchRequest[] = [
          { orgId: o.id, spaceId: r.id, kind: "boards" },
          { orgId: o.id, spaceId: r.id, kind: "chat-rooms" },
        ];

        if (isFilesEnabled(r.id) && !isFilesBranchCollapsed(r.id)) {
          out.push({ orgId: o.id, spaceId: r.id, kind: "file-tree" });
        }

        return out;
      });
    });
  }, [
    orgs,
    spaceQueries,
    expanded,
    spaceCollapsed,
    filesBranchCollapsed,
    filesFolderExpanded,
    canAccessBySpaceId,
    accessSummaryDeniedByOrgId,
    viewAsByOrgId,
    allowedRoleIdsBySpaceId,
  ]);

  const branchQ = useQueries({
    queries: branchRequests.map((req) => ({
      queryKey:
        req.kind === "file-tree"
          ? (["file-tree", req.spaceId] as const)
          : ([req.kind, req.spaceId] as const),
      queryFn: async () => {
        if (req.kind === "boards") {
          const res = await apiFetch(
            `/api/v1/organizations/${req.orgId}/spaces/${req.spaceId}/boards`,
          );
          if (!res.ok) {
            throw new Error("boards");
          }
          const j = (await res.json()) as { boards: Board[] };
          return j.boards;
        }
        if (req.kind === "chat-rooms") {
          const res = await apiFetch(
            `/api/v1/organizations/${req.orgId}/spaces/${req.spaceId}/chat-rooms`,
          );
          if (!res.ok) {
            throw new Error("chat rooms");
          }
          const j = (await res.json()) as { chat_rooms: ChatRoom[] };
          return j.chat_rooms;
        }
        const res = await apiFetch(
          `/api/v1/organizations/${req.orgId}/spaces/${req.spaceId}/files/tree`,
        );
        if (!res.ok) {
          throw new Error("files");
        }
        const j = (await res.json()) as { nodes: FileNode[] };
        return j.nodes;
      },
    })),
  });

  const branchBySpaceId = useMemo(() => {
    const m = new Map<
      string,
      { boards: Board[]; chats: ChatRoom[]; filesByParent: Map<string, FileNode[]> }
    >();
    for (let i = 0; i < branchRequests.length; i++) {
      const req = branchRequests[i];
      const data = branchQ[i]?.data;
      if (!req || data === undefined) {
        continue;
      }
      const cur =
        m.get(req.spaceId) ?? { boards: [], chats: [], filesByParent: new Map<string, FileNode[]>() };
      if (req.kind === "boards") {
        cur.boards = data as Board[];
      } else if (req.kind === "chat-rooms") {
        cur.chats = data as ChatRoom[];
      } else {
        // Build parent -> children map for tree rendering.
        const nodes = data as FileNode[];
        const byParent = new Map<string, FileNode[]>();
        for (const n of nodes) {
          const k = n.parent_id ?? "root";
          const arr = byParent.get(k) ?? [];
          arr.push(n);
          byParent.set(k, arr);
        }
        for (const [, arr] of byParent) {
          arr.sort((a, b) =>
            a.kind !== b.kind ? a.kind.localeCompare(b.kind) : a.name.localeCompare(b.name),
          );
        }
        cur.filesByParent = byParent;
      }
      m.set(req.spaceId, cur);
    }
    return m;
  }, [branchRequests, branchQ]);

  const createSpace = useMutation({
    mutationFn: async ({ orgId: oid, name }: { orgId: string; name: string }) => {
      const res = await apiFetch(`/api/v1/organizations/${oid}/spaces`, {
        method: "POST",
        json: { name: name.trim(), description: "" },
      });
      if (!res.ok) {
        const e = await res.json().catch(() => ({}));
        throw new Error((e as { error?: string }).error || "Create failed");
      }
      return res.json() as Promise<Project>;
    },
    onSuccess: (space, vars) => {
      void qc.invalidateQueries({ queryKey: ["projects", vars.orgId] });
      void qc.invalidateQueries({ queryKey: ["orgs"] });
      setCreateModalOrgId(null);
      setCreateName("");
      void navigate(`/o/${vars.orgId}/p/${space.id}`);
    },
  });

  const addTaskBoardInSpace = useMutation({
    mutationFn: async (vars: { orgId: string; spaceId: string }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${vars.orgId}/spaces/${vars.spaceId}/boards`,
        { method: "POST", json: { name: "Task board" } },
      );
      if (!res.ok) {
        const e = await res.json().catch(() => ({}));
        throw new Error((e as { error?: string }).error || "Create board failed");
      }
      return res.json() as Promise<Board>;
    },
    onSuccess: (board, vars) => {
      void qc.invalidateQueries({ queryKey: ["boards", vars.spaceId] });
      void qc.invalidateQueries({ queryKey: ["boards", vars.orgId, vars.spaceId] });
      setSpaceAddMenu(null);
      setSpaceCollapsed((prev) => ({ ...prev, [vars.spaceId]: false }));
      void navigate(`/o/${vars.orgId}/p/${vars.spaceId}/b/${board.id}`);
    },
  });

  const addChatInSpace = useMutation({
    mutationFn: async (vars: { orgId: string; spaceId: string }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${vars.orgId}/spaces/${vars.spaceId}/chat-rooms`,
        { method: "POST", json: { name: "Chat" } },
      );
      if (!res.ok) {
        const e = await res.json().catch(() => ({}));
        throw new Error((e as { error?: string }).error || "Create chat failed");
      }
      return res.json() as Promise<ChatRoom>;
    },
    onSuccess: (chat, vars) => {
      void qc.invalidateQueries({ queryKey: ["chat-rooms", vars.spaceId] });
      setSpaceAddMenu(null);
      setSpaceCollapsed((prev) => ({ ...prev, [vars.spaceId]: false }));
      void navigate(`/o/${vars.orgId}/p/${vars.spaceId}/c/${chat.id}`);
    },
  });

  const deleteBoard = useMutation({
    mutationFn: async (vars: { orgId: string; spaceId: string; boardId: string }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${vars.orgId}/spaces/${vars.spaceId}/boards/${vars.boardId}`,
        { method: "DELETE" },
      );
      if (!res.ok) {
        const e = await res.json().catch(() => ({}));
        throw new Error((e as { error?: string }).error || "Delete board failed");
      }
      return true;
    },
    onSuccess: (_ok, vars) => {
      void qc.invalidateQueries({ queryKey: ["boards", vars.spaceId] });
      void qc.invalidateQueries({ queryKey: ["boards", vars.orgId, vars.spaceId] });
      void qc.invalidateQueries({ queryKey: ["tasks", vars.spaceId] });
      setPageMenu(null);
      if (location.pathname.includes(`/p/${vars.spaceId}/b/${vars.boardId}`)) {
        void navigate(`/o/${vars.orgId}/p/${vars.spaceId}`);
      }
    },
  });

  const deleteChat = useMutation({
    mutationFn: async (vars: { orgId: string; spaceId: string; chatRoomId: string }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${vars.orgId}/spaces/${vars.spaceId}/chat-rooms/${vars.chatRoomId}`,
        { method: "DELETE" },
      );
      if (!res.ok) {
        const e = await res.json().catch(() => ({}));
        throw new Error((e as { error?: string }).error || "Delete chat failed");
      }
      return true;
    },
    onSuccess: (_ok, vars) => {
      void qc.invalidateQueries({ queryKey: ["chat-rooms", vars.spaceId] });
      setPageMenu(null);
      if (location.pathname.includes(`/p/${vars.spaceId}/c/${vars.chatRoomId}`)) {
        void navigate(`/o/${vars.orgId}/p/${vars.spaceId}`);
      }
    },
  });

  function openCreateSpaceFromMenu() {
    if (!ctxMenu) {
      return;
    }
    setCreateModalOrgId(ctxMenu.orgId);
    setCtxMenu(null);
    setSpaceAddMenu(null);
    setPageMenu(null);
  }

  function onCreateSpaceSubmit(e: FormEvent) {
    e.preventDefault();
    if (!createModalOrgId || !createName.trim()) {
      return;
    }
    createSpace.mutate({ orgId: createModalOrgId, name: createName.trim() });
  }

  /** Org for create-space: workspace under cursor, else route org, else first workspace. */
  function resolveOrgIdFromEventTarget(target: HTMLElement): string | null {
    const ws = target.closest("[data-workspace-id]");
    if (ws) {
      return ws.getAttribute("data-workspace-id");
    }
    if (orgId) {
      return orgId;
    }
    return firstOrgId ?? null;
  }

  function handleSidebarContextMenu(e: MouseEvent) {
    const t = e.target as HTMLElement;
    if (t.closest('a[href^="http"]')) {
      return;
    }
    const pageEl = t.closest("[data-page-item]");
    if (pageEl) {
      e.preventDefault();
      const kind = pageEl.getAttribute("data-page-kind");
      const oid = pageEl.getAttribute("data-org-id");
      const sid = pageEl.getAttribute("data-space-id");
      const pid = pageEl.getAttribute("data-page-id");
      const label = pageEl.getAttribute("data-page-label") || "Page";
      if (kind && oid && sid) {
        if (kind === "board" && pid) {
          setPageMenu({
            x: e.clientX,
            y: e.clientY,
            orgId: oid,
            spaceId: sid,
            kind: "board",
            pageId: pid,
            label,
          });
        } else if (kind === "chat" && pid) {
          setPageMenu({
            x: e.clientX,
            y: e.clientY,
            orgId: oid,
            spaceId: sid,
            kind: "chat",
            pageId: pid,
            label,
          });
        } else if (kind === "files") {
          setPageMenu({
            x: e.clientX,
            y: e.clientY,
            orgId: oid,
            spaceId: sid,
            kind: "files",
            label,
          });
        } else if (kind === "ide") {
          setPageMenu({
            x: e.clientX,
            y: e.clientY,
            orgId: oid,
            spaceId: sid,
            kind: "ide",
            label,
          });
        } else if (kind === "terminal") {
          setPageMenu({
            x: e.clientX,
            y: e.clientY,
            orgId: oid,
            spaceId: sid,
            kind: "terminal",
            label,
          });
        } else if (kind === "automations") {
          setPageMenu({
            x: e.clientX,
            y: e.clientY,
            orgId: oid,
            spaceId: sid,
            kind: "automations",
            label,
          });
        } else if (kind === "datasets") {
          setPageMenu({
            x: e.clientX,
            y: e.clientY,
            orgId: oid,
            spaceId: sid,
            kind: "datasets",
            label,
          });
        }
        setCtxMenu(null);
        setSpaceAddMenu(null);
      }
      return;
    }
    const spaceEl = t.closest("[data-space-item]");
    if (spaceEl) {
      e.preventDefault();
      const sid = spaceEl.getAttribute("data-space-id");
      const oid = spaceEl.getAttribute("data-org-id");
      if (sid && oid) {
        setSpaceAddMenu({ x: e.clientX, y: e.clientY, orgId: oid, spaceId: sid });
        setCtxMenu(null);
        setPageMenu(null);
      }
      return;
    }
    const header = t.closest("[data-workspace-header]");
    if (header) {
      e.preventDefault();
      const oid = header.getAttribute("data-org-id");
      if (oid) {
        setCtxMenu({ x: e.clientX, y: e.clientY, orgId: oid });
        setSpaceAddMenu(null);
        setPageMenu(null);
      }
      return;
    }
    e.preventDefault();
    const oid = resolveOrgIdFromEventTarget(t);
    if (!oid) {
      return;
    }
    setCtxMenu({ x: e.clientX, y: e.clientY, orgId: oid });
    setSpaceAddMenu(null);
    setPageMenu(null);
  }

  /** Left-click on the workspace header row (not links/buttons) opens the create-space modal. */
  function handleSidebarClick(e: MouseEvent) {
    if (e.button !== 0) {
      return;
    }
    const t = e.target as HTMLElement;
    if (
      t.closest(
        "a[href], button, input, textarea, select, [role='menu'], [role='menuitem']",
      )
    ) {
      return;
    }
    const header = t.closest("[data-workspace-header]");
    if (!header) {
      return;
    }
    const oid = header.getAttribute("data-org-id");
    if (!oid) {
      return;
    }
    setCtxMenu(null);
    setSpaceAddMenu(null);
    setPageMenu(null);
    setCreateModalOrgId(oid);
  }

  function openFilesFromSpaceMenu() {
    if (!spaceAddMenu) {
      return;
    }
    const { orgId: oid, spaceId: sid } = spaceAddMenu;
    setSpaceAddMenu(null);
    setSpaceCollapsed((prev) => ({ ...prev, [sid]: false }));
    setFilesEnabled(sid);
    setPageMenu(null);
    void navigate(`/o/${oid}/p/${sid}/files`);
  }

  function openIdeFromSpaceMenu() {
    if (!spaceAddMenu) {
      return;
    }
    const { orgId: oid, spaceId: sid } = spaceAddMenu;
    setSpaceAddMenu(null);
    setSpaceCollapsed((prev) => ({ ...prev, [sid]: false }));
    setIdeEnabled(sid);
    setPageMenu(null);
    void navigate(`/o/${oid}/p/${sid}/ide`);
  }

  function openTerminalFromSpaceMenu() {
    if (!spaceAddMenu) {
      return;
    }
    const { orgId: oid, spaceId: sid } = spaceAddMenu;
    setSpaceAddMenu(null);
    setSpaceCollapsed((prev) => ({ ...prev, [sid]: false }));
    setTerminalEnabled(sid);
    setPageMenu(null);
    void navigate(`/o/${oid}/p/${sid}/terminal`);
  }

  function openAutomationsFromSpaceMenu() {
    if (!spaceAddMenu) {
      return;
    }
    const { orgId: oid, spaceId: sid } = spaceAddMenu;
    setSpaceAddMenu(null);
    setSpaceCollapsed((prev) => ({ ...prev, [sid]: false }));
    setAutomationsEnabled(sid);
    setPageMenu(null);
    void navigate(`/o/${oid}/p/${sid}/automations`);
  }

  function openDatasetsFromSpaceMenu() {
    if (!spaceAddMenu) {
      return;
    }
    const { orgId: oid, spaceId: sid } = spaceAddMenu;
    setSpaceAddMenu(null);
    setSpaceCollapsed((prev) => ({ ...prev, [sid]: false }));
    setDatasetsPageEnabled(sid);
    setPageMenu(null);
    void navigate(`/o/${oid}/p/${sid}/datasets`);
  }

  return (
    <>
      <aside
        className="relative z-20 flex h-full min-h-0 w-64 min-w-64 shrink-0 flex-col border-r border-border bg-card"
        onClick={handleSidebarClick}
        onContextMenuCapture={handleSidebarContextMenu}
      >
        <div className="flex items-center gap-2 border-b border-border px-3 py-3">
          <span className="flex h-8 w-8 items-center justify-center rounded-sm bg-primary/15 text-primary">
            <Zap className="h-4 w-4" aria-hidden />
          </span>
          <span className="font-semibold tracking-tight text-foreground">
            Hyperspeed
          </span>
          <button
            type="button"
            className="ml-auto rounded-sm p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
            title="Search (coming soon)"
            aria-label="Search"
          >
            <Search className="h-4 w-4" />
          </button>
        </div>

        <div className="flex items-center gap-2 border-b border-border px-3 py-2">
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-secondary text-sm font-medium text-secondary-foreground">
            {initial}
          </span>
          <span className="min-w-0 truncate text-xs text-muted-foreground" title={email}>
            {email || "—"}
          </span>
        </div>

        <nav className="flex flex-col gap-0.5 px-2 py-3">
          <NavLink to="/" end className={navLinkClass}>
            <LayoutDashboard className="h-4 w-4 shrink-0 opacity-80" />
            Dashboard
          </NavLink>
          <NavLink to="/inbox" className={navLinkClass}>
            <Inbox className="h-4 w-4 shrink-0 opacity-80" />
            <span className="min-w-0 flex-1 truncate">Inbox</span>
            {inboxUnreadTotal > 0 ? (
              <span
                className="shrink-0 rounded-md bg-destructive px-1.5 py-0.5 text-center text-[11px] font-semibold tabular-nums leading-none text-destructive-foreground"
                aria-label={`${inboxUnreadTotal} unread notifications`}
              >
                {inboxUnreadTotal > 99 ? "99+" : inboxUnreadTotal}
              </span>
            ) : null}
          </NavLink>
          <NavLink to="/peek" className={navLinkClass}>
            <Search className="h-4 w-4 shrink-0 opacity-80" />
            Peek
          </NavLink>
        </nav>

        <div className="px-3 pb-1">
          <p className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
            Workspaces
          </p>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto px-2">
          <ul className="space-y-0.5 pb-2">
            {orgs.map((o, i) => {
              const spacesQ = spaceQueries[i];
              const spaces = asSpaceList(spacesQ?.data);
              const spacesListPending = Boolean(spacesQ?.isPending);
              const isOpen = expanded[o.id] ?? false;
              const isActiveOrg = orgId === o.id;
              return (
                <li key={o.id} data-workspace-id={o.id}>
                  <div
                    className="flex items-stretch gap-0.5 rounded-sm"
                    data-workspace-header
                    data-org-id={o.id}
                  >
                    <button
                      type="button"
                      aria-expanded={isOpen}
                      className="flex w-6 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:bg-accent hover:text-foreground"
                      onClick={() =>
                        setExpanded((prev) => ({
                          ...prev,
                          [o.id]: !prev[o.id],
                        }))
                      }
                    >
                      <ChevronRight
                        className={`h-3.5 w-3.5 transition-transform ${isOpen ? "rotate-90" : ""}`}
                      />
                    </button>
                    <button
                      type="button"
                      aria-expanded={isOpen}
                      title="Expand or collapse workspace"
                      className={[
                        "min-w-0 flex-1 rounded-sm px-2 py-1.5 text-left text-sm transition-colors",
                        isActiveOrg
                          ? "bg-accent/60 text-foreground"
                          : "text-muted-foreground hover:bg-accent/40 hover:text-foreground",
                      ].join(" ")}
                      onClick={(e) => {
                        e.stopPropagation();
                        setExpanded((prev) => ({
                          ...prev,
                          [o.id]: !prev[o.id],
                        }));
                      }}
                    >
                      <span className="flex items-center gap-2">
                        <span
                          className="h-2 w-2 shrink-0 rounded-full"
                          style={orgDotStyle(o.id)}
                          aria-hidden
                        />
                        <span className="min-w-0 truncate">{o.name}</span>
                      </span>
                    </button>
                    {isOrgAdmin.get(o.id) ? (
                      <button
                        type="button"
                        className="flex items-center justify-center rounded-sm px-2 text-muted-foreground hover:bg-accent hover:text-foreground"
                        title="Workspace settings"
                        aria-label="Workspace settings"
                        onClick={(e) => {
                          e.stopPropagation();
                          void navigate(`/o/${o.id}/settings`);
                        }}
                      >
                        <Settings className="h-4 w-4 opacity-80" />
                      </button>
                    ) : null}
                  </div>
                  {isOpen && (
                    <div className="ml-4 mt-0.5 border-l border-border pl-2">
                      <p className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                        Spaces
                      </p>
                      <ul className="space-y-0.5">
                        {spaces.map((r) => {
                          const branch = branchBySpaceId.get(r.id);
                          const boards = branch?.boards ?? [];
                          const chats = branch?.chats ?? [];
                          const showFiles = isFilesEnabled(r.id);
                          const showIde = isIdeEnabled(r.id);
                          const orgHasDatasetsFeature =
                            datasetsEnabledByOrgId.get(o.id) === true;
                          const showDatasets =
                            orgHasDatasetsFeature && isDatasetsPageEnabled(r.id);
                          const locked = !canAccessSpace(o.id, r.id);
                          const spaceActive = location.pathname.startsWith(
                            `/o/${o.id}/p/${r.id}/`,
                          );
                          const branchOpen = !isSpaceBranchCollapsed(r.id);
                          return (
                            <li key={r.id}>
                              <div
                                className="flex items-stretch gap-0.5 rounded-sm"
                                data-space-item
                                data-space-id={r.id}
                                data-org-id={o.id}
                              >
                                <button
                                  type="button"
                                  aria-expanded={branchOpen}
                                  className="flex w-6 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:bg-accent hover:text-foreground"
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    toggleSpaceBranch(r.id);
                                  }}
                                >
                                  <ChevronRight
                                    className={`h-3.5 w-3.5 transition-transform ${branchOpen ? "rotate-90" : ""}`}
                                  />
                                </button>
                                <button
                                  type="button"
                                  aria-expanded={branchOpen}
                                  className="flex min-w-0 flex-1 items-center gap-1.5 rounded-sm px-2 py-1 text-left text-sm text-muted-foreground hover:bg-accent/30 hover:text-foreground"
                                  onClick={() => toggleSpaceBranch(r.id)}
                                >
                                  <Hash className="h-3.5 w-3.5 shrink-0 opacity-70" />
                                  <span
                                    className={`min-w-0 truncate ${spaceActive ? "font-medium text-foreground" : ""}`}
                                  >
                                    {r.name}
                                  </span>
                                  {locked ? <Lock className="ml-auto h-3.5 w-3.5 shrink-0 opacity-60" /> : null}
                                </button>
                              </div>
                              {branchOpen && (
                                <ul className="ml-4 mt-0.5 space-y-0.5 border-l border-border pl-2">
                                  {locked ? (
                                    <li>
                                      <div className="flex items-center gap-1.5 rounded-sm px-2 py-1 text-sm text-muted-foreground/80">
                                        <Lock className="h-3.5 w-3.5 shrink-0 opacity-70" />
                                        <span className="min-w-0 truncate">Locked</span>
                                      </div>
                                    </li>
                                  ) : null}
                                  {boards.map((b) => (
                                    <li key={`b-${b.id}`}>
                                      <NavLink
                                        to={`/o/${o.id}/p/${r.id}/b/${b.id}`}
                                        data-page-item
                                        data-page-kind="board"
                                        data-org-id={o.id}
                                        data-space-id={r.id}
                                        data-page-id={b.id}
                                        data-page-label={b.name}
                                        className={({ isActive }) =>
                                          [
                                            "flex items-center gap-1.5 rounded-sm px-2 py-1 text-sm",
                                            isActive
                                              ? "bg-accent/50 text-foreground"
                                              : "text-muted-foreground hover:bg-accent/30 hover:text-foreground",
                                          ].join(" ")
                                        }
                                      >
                                        <LayoutGrid className="h-3.5 w-3.5 shrink-0 opacity-70" />
                                        <span className="min-w-0 truncate">{b.name}</span>
                                      </NavLink>
                                    </li>
                                  ))}
                                  {chats.map((c) => (
                                    <li key={`c-${c.id}`}>
                                      <NavLink
                                        to={`/o/${o.id}/p/${r.id}/c/${c.id}`}
                                        data-page-item
                                        data-page-kind="chat"
                                        data-org-id={o.id}
                                        data-space-id={r.id}
                                        data-page-id={c.id}
                                        data-page-label={c.name}
                                        className={({ isActive }) =>
                                          [
                                            "flex items-center gap-1.5 rounded-sm px-2 py-1 text-sm",
                                            isActive
                                              ? "bg-accent/50 text-foreground"
                                              : "text-muted-foreground hover:bg-accent/30 hover:text-foreground",
                                          ].join(" ")
                                        }
                                      >
                                        <MessageCircle className="h-3.5 w-3.5 shrink-0 opacity-70" />
                                        <span className="min-w-0 truncate">
                                          {["new chat", "chat"].includes(c.name.toLowerCase())
                                            ? "Chat"
                                            : c.name}
                                        </span>
                                      </NavLink>
                                    </li>
                                  ))}
                                  {showIde && !locked && (
                                    <li>
                                      <NavLink
                                        to={`/o/${o.id}/p/${r.id}/ide`}
                                        data-page-item
                                        data-page-kind="ide"
                                        data-org-id={o.id}
                                        data-space-id={r.id}
                                        data-page-label="IDE"
                                        className={({ isActive }) =>
                                          [
                                            "flex items-center gap-1.5 rounded-sm px-2 py-1 text-sm",
                                            isActive
                                              ? "bg-accent/50 text-foreground"
                                              : "text-muted-foreground hover:bg-accent/30 hover:text-foreground",
                                          ].join(" ")
                                        }
                                      >
                                        <Code2 className="h-3.5 w-3.5 shrink-0 opacity-70" />
                                        <span className="min-w-0 truncate">IDE</span>
                                      </NavLink>
                                    </li>
                                  )}
                                  {showFiles && !locked && (
                                    <li>
                                      <NavLink
                                        to={`/o/${o.id}/p/${r.id}/files`}
                                        data-page-item
                                        data-page-kind="files"
                                        data-org-id={o.id}
                                        data-space-id={r.id}
                                        data-page-label="Files"
                                        className={({ isActive }) =>
                                          [
                                            "flex items-center gap-1.5 rounded-sm px-2 py-1 text-sm",
                                            isActive
                                              ? "bg-accent/50 text-foreground"
                                              : "text-muted-foreground hover:bg-accent/30 hover:text-foreground",
                                          ].join(" ")
                                        }
                                        onDragOver={(e) => {
                                          // Don't call getData() here—some browsers keep it empty until drop.
                                          if (!e.dataTransfer.types.includes(dndNodeMime)) return;
                                          e.preventDefault();
                                          e.dataTransfer.dropEffect = "move";
                                        }}
                                        onDrop={(e) => {
                                          const dragged = getDraggedNode(e);
                                          if (!dragged) return;
                                          e.preventDefault();
                                          e.stopPropagation();
                                          moveNode.mutate({
                                            orgId: o.id,
                                            spaceId: r.id,
                                            nodeId: dragged.id,
                                            parentId: null,
                                          });
                                        }}
                                      >
                                        <Folder className="h-3.5 w-3.5 shrink-0 opacity-70" />
                                        <span className="min-w-0 flex-1 truncate">Files</span>
                                        <button
                                          type="button"
                                          className="rounded-sm p-1 text-muted-foreground hover:bg-accent/30 hover:text-foreground"
                                          title={isFilesBranchCollapsed(r.id) ? "Expand files" : "Collapse files"}
                                          aria-label={
                                            isFilesBranchCollapsed(r.id) ? "Expand files" : "Collapse files"
                                          }
                                          onClick={(e) => {
                                            e.preventDefault();
                                            e.stopPropagation();
                                            toggleFilesBranch(r.id);
                                          }}
                                        >
                                          <ChevronRight
                                            className={[
                                              "h-3.5 w-3.5 opacity-70 transition-transform",
                                              isFilesBranchCollapsed(r.id) ? "" : "rotate-90",
                                            ].join(" ")}
                                          />
                                        </button>
                                      </NavLink>
                                      {!isFilesBranchCollapsed(r.id) ? (
                                        <div className="pl-6">
                                          <FilesTree
                                            orgId={o.id}
                                            spaceId={r.id}
                                            nodesByParent={branch?.filesByParent}
                                            isFolderOpen={(folderId) => isFilesFolderExpanded(r.id, folderId)}
                                            onToggleFolder={(folderId) => toggleFilesFolder(r.id, folderId)}
                                            onMoveNode={({ nodeId, parentId }) =>
                                              moveNode.mutate({
                                                orgId: o.id,
                                                spaceId: r.id,
                                                nodeId,
                                                parentId,
                                              })
                                            }
                                          />
                                        </div>
                                      ) : null}
                                    </li>
                                  )}
                                  {showDatasets && !locked && (
                                    <li>
                                      <NavLink
                                        to={`/o/${o.id}/p/${r.id}/datasets`}
                                        data-page-item
                                        data-page-kind="datasets"
                                        data-org-id={o.id}
                                        data-space-id={r.id}
                                        data-page-label="Datasets"
                                        className={({ isActive }) =>
                                          [
                                            "flex items-center gap-1.5 rounded-sm px-2 py-1 text-sm",
                                            isActive
                                              ? "bg-accent/50 text-foreground"
                                              : "text-muted-foreground hover:bg-accent/30 hover:text-foreground",
                                          ].join(" ")
                                        }
                                      >
                                        <Table2 className="h-3.5 w-3.5 shrink-0 opacity-70" />
                                        <span className="min-w-0 truncate">Datasets</span>
                                      </NavLink>
                                    </li>
                                  )}
                                  {isAutomationsEnabled(r.id) && !locked && (
                                    <li>
                                      <NavLink
                                        to={`/o/${o.id}/p/${r.id}/automations`}
                                        data-page-item
                                        data-page-kind="automations"
                                        data-org-id={o.id}
                                        data-space-id={r.id}
                                        data-page-label="Automations"
                                        className={({ isActive }) =>
                                          [
                                            "flex items-center gap-1.5 rounded-sm px-2 py-1 text-sm",
                                            isActive
                                              ? "bg-accent/50 text-foreground"
                                              : "text-muted-foreground hover:bg-accent/30 hover:text-foreground",
                                          ].join(" ")
                                        }
                                      >
                                        <Zap className="h-3.5 w-3.5 shrink-0 opacity-70" />
                                        <span className="min-w-0 truncate">Automations</span>
                                      </NavLink>
                                    </li>
                                  )}
                                  {isTerminalEnabled(r.id) && !locked && (
                                    <li>
                                      <NavLink
                                        to={`/o/${o.id}/p/${r.id}/terminal`}
                                        data-page-item
                                        data-page-kind="terminal"
                                        data-org-id={o.id}
                                        data-space-id={r.id}
                                        data-page-label="Terminal"
                                        className={({ isActive }) =>
                                          [
                                            "flex items-center gap-1.5 rounded-sm px-2 py-1 text-sm",
                                            isActive
                                              ? "bg-accent/50 text-foreground"
                                              : "text-muted-foreground hover:bg-accent/30 hover:text-foreground",
                                          ].join(" ")
                                        }
                                      >
                                        <Terminal className="h-3.5 w-3.5 shrink-0 opacity-70" />
                                        <span className="min-w-0 truncate">Terminal</span>
                                      </NavLink>
                                    </li>
                                  )}
                                </ul>
                              )}
                            </li>
                          );
                        })}
                      </ul>
                      {spacesListPending ? (
                        <p className="px-2 py-1 text-xs text-muted-foreground">Loading spaces…</p>
                      ) : spaces.length === 0 ? (
                        <p className="px-2 py-1 text-xs text-muted-foreground">
                          Right-click the workspace name or empty sidebar area to add a space
                        </p>
                      ) : null}
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
          {orgs.length === 0 && !orgsQ.isLoading && (
            <p className="px-2 text-xs text-muted-foreground">No workspaces yet.</p>
          )}
        </div>

        <div className="mt-auto flex items-center justify-between border-t border-border px-2 py-2">
          <button
            type="button"
            onClick={() => void navigate("/")}
            className="rounded-sm p-2 text-muted-foreground hover:bg-accent hover:text-foreground"
            title="Add workspace"
            aria-label="Add workspace"
          >
            <Plus className="h-4 w-4" />
          </button>
          <a
            href="https://github.com"
            target="_blank"
            rel="noreferrer"
            className="flex items-center gap-1 rounded-sm px-2 py-1.5 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <BookOpen className="h-3.5 w-3.5" />
            Docs
          </a>
          <button
            type="button"
            onClick={() => void logout()}
            className="rounded-sm px-2 py-1.5 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            Sign out
          </button>
        </div>
      </aside>

      {ctxMenu && (
        <div
          className="fixed z-[100] min-w-[200px] rounded-sm border border-border bg-popover py-1 text-sm shadow-md"
          style={{ left: ctxMenu.x, top: ctxMenu.y }}
          role="menu"
          onClick={(e) => e.stopPropagation()}
        >
          <button
            type="button"
            role="menuitem"
            className="flex w-full items-center gap-2 px-3 py-2 text-left text-popover-foreground hover:bg-accent"
            onClick={openCreateSpaceFromMenu}
          >
            <Hash className="h-4 w-4 opacity-70" />
            Create space
          </button>
        </div>
      )}

      {spaceAddMenu && (
        <div
          className="fixed z-[100] min-w-[220px] rounded-sm border border-border bg-popover py-1 text-sm shadow-md"
          style={{ left: spaceAddMenu.x, top: spaceAddMenu.y }}
          role="menu"
          onClick={(e) => e.stopPropagation()}
        >
          <p className="px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            Add page
          </p>
          <button
            type="button"
            role="menuitem"
            className="flex w-full items-center gap-2 px-3 py-2 text-left text-popover-foreground hover:bg-accent disabled:opacity-50"
            disabled={addTaskBoardInSpace.isPending}
            onClick={() => {
              if (!spaceAddMenu) {
                return;
              }
              addTaskBoardInSpace.mutate({
                orgId: spaceAddMenu.orgId,
                spaceId: spaceAddMenu.spaceId,
              });
            }}
          >
            <LayoutGrid className="h-4 w-4 opacity-70" />
            Task board
          </button>
          <button
            type="button"
            role="menuitem"
            className="flex w-full items-center gap-2 px-3 py-2 text-left text-popover-foreground hover:bg-accent disabled:opacity-50"
            disabled={addChatInSpace.isPending}
            onClick={() => {
              if (!spaceAddMenu) {
                return;
              }
              addChatInSpace.mutate({
                orgId: spaceAddMenu.orgId,
                spaceId: spaceAddMenu.spaceId,
              });
            }}
          >
            <MessageCircle className="h-4 w-4 opacity-70" />
            Chat
          </button>
          <button
            type="button"
            role="menuitem"
            className="flex w-full items-center gap-2 px-3 py-2 text-left text-popover-foreground hover:bg-accent"
            onClick={openFilesFromSpaceMenu}
          >
            <Folder className="h-4 w-4 opacity-70" />
            Files
          </button>
          <button
            type="button"
            role="menuitem"
            className="flex w-full items-center gap-2 px-3 py-2 text-left text-popover-foreground hover:bg-accent"
            onClick={openIdeFromSpaceMenu}
          >
            <Code2 className="h-4 w-4 opacity-70" />
            IDE
          </button>
          {datasetsEnabledByOrgId.get(spaceAddMenu.orgId) ? (
            <button
              type="button"
              role="menuitem"
              className="flex w-full items-center gap-2 px-3 py-2 text-left text-popover-foreground hover:bg-accent"
              onClick={openDatasetsFromSpaceMenu}
            >
              <Table2 className="h-4 w-4 opacity-70" />
              Datasets
            </button>
          ) : null}
          <button
            type="button"
            role="menuitem"
            className="flex w-full items-center gap-2 px-3 py-2 text-left text-popover-foreground hover:bg-accent"
            onClick={openTerminalFromSpaceMenu}
          >
            <Terminal className="h-4 w-4 opacity-70" />
            Terminal
          </button>
          <button
            type="button"
            role="menuitem"
            className="flex w-full items-center gap-2 px-3 py-2 text-left text-popover-foreground hover:bg-accent"
            onClick={openAutomationsFromSpaceMenu}
          >
            <Zap className="h-4 w-4 opacity-70" />
            Automations
          </button>
        </div>
      )}

      {pageMenu && (
        <div
          className="fixed z-[100] min-w-[220px] rounded-sm border border-border bg-popover py-1 text-sm shadow-md"
          style={{ left: pageMenu.x, top: pageMenu.y }}
          role="menu"
          onClick={(e) => e.stopPropagation()}
        >
          <p className="px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            {pageMenu.label}
          </p>
          <button
            type="button"
            role="menuitem"
            className="flex w-full items-center gap-2 px-3 py-2 text-left text-destructive hover:bg-accent disabled:opacity-50"
            disabled={deleteBoard.isPending || deleteChat.isPending}
            onClick={() => {
              if (pageMenu.kind === "board") {
                deleteBoard.mutate({
                  orgId: pageMenu.orgId,
                  spaceId: pageMenu.spaceId,
                  boardId: pageMenu.pageId,
                });
                return;
              }
              if (pageMenu.kind === "chat") {
                deleteChat.mutate({
                  orgId: pageMenu.orgId,
                  spaceId: pageMenu.spaceId,
                  chatRoomId: pageMenu.pageId,
                });
                return;
              }
              if (pageMenu.kind === "files") {
                clearFilesEnabled(pageMenu.spaceId);
              } else if (pageMenu.kind === "ide") {
                clearIdeEnabled(pageMenu.spaceId);
              } else if (pageMenu.kind === "terminal") {
                clearTerminalEnabled(pageMenu.spaceId);
              } else if (pageMenu.kind === "automations") {
                clearAutomationsEnabled(pageMenu.spaceId);
              } else if (pageMenu.kind === "datasets") {
                clearDatasetsPageEnabled(pageMenu.spaceId);
              }
              const sid = pageMenu.spaceId;
              const oid = pageMenu.orgId;
              const path = location.pathname;
              const leaveIfOn = (segment: string) => {
                if (path.includes(`/p/${sid}/${segment}`)) {
                  void navigate(`/o/${oid}/p/${sid}`);
                }
              };
              setPageMenu(null);
              if (pageMenu.kind === "files") {
                leaveIfOn("files");
              } else if (pageMenu.kind === "ide") {
                leaveIfOn("ide");
              } else if (pageMenu.kind === "terminal") {
                leaveIfOn("terminal");
              } else if (pageMenu.kind === "automations") {
                leaveIfOn("automations");
              } else if (pageMenu.kind === "datasets") {
                leaveIfOn("datasets");
              }
            }}
          >
            Delete page
          </button>
        </div>
      )}

      {createModalOrgId && (
        <div
          className="fixed inset-y-0 right-0 left-64 z-[101] flex items-center justify-center bg-black/60 px-4"
          onClick={() => {
            setCreateModalOrgId(null);
            setCreateName("");
          }}
        >
          <div
            className="w-full max-w-sm rounded-sm border border-border bg-card p-4 shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="text-sm font-semibold text-card-foreground">
              New space
            </h2>
            <p className="mt-1 text-xs text-muted-foreground">
              Spaces use the same task board as before — pick a name for this
              space.
            </p>
            <form onSubmit={onCreateSpaceSubmit} className="mt-4 space-y-3">
              <input
                autoFocus
                className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
                placeholder="space-name"
                value={createName}
                onChange={(e) => setCreateName(e.target.value)}
              />
              <div className="flex justify-end gap-2">
                <button
                  type="button"
                  className="rounded-sm border border-border px-3 py-1.5 text-sm hover:bg-accent"
                  onClick={() => {
                    setCreateModalOrgId(null);
                    setCreateName("");
                  }}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={createSpace.isPending || !createName.trim()}
                  className="rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                >
                  Create space
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  );
}
