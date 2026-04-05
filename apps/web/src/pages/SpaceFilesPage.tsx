import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Editor } from "@monaco-editor/react";
import { MonacoDiffViewer } from "../components/MonacoDiffViewer";
import {
  ArrowLeft,
  Bot,
  Bug,
  ChevronRight,
  Code2,
  Download,
  Eye,
  File,
  FilePlus2,
  Files,
  Folder,
  GitBranch,
  PanelLeft,
  PanelRight,
  Play,
  Search,
  Send,
  Sparkles,
  Upload,
} from "lucide-react";
import type * as Monaco from "monaco-editor";
import { DragEvent, KeyboardEvent, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate, useSearchParams, useParams } from "react-router-dom";
import { AgentInvokeError, invokeAgentTool } from "../api/agentTools";
import { apiFetch } from "../api/http";
import { createPreviewSession, deletePreviewSession } from "../api/previewSessions";
import type { AgentChatMode, FileEditProposal, FileNode, Project, UUID } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { useOrgRealtime } from "../hooks/useOrgRealtime";
import { useSpaceFileCollab } from "../hooks/useSpaceFileCollab";
import { FilesRichEditor } from "../components/FilesRichEditor";
import { IdeHtmlPreview } from "../components/IdeHtmlPreview";
import { IdeSourceControlPanel } from "../components/IdeSourceControlPanel";
import { sanitizeRichHtml } from "../lib/richEditorSanitize";

const defaultNewFileName = "(insert file name here).txt";
const dndNodeMime = "application/x-hyperspeed-node";
const ideExplorerStorageKey = (spaceId?: string) => `ide-explorer-open:${spaceId ?? "unknown"}`;
const idePanelStorageKey = (spaceId?: string) => `ide-ai-panel-open:${spaceId ?? "unknown"}`;
const idePanelSessionKey = (spaceId?: string) => `ide-ai-panel-session:${spaceId ?? "unknown"}`;
const ideChatModeStorageKey = (spaceId?: string) => `ide-ai-chat-mode:${spaceId ?? "unknown"}`;
const idePreviewStorageKey = (spaceId?: string) => `ide-preview-open:${spaceId ?? "unknown"}`;
const ideMainViewStorageKey = (spaceId?: string) => `ide-main-view:${spaceId ?? "unknown"}`;
const idePreviewModeStorageKey = (spaceId?: string) => `ide-preview-mode:${spaceId ?? "unknown"}`;

function inferIdeToolAction(mode: AgentChatMode, prompt: string, hasFile: boolean): "propose" | "read" | "list" {
  const listIntent = /\b(list|ls|files in|folder|directory|dir|contents of|what'?s in|show (me )?files)\b/i.test(
    prompt,
  );
  const editIntent = /\b(edit|change|fix|replace|patch|propose|write|add|refactor|update|implement|delete|remove|apply)\b/i.test(
    prompt,
  );
  if (mode === "ask" || mode === "plan") {
    if (listIntent || !hasFile) return "list";
    return "read";
  }
  if (editIntent && hasFile) return "propose";
  if (listIntent) return "list";
  if (hasFile) return "read";
  return "list";
}

function wantsEditInReadOnlyMode(prompt: string): boolean {
  return /\b(edit|change|fix|replace|patch|propose|write|add|refactor|update|implement|delete|remove|apply)\b/i.test(
    prompt,
  );
}

const enableYjsCollab = import.meta.env.VITE_YJS_COLLAB === "true";

/** Extensions we treat as code on the Files page (Monaco toggle). */
const CODE_FILE_EXTENSIONS = new Set(
  [
    "ts",
    "tsx",
    "mts",
    "cts",
    "js",
    "jsx",
    "mjs",
    "cjs",
    "go",
    "rs",
    "py",
    "pyw",
    "rb",
    "php",
    "java",
    "kt",
    "kts",
    "swift",
    "c",
    "h",
    "cc",
    "cpp",
    "cxx",
    "hpp",
    "hh",
    "cs",
    "fs",
    "fsx",
    "scala",
    "clj",
    "hs",
    "elm",
    "vue",
    "svelte",
    "css",
    "scss",
    "sass",
    "less",
    "html",
    "htm",
    "xml",
    "svg",
    "json",
    "jsonc",
    "yaml",
    "yml",
    "toml",
    "ini",
    "mdx",
    "sql",
    "sh",
    "bash",
    "zsh",
    "ps1",
    "bat",
    "cmd",
    "dockerfile",
    "r",
    "lua",
    "pl",
    "pm",
    "gradle",
    "properties",
    "env",
    "ex",
    "exs",
  ].map((e) => e.toLowerCase()),
);

/** Extensions that stay a plain textarea in simple view (not rich HTML). */
const PLAIN_TEXT_EXTENSIONS = new Set(["csv", "tsv", "log", "json"].map((e) => e.toLowerCase()));

function isPlainTextExtension(fileName: string): boolean {
  const ext = fileName.split(".").pop()?.toLowerCase() ?? "";
  return PLAIN_TEXT_EXTENSIONS.has(ext);
}

function fileLooksLikeCode(fileName: string, mimeType?: string | null): boolean {
  const ext = fileName.split(".").pop()?.toLowerCase() ?? "";
  if (CODE_FILE_EXTENSIONS.has(ext)) return true;
  const base = fileName.split("/").pop()?.toLowerCase() ?? "";
  if (base === "dockerfile" || base === "makefile" || base.endsWith("ignore")) return true;
  if (mimeType) {
    const m = mimeType.toLowerCase();
    if (
      m.includes("javascript") ||
      m.includes("typescript") ||
      m.includes("json") ||
      m.includes("xml") ||
      m.includes("html") ||
      m.includes("css")
    ) {
      return true;
    }
  }
  return false;
}

function monacoLanguageFromFileName(name: string): string {
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  switch (ext) {
    case "ts":
    case "tsx":
    case "mts":
    case "cts":
      return "typescript";
    case "js":
    case "jsx":
    case "mjs":
    case "cjs":
      return "javascript";
    case "go":
      return "go";
    case "md":
    case "mdx":
      return "markdown";
    case "json":
    case "jsonc":
      return "json";
    case "css":
    case "scss":
    case "sass":
    case "less":
      return "css";
    case "html":
    case "htm":
      return "html";
    case "yaml":
    case "yml":
      return "yaml";
    default:
      return "plaintext";
  }
}

type IdeAgentMessage = {
  id: string;
  role: "user" | "assistant";
  text: string;
  /** Set when message was produced in Plan mode (read-only planning tone). */
  variant?: "plan";
  card?:
    | { kind: "proposal"; proposalId: UUID; nodeId: UUID }
    | { kind: "direct-apply"; prompt: string; content: string };
};

export default function SpaceFilesPage({ mode }: { mode?: "ide" | "files" } = {}) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const { state: authState } = useAuth();
  const me = authState.status === "authenticated" ? authState.user : null;
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();
  const [sp, setSp] = useSearchParams();
  const parentId = (sp.get("parentId") || "").trim() || null;
  const fileId = (sp.get("fileId") || "").trim() || null;
  const projectFolderId = (sp.get("projectFolderId") || "").trim() || null;
  const isIDE = mode ? mode === "ide" : location.pathname.endsWith("/ide");
  const pageBase = `/o/${orgId}/p/${projectId}/${isIDE ? "ide" : "files"}`;

  useOrgRealtime(orgId, !!orgId);

  const [q, setQ] = useState("");
  const [scope, setScope] = useState<"folder" | "space">("folder");
  const [selected, setSelected] = useState<FileNode | null>(null);
  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [newFolderName, setNewFolderName] = useState("");
  const [moveOpen, setMoveOpen] = useState<FileNode | null>(null);
  const [moveDest, setMoveDest] = useState<UUID | "root">("root");
  const [fileNameDraft, setFileNameDraft] = useState(defaultNewFileName);
  const [fileNameTouched, setFileNameTouched] = useState(false);
  const [contentDraft, setContentDraft] = useState("");
  const [saveState, setSaveState] = useState<"idle" | "saving" | "saved" | "error">("idle");
  const [reviewProposal, setReviewProposal] = useState<FileEditProposal | null>(null);
  const [ideTabs, setIdeTabs] = useState<Array<{ id: UUID; name: string }>>([]);
  const [selectionRange, setSelectionRange] = useState<{
    startLineNumber: number;
    startColumn: number;
    endLineNumber: number;
    endColumn: number;
  } | null>(null);
  const [ideExplorerOpen, setIdeExplorerOpen] = useState(true);
  const [ideMainView, setIdeMainView] = useState<"editor" | "preview">("editor");
  /** After × on the Preview tab, hide that tab until user opens preview again (header / Play). */
  const [idePreviewTabDismissed, setIdePreviewTabDismissed] = useState(false);
  const [ideSidebarView, setIdeSidebarView] = useState<"explorer" | "source-control" | "problems">("explorer");
  const [idePreviewMode, setIdePreviewMode] = useState<"blob" | "server">("blob");
  const [ideServerSessionId, setIdeServerSessionId] = useState<string | null>(null);
  const [ideServerPreviewUrl, setIdeServerPreviewUrl] = useState<string | null>(null);
  const [ideServerLoading, setIdeServerLoading] = useState(false);
  const [ideServerError, setIdeServerError] = useState<string | null>(null);
  const ideServerSidRef = useRef<string | null>(null);
  const [ideAIPanelOpen, setIdeAIPanelOpen] = useState(true);
  const [ideAIPrompt, setIdeAIPrompt] = useState("");
  const [ideAISending, setIdeAISending] = useState(false);
  const [ideAIMessages, setIdeAIMessages] = useState<IdeAgentMessage[]>([]);
  const [ideChatMode, setIdeChatMode] = useState<AgentChatMode>("agent");
  const [enableDirectApply, setEnableDirectApply] = useState(false);
  const [pendingDirectApply, setPendingDirectApply] = useState<{ prompt: string; content: string } | null>(null);
  const [invokeFailure, setInvokeFailure] = useState<{ prompt: string; message: string; retryable: boolean } | null>(null);
  /** Files route only: start in simple text view; Monaco when user opts in (code-like files). */
  const [filesPageUseCodeEditor, setFilesPageUseCodeEditor] = useState(false);

  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const zipImportRef = useRef<HTMLInputElement | null>(null);
  const saveTimerRef = useRef<number | null>(null);
  const useRichEditorRef = useRef(false);
  const [monacoCtx, setMonacoCtx] = useState<{
    editor: Monaco.editor.IStandaloneCodeEditor;
    monaco: typeof Monaco;
  } | null>(null);
  const collabDecoIdsRef = useRef<string[]>([]);

  const projectQ = useQuery({
    queryKey: ["project", orgId, projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${projectId}`);
      if (!res.ok) throw new Error("project");
      return res.json() as Promise<Project>;
    },
  });

  const nodesQ = useQuery({
    queryKey: ["file-nodes", projectId, parentId, q, scope],
    enabled: !!orgId && !!projectId && (!isIDE || !!projectFolderId),
    queryFn: async () => {
      const qs = new URLSearchParams();
      if (parentId) qs.set("parentId", parentId);
      if (q.trim()) qs.set("q", q.trim());
      qs.set("scope", scope);
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files?${qs.toString()}`,
      );
      if (!res.ok) throw new Error("files");
      const j = (await res.json()) as { nodes: FileNode[] };
      return j.nodes;
    },
  });

  const folderQ = useQuery({
    queryKey: ["file-node", projectId, parentId],
    enabled: !!orgId && !!projectId && !!parentId && !fileId && (!isIDE || !!projectFolderId),
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/${parentId}`,
      );
      if (!res.ok) throw new Error("folder");
      const j = (await res.json()) as { node: FileNode };
      return j.node;
    },
  });

  const createFolder = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/folders`,
        {
          method: "POST",
          json: { parent_id: parentId, name: newFolderName.trim() },
        },
      );
      if (!res.ok) throw new Error("create folder");
      return res.json() as Promise<{ node: FileNode }>;
    },
    onSuccess: () => {
      setNewFolderOpen(false);
      setNewFolderName("");
      void qc.invalidateQueries({ queryKey: ["file-nodes", projectId, parentId, q, scope] });
      void qc.invalidateQueries({ queryKey: ["file-nodes", projectId, parentId, "", "folder"] });
      void qc.invalidateQueries({ queryKey: ["file-tree", projectId] });
    },
  });

  const renameNode = useMutation({
    mutationFn: async (vars: { nodeId: UUID; name: string }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/${vars.nodeId}`,
        { method: "PATCH", json: { name: vars.name } },
      );
      if (!res.ok) throw new Error("rename");
      return res.json() as Promise<{ ok: boolean }>;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["file-nodes", projectId] });
      void qc.invalidateQueries({ queryKey: ["file-tree", projectId] });
    },
  });

  const moveNode = useMutation({
    mutationFn: async (vars: { nodeId: UUID; parentId: UUID | null }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/${vars.nodeId}`,
        { method: "PATCH", json: { parent_id: vars.parentId } },
      );
      if (!res.ok) throw new Error("move");
      return res.json() as Promise<{ ok: boolean }>;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["file-nodes", projectId], exact: false });
      void qc.invalidateQueries({ queryKey: ["file-tree", projectId] });
      setSelected(null);
    },
  });

  const deleteNode = useMutation({
    mutationFn: async (nodeId: UUID) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/${nodeId}`,
        { method: "DELETE" },
      );
      if (!res.ok) throw new Error("delete");
      return true;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["file-nodes", projectId] });
      void qc.invalidateQueries({ queryKey: ["file-tree", projectId] });
      setSelected(null);
    },
  });

  const initUpload = useMutation({
    mutationFn: async (vars: { file: File; parent_id: string | null }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/upload/init`,
        {
          method: "POST",
          json: {
            parent_id: vars.parent_id,
            name: vars.file.name,
            mime_type: vars.file.type || null,
            size_bytes: vars.file.size,
          },
        },
      );
      if (!res.ok) throw new Error("upload init");
      const j = (await res.json()) as {
        node: FileNode;
        upload_url: string;
        upload_via_api_url: string;
      };
      const putTarget = j.upload_via_api_url || j.upload_url;
      const isSameOriginApiUpload =
        putTarget.startsWith("/api/") ||
        (() => {
          try {
            const u = new URL(putTarget, window.location.origin);
            return (
              u.origin === window.location.origin && u.pathname.startsWith("/api/")
            );
          } catch {
            return false;
          }
        })();
      const pathForApi = putTarget.startsWith("/")
        ? putTarget
        : (() => {
            try {
              return new URL(putTarget).pathname + new URL(putTarget).search;
            } catch {
              return putTarget;
            }
          })();
      const up = isSameOriginApiUpload
        ? await apiFetch(pathForApi, {
            method: "PUT",
            body: vars.file,
            headers: {
              "Content-Type": vars.file.type || "application/octet-stream",
            },
          })
        : await fetch(putTarget, {
            method: "PUT",
            body: vars.file,
            headers: { "Content-Type": vars.file.type || "application/octet-stream" },
          });
      if (!up.ok) throw new Error("upload failed");
      return j;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["file-nodes", projectId] });
      void qc.invalidateQueries({ queryKey: ["file-tree", projectId] });
    },
  });

  const createTextFile = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/text`,
        { method: "POST", json: { parent_id: parentId, name: defaultNewFileName } },
      );
      if (!res.ok) throw new Error("create file");
      return res.json() as Promise<{ node: FileNode }>;
    },
    onSuccess: (j) => {
      setFileNameTouched(false);
      setFileNameDraft(j.node.name);
      setContentDraft("");
      setSaveState("idle");
      setSelected(null);
      setSp((prev) => {
        const next = new URLSearchParams(prev);
        next.set("fileId", j.node.id);
        return next;
      });
      const nqs = new URLSearchParams();
      if (parentId) nqs.set("parentId", parentId);
      nqs.set("fileId", j.node.id);
      void navigate(`${pageBase}?${nqs.toString()}`);
      void qc.invalidateQueries({ queryKey: ["file-nodes", projectId] });
      void qc.invalidateQueries({ queryKey: ["file-tree", projectId] });
    },
  });

  const fileTextQ = useQuery({
    queryKey: ["file-text", projectId, fileId],
    enabled: !!orgId && !!projectId && !!fileId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/${fileId}/text`,
      );
      if (!res.ok) throw new Error("file text");
      return res.json() as Promise<{ node: FileNode; content: string }>;
    },
  });

  const collab = useSpaceFileCollab({
    orgId,
    spaceId: projectId,
    fileId,
    selfUserId: me?.id,
    displayName: (me?.display_name || me?.email || "You").trim(),
    editor: monacoCtx?.editor ?? null,
    monaco: monacoCtx?.monaco ?? null,
    enableYjs: enableYjsCollab,
    yjsContentReady: !!fileId && !!fileTextQ.isSuccess && (isIDE || filesPageUseCodeEditor),
  });

  useEffect(() => {
    setFilesPageUseCodeEditor(false);
  }, [fileId]);

  useEffect(() => {
    if (!fileId) {
      setMonacoCtx(null);
      return;
    }
    if (!isIDE && !filesPageUseCodeEditor) {
      setMonacoCtx(null);
    }
  }, [fileId, isIDE, filesPageUseCodeEditor]);

  useEffect(() => {
    const ed = monacoCtx?.editor;
    const monaco = monacoCtx?.monaco;
    if (!ed || !monaco || !fileId) {
      collabDecoIdsRef.current = [];
      return;
    }
    const colors = ["#f97316", "#22c55e", "#6366f1", "#ec4899", "#eab308"];
    const decos: Monaco.editor.IModelDeltaDecoration[] = [];
    let i = 0;
    for (const p of Object.values(collab.peers)) {
      if (!p.cursor) continue;
      const line = p.cursor.lineNumber;
      const c = colors[i % colors.length];
      i++;
      decos.push({
        range: new monaco.Range(line, 1, line, 1),
        options: {
          isWholeLine: true,
          className: "collab-peer-line",
          overviewRuler: {
            color: c,
            position: monaco.editor.OverviewRulerLane.Left,
          },
        },
      });
    }
    collabDecoIdsRef.current = ed.deltaDecorations(collabDecoIdsRef.current, decos);
    return () => {
      collabDecoIdsRef.current = ed.deltaDecorations(collabDecoIdsRef.current, []);
    };
  }, [collab.peers, monacoCtx, fileId]);

  const proposalsQ = useQuery({
    queryKey: ["file-proposals", projectId, fileId],
    enabled: !!orgId && !!projectId && !!fileId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/${fileId}/proposals`,
      );
      if (!res.ok) throw new Error("proposals");
      const j = (await res.json()) as { proposals: FileEditProposal[] };
      return j.proposals;
    },
  });

  const pendingProposals = useMemo(
    () => (proposalsQ.data ?? []).filter((p) => p.status === "pending"),
    [proposalsQ.data],
  );

  const acceptProposal = useMutation({
    mutationFn: async (proposalId: UUID) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/proposals/${proposalId}/accept`,
        { method: "POST" },
      );
      if (res.status === 409) {
        const err = await res.json().catch(() => ({}));
        throw new Error((err as { error?: string }).error || "base file changed");
      }
      if (!res.ok) throw new Error("accept");
    },
    onSuccess: () => {
      setReviewProposal(null);
      void qc.invalidateQueries({ queryKey: ["file-text", projectId, fileId] });
      void qc.invalidateQueries({ queryKey: ["file-proposals", projectId, fileId] });
    },
  });

  const rejectProposal = useMutation({
    mutationFn: async (proposalId: UUID) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/proposals/${proposalId}/reject`,
        { method: "POST" },
      );
      if (!res.ok) throw new Error("reject");
    },
    onSuccess: () => {
      setReviewProposal(null);
      void qc.invalidateQueries({ queryKey: ["file-proposals", projectId, fileId] });
    },
  });

  useEffect(() => {
    if (!fileId) {
      return;
    }
    if (fileTextQ.data) {
      setFileNameDraft(fileTextQ.data.node.name);
      setFileNameTouched(fileTextQ.data.node.name !== defaultNewFileName);
      setContentDraft(fileTextQ.data.content ?? "");
      setSaveState("idle");
    }
  }, [fileId, fileTextQ.data]);

  const renameActiveFile = useMutation({
    mutationFn: async (name: string) => {
      if (!fileId) throw new Error("no file");
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/${fileId}`,
        { method: "PATCH", json: { name } },
      );
      if (!res.ok) throw new Error("rename");
      return res.json() as Promise<{ ok: boolean }>;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["file-nodes", projectId] });
      void qc.invalidateQueries({ queryKey: ["file-text", projectId, fileId] });
      void qc.invalidateQueries({ queryKey: ["file-tree", projectId] });
    },
  });

  const saveActiveFile = useMutation({
    mutationFn: async (content: string) => {
      if (!fileId) throw new Error("no file");
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/${fileId}/text`,
        { method: "PUT", json: { content } },
      );
      if (!res.ok) throw new Error("save");
      return res.json() as Promise<{ ok: boolean }>;
    },
    onMutate: () => setSaveState("saving"),
    onSuccess: () => {
      setSaveState("saved");
      void qc.invalidateQueries({ queryKey: ["file-nodes", projectId] });
      void qc.invalidateQueries({ queryKey: ["file-text", projectId, fileId] });
      void qc.invalidateQueries({ queryKey: ["file-proposals", projectId, fileId] });
    },
    onError: () => setSaveState("error"),
  });

  const foldersInSpaceQ = useQuery({
    queryKey: ["file-folders", projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const qs = new URLSearchParams();
      qs.set("q", "");
      qs.set("scope", "space");
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files?${qs.toString()}`,
      );
      if (!res.ok) throw new Error("folders");
      const j = (await res.json()) as { nodes: FileNode[] };
      return j.nodes.filter((n) => n.kind === "folder");
    },
  });

  const nodes = nodesQ.data ?? [];
  const folders = useMemo(() => nodes.filter((n) => n.kind === "folder"), [nodes]);
  const files = useMemo(() => nodes.filter((n) => n.kind === "file"), [nodes]);
  const fileNameById = useMemo(() => {
    const m = new Map<UUID, string>();
    for (const n of files) m.set(n.id, n.name);
    return m;
  }, [files]);

  async function onPickUpload(filesList: FileList | null) {
    if (!filesList || filesList.length === 0) return;
    const arr = Array.from(filesList);
    for (const f of arr) {
      // eslint-disable-next-line no-await-in-loop
      await initUpload.mutateAsync({ file: f, parent_id: parentId });
    }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    void onPickUpload(e.dataTransfer.files);
  }

  function onDragOver(e: DragEvent) {
    e.preventDefault();
  }

  function setDraggedNode(e: React.DragEvent, n: FileNode) {
    e.dataTransfer.effectAllowed = "move";
    const payload = JSON.stringify({ id: n.id, kind: n.kind });
    // Some browsers don't reliably round-trip custom MIME types; keep a text fallback.
    e.dataTransfer.setData(dndNodeMime, payload);
    e.dataTransfer.setData("text/plain", payload);
  }

  function getDraggedNode(e: React.DragEvent): { id: UUID; kind: FileNode["kind"] } | null {
    const raw = e.dataTransfer.getData(dndNodeMime) || e.dataTransfer.getData("text/plain");
    if (!raw) return null;
    try {
      const j = JSON.parse(raw) as { id?: UUID; kind?: FileNode["kind"] };
      if (!j?.id || (j.kind !== "file" && j.kind !== "folder")) return null;
      return { id: j.id, kind: j.kind };
    } catch {
      return null;
    }
  }

  const breadcrumbLabel = useMemo(() => {
    if (!parentId) return "Files";
    if (folderQ.data?.name) return folderQ.data.name;
    return "Folder";
  }, [folderQ.data?.name, parentId]);

  function goFolderBack() {
    const nextParent = folderQ.data?.parent_id || null;
    setSelected(null);
    setSp((prev) => {
      const next = new URLSearchParams(prev);
      if (nextParent) next.set("parentId", nextParent);
      else next.delete("parentId");
      return next;
    });
    const qs = new URLSearchParams();
    if (nextParent) qs.set("parentId", nextParent);
    void navigate(`${pageBase}${qs.toString() ? `?${qs}` : ""}`);
  }

  function openFolder(nextParentId: UUID | null) {
    if (isIDE && !projectFolderId) {
      return;
    }
    if (isIDE && projectFolderId && !nextParentId) {
      return;
    }
    setSelected(null);
    setSp((prev) => {
      const next = new URLSearchParams(prev);
      if (nextParentId) next.set("parentId", nextParentId);
      else next.delete("parentId");
      if (isIDE && projectFolderId) {
        next.set("projectFolderId", projectFolderId);
      }
      next.delete("fileId");
      return next;
    });
    const qs = new URLSearchParams();
    if (nextParentId) qs.set("parentId", nextParentId);
    void navigate(`${pageBase}${qs.toString() ? `?${qs}` : ""}`);
  }

  function openFileForEditing(nextFileId: UUID) {
    if (isIDE) setIdeMainView("editor");
    setSp((prev) => {
      const next = new URLSearchParams(prev);
      next.set("fileId", nextFileId);
      return next;
    });
    const qs = new URLSearchParams();
    if (projectFolderId) qs.set("projectFolderId", projectFolderId);
    if (parentId) qs.set("parentId", parentId);
    qs.set("fileId", nextFileId);
    void navigate(`${pageBase}?${qs.toString()}`);
  }

  function openIDEFileTab(nextFileID: UUID) {
    openFileForEditing(nextFileID);
  }

  function openIdePreview() {
    setIdePreviewTabDismissed(false);
    setIdeMainView("preview");
  }

  function closeIDEFileTab(tabID: UUID) {
    setIdeTabs((prev) => {
      const idx = prev.findIndex((t) => t.id === tabID);
      if (idx < 0) return prev;
      const next = prev.filter((t) => t.id !== tabID);
      if (fileId === tabID) {
        const fallback = next[idx] ?? next[idx - 1];
        if (fallback) {
          void openIDEFileTab(fallback.id);
        } else {
          setSp((prevSp) => {
            const qs = new URLSearchParams(prevSp);
            qs.delete("fileId");
            return qs;
          });
          const qs = new URLSearchParams();
          if (projectFolderId) qs.set("projectFolderId", projectFolderId);
          if (parentId) qs.set("parentId", parentId);
          void navigate(`${pageBase}?${qs.toString()}`);
        }
      }
      return next;
    });
  }

  function selectProjectFolder(folderId: UUID) {
    setSelected(null);
    setSp((prev) => {
      const next = new URLSearchParams(prev);
      next.set("projectFolderId", folderId);
      next.set("parentId", folderId);
      next.delete("fileId");
      return next;
    });
    void navigate(
      `/o/${orgId}/p/${projectId}/ide?${new URLSearchParams({
        projectFolderId: folderId,
        parentId: folderId,
      }).toString()}`,
    );
  }

  useEffect(() => {
    if (!isIDE || !fileId) return;
    const name =
      fileTextQ.data?.node?.name || fileNameById.get(fileId) || `file-${fileId.slice(0, 8)}`;
    setIdeTabs((prev) => {
      const idx = prev.findIndex((t) => t.id === fileId);
      if (idx >= 0) {
        const next = [...prev];
        next[idx] = { id: fileId, name };
        return next;
      }
      return [...prev, { id: fileId, name }];
    });
  }, [isIDE, fileId, fileTextQ.data?.node?.name, fileNameById]);

  useEffect(() => {
    if (!isIDE) return;
    setIdePreviewTabDismissed(false);
  }, [isIDE, fileId]);

  useEffect(() => {
    if (!isIDE) return;
    try {
      const raw = localStorage.getItem(ideExplorerStorageKey(projectId));
      if (raw === "0") setIdeExplorerOpen(false);
      if (raw === "1") setIdeExplorerOpen(true);
    } catch {
      // Ignore localStorage errors.
    }
  }, [isIDE, projectId]);

  useEffect(() => {
    if (!isIDE) return;
    try {
      localStorage.setItem(ideExplorerStorageKey(projectId), ideExplorerOpen ? "1" : "0");
    } catch {
      // Ignore localStorage errors.
    }
  }, [isIDE, projectId, ideExplorerOpen]);

  useEffect(() => {
    if (!isIDE) return;
    try {
      const raw = localStorage.getItem(ideMainViewStorageKey(projectId));
      if (raw === "preview" || raw === "editor") {
        setIdeMainView(raw);
        if (raw === "preview") setIdePreviewTabDismissed(false);
        return;
      }
      const legacy = localStorage.getItem(idePreviewStorageKey(projectId));
      if (legacy === "1") {
        setIdePreviewTabDismissed(false);
        setIdeMainView("preview");
      }
    } catch {
      // Ignore localStorage errors.
    }
  }, [isIDE, projectId]);

  useEffect(() => {
    if (!isIDE) return;
    try {
      localStorage.setItem(ideMainViewStorageKey(projectId), ideMainView);
    } catch {
      // Ignore localStorage errors.
    }
  }, [isIDE, projectId, ideMainView]);

  useEffect(() => {
    if (!isIDE) return;
    try {
      const raw = localStorage.getItem(idePreviewModeStorageKey(projectId));
      if (raw === "server" || raw === "blob") setIdePreviewMode(raw);
    } catch {
      // Ignore localStorage errors.
    }
  }, [isIDE, projectId]);

  useEffect(() => {
    if (!isIDE) return;
    try {
      localStorage.setItem(idePreviewModeStorageKey(projectId), idePreviewMode);
    } catch {
      // Ignore localStorage errors.
    }
  }, [isIDE, projectId, idePreviewMode]);

  useEffect(() => {
    ideServerSidRef.current = ideServerSessionId;
  }, [ideServerSessionId]);

  useEffect(() => {
    if (!orgId || !projectId) return;
    const sid = ideServerSidRef.current;
    if (sid) {
      void deletePreviewSession(orgId, projectId, sid);
      ideServerSidRef.current = null;
    }
    setIdeServerSessionId(null);
    setIdeServerPreviewUrl(null);
    setIdeServerError(null);
  }, [fileId, orgId, projectId]);

  useEffect(() => {
    return () => {
      const sid = ideServerSidRef.current;
      if (orgId && projectId && sid) {
        void deletePreviewSession(orgId, projectId, sid);
      }
    };
  }, [orgId, projectId]);

  useEffect(() => {
    if (!isIDE) return;
    try {
      const raw = localStorage.getItem(idePanelStorageKey(projectId));
      if (raw === "0") setIdeAIPanelOpen(false);
      if (raw === "1") setIdeAIPanelOpen(true);
    } catch {
      // Ignore localStorage errors.
    }
  }, [isIDE, projectId]);

  useEffect(() => {
    if (!isIDE) return;
    try {
      localStorage.setItem(idePanelStorageKey(projectId), ideAIPanelOpen ? "1" : "0");
    } catch {
      // Ignore localStorage errors.
    }
  }, [isIDE, projectId, ideAIPanelOpen]);

  useEffect(() => {
    if (!isIDE) return;
    try {
      const raw = localStorage.getItem(ideChatModeStorageKey(projectId));
      if (raw === "ask" || raw === "plan" || raw === "agent") setIdeChatMode(raw);
    } catch {
      // Ignore localStorage errors.
    }
  }, [isIDE, projectId]);

  useEffect(() => {
    if (!isIDE) return;
    try {
      localStorage.setItem(ideChatModeStorageKey(projectId), ideChatMode);
    } catch {
      // Ignore localStorage errors.
    }
  }, [isIDE, projectId, ideChatMode]);

  useEffect(() => {
    const ed = monacoCtx?.editor;
    if (!ed || !isIDE) return;
    const updateSelection = () => {
      const sel = ed.getSelection();
      if (!sel) {
        setSelectionRange(null);
        return;
      }
      setSelectionRange({
        startLineNumber: sel.startLineNumber,
        startColumn: sel.startColumn,
        endLineNumber: sel.endLineNumber,
        endColumn: sel.endColumn,
      });
    };
    updateSelection();
    const disp = ed.onDidChangeCursorSelection(updateSelection);
    return () => disp.dispose();
  }, [monacoCtx, isIDE]);

  function closeEditor() {
    setFilesPageUseCodeEditor(false);
    setSp((prev) => {
      const next = new URLSearchParams(prev);
      next.delete("fileId");
      return next;
    });
    void navigate(
      `${pageBase}?${new URLSearchParams({
        ...(isIDE && projectFolderId ? { projectFolderId } : {}),
        ...(parentId ? { parentId } : {}),
      }).toString()}`,
    );
  }

  const fileNameIsPlaceholder = !fileNameTouched && fileNameDraft === defaultNewFileName;
  const idePanelDisabled = !projectFolderId;
  const activeFileNameForKind = fileTextQ.data?.node?.name ?? fileNameDraft;
  const filesPageShowCodeEditorToggle =
    !isIDE && fileLooksLikeCode(activeFileNameForKind, fileTextQ.data?.node?.mime_type);
  const useMonacoForBody = isIDE || filesPageUseCodeEditor;
  const ideHtmlPreviewEligible =
    isIDE && !!fileId && monacoLanguageFromFileName(activeFileNameForKind) === "html";

  useEffect(() => {
    if (!isIDE) return;
    if (!ideHtmlPreviewEligible && ideMainView === "preview") {
      setIdeMainView("editor");
    }
  }, [isIDE, ideHtmlPreviewEligible, ideMainView]);

  function switchIdePreviewMode(next: "blob" | "server") {
    if (next === "blob" && orgId && projectId && ideServerSessionId) {
      void deletePreviewSession(orgId, projectId, ideServerSessionId);
    }
    setIdeServerSessionId(null);
    setIdeServerPreviewUrl(null);
    setIdeServerError(null);
    setIdePreviewMode(next);
  }

  useEffect(() => {
    if (!isIDE || ideMainView !== "preview" || idePreviewMode !== "server" || !ideHtmlPreviewEligible) {
      return;
    }
    if (!orgId || !projectId) return;
    if (ideServerSessionId) return;
    let cancelled = false;
    setIdeServerLoading(true);
    setIdeServerError(null);
    void createPreviewSession(orgId, projectId)
      .then((s) => {
        if (cancelled) return;
        setIdeServerSessionId(s.id);
        setIdeServerPreviewUrl(s.preview_url);
      })
      .catch((e: unknown) => {
        if (!cancelled) setIdeServerError(e instanceof Error ? e.message : "Preview failed");
      })
      .finally(() => {
        if (!cancelled) setIdeServerLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [isIDE, ideMainView, idePreviewMode, ideHtmlPreviewEligible, orgId, projectId, ideServerSessionId]);

  const usePlainTextarea =
    !isIDE && !filesPageUseCodeEditor && isPlainTextExtension(activeFileNameForKind);
  const useRichEditor = !useMonacoForBody && !usePlainTextarea;
  useRichEditorRef.current = useRichEditor;

  function persistBodyContent(next: string): string {
    return useRichEditorRef.current ? sanitizeRichHtml(next) : next;
  }

  function scheduleSave(next: string) {
    if (saveTimerRef.current) {
      window.clearTimeout(saveTimerRef.current);
    }
    saveTimerRef.current = window.setTimeout(() => {
      saveTimerRef.current = null;
      saveActiveFile.mutate(persistBodyContent(next));
    }, 600);
  }

  function onEditorKeyDown(e: KeyboardEvent) {
    const isSave = (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s";
    if (!isSave) return;
    e.preventDefault();
    if (saveTimerRef.current) {
      window.clearTimeout(saveTimerRef.current);
      saveTimerRef.current = null;
    }
    saveActiveFile.mutate(persistBodyContent(contentDraft));
  }

  async function uploadImageForRichEditor(file: File) {
    const parentForUpload = fileTextQ.data?.node?.parent_id ?? parentId ?? null;
    const j = await initUpload.mutateAsync({ file, parent_id: parentForUpload });
    return j.node.id;
  }

  useEffect(() => {
    if (isIDE) return;
    if (!fileLooksLikeCode(activeFileNameForKind, fileTextQ.data?.node?.mime_type)) {
      setFilesPageUseCodeEditor(false);
    }
  }, [isIDE, activeFileNameForKind, fileTextQ.data?.node?.mime_type]);

  function ideSessionId(): string {
    try {
      const key = idePanelSessionKey(projectId);
      const existing = localStorage.getItem(key);
      if (existing) return existing;
      const created = `ide-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
      localStorage.setItem(key, created);
      return created;
    } catch {
      return `ide-${Date.now()}`;
    }
  }

  async function onSendToAIPanel(retryPrompt?: string) {
    if (!orgId || !projectId || idePanelDisabled || ideAISending || !me) return;
    const prompt = (retryPrompt ?? ideAIPrompt).trim();
    if (!prompt) return;
    setIdeAISending(true);
    setInvokeFailure(null);
    const msgID = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    if (!retryPrompt) {
      setIdeAIMessages((prev) => [...prev, { id: `${msgID}-u`, role: "user", text: prompt }]);
    }
    try {
      if ((ideChatMode === "ask" || ideChatMode === "plan") && wantsEditInReadOnlyMode(prompt)) {
        setIdeAIMessages((prev) => [
          ...prev,
          {
            id: `${msgID}-hint`,
            role: "assistant",
            text: "To create proposals or apply edits, switch to Agent mode. Ask and Plan can only read files and list folders.",
          },
        ]);
        setIdeAIPrompt("");
        return;
      }

      const hasFile = !!fileId;
      const action = inferIdeToolAction(ideChatMode, prompt, hasFile);
      const planPrefix = ideChatMode === "plan" ? "Plan (read-only):\n\n" : "";
      const assistantVariant = ideChatMode === "plan" ? ("plan" as const) : undefined;

      const baseArgs = {
        prompt,
        ide_context: {
          space_id: projectId,
          project_folder_id: projectFolderId,
          current_file_id: fileId,
          parent_id: parentId,
          selection: selectionRange,
          open_tab_ids: ideTabs.map((t) => t.id),
          mode: ideChatMode,
        },
      };

      if (action === "propose") {
        if (!fileId) throw new Error("Open a file first");
        const proposedContent = contentDraft || fileTextQ.data?.content || "";
        const result = await invokeAgentTool<{ proposal: { id: UUID; node_id: UUID } }>(orgId, {
          tool: "space.file.propose_patch",
          mode: ideChatMode,
          session_id: ideSessionId(),
          arguments: {
            space_id: projectId,
            node_id: fileId,
            proposed_content: proposedContent,
            ...baseArgs,
          },
        });
        void qc.invalidateQueries({ queryKey: ["file-proposals", projectId, fileId] });
        setIdeAIMessages((prev) => [
          ...prev,
          {
            id: `${msgID}-a`,
            role: "assistant",
            text: "Proposal created. Review and accept it when ready.",
            card: { kind: "proposal", proposalId: result.proposal.id, nodeId: result.proposal.node_id },
          },
        ]);
        if (ideChatMode === "agent" && !me.service_account && enableDirectApply) {
          setPendingDirectApply({ prompt, content: proposedContent });
          setIdeAIMessages((prev) => [
            ...prev,
            {
              id: `${msgID}-direct`,
              role: "assistant",
              text: "Direct apply is available for this human session.",
              card: { kind: "direct-apply", prompt, content: proposedContent },
            },
          ]);
        }
      } else if (action === "read") {
        if (!fileId) throw new Error("Open a file first");
        const result = await invokeAgentTool<{ content: string }>(orgId, {
          tool: "space.file.read",
          mode: ideChatMode,
          session_id: ideSessionId(),
          arguments: {
            space_id: projectId,
            node_id: fileId,
            ...baseArgs,
          },
        });
        const body = (result.content || "").slice(0, 8000) || "(empty file)";
        setIdeAIMessages((prev) => [
          ...prev,
          {
            id: `${msgID}-a`,
            role: "assistant",
            text: `${planPrefix}${body}`,
            variant: assistantVariant,
          },
        ]);
      } else {
        const result = await invokeAgentTool<{ nodes: Array<{ id: UUID; name: string; kind: string }> }>(orgId, {
          tool: "space.list_files",
          mode: ideChatMode,
          session_id: ideSessionId(),
          arguments: {
            space_id: projectId,
            parent_id: parentId ?? projectFolderId,
            ...baseArgs,
          },
        });
        const listing = (result.nodes || [])
          .slice(0, 40)
          .map((n) => `${n.kind === "folder" ? "DIR" : "FILE"}  ${n.name}`)
          .join("\n");
        setIdeAIMessages((prev) => [
          ...prev,
          {
            id: `${msgID}-a`,
            role: "assistant",
            text: `${planPrefix}${listing || "No files found in this folder."}`,
            variant: assistantVariant,
          },
        ]);
      }
      setIdeAIPrompt("");
    } catch (err) {
      const invErr =
        err instanceof AgentInvokeError ? err : new AgentInvokeError((err as Error)?.message || "invoke failed", 0);
      const text = invErr.message || "invoke failed";
      setInvokeFailure({ prompt, message: text, retryable: invErr.retryable });
      setIdeAIMessages((prev) => [...prev, { id: `${msgID}-e`, role: "assistant", text: `Error: ${text}` }]);
    } finally {
      setIdeAISending(false);
    }
  }

  return (
    <div
      className="flex h-full min-h-0 flex-1 flex-col overflow-hidden bg-background"
      onDrop={onDrop}
      onDragOver={onDragOver}
    >
      <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <header className="border-b border-border px-4 py-3">
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0">
              <p className="text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
                {projectQ.data?.name ?? "Space"}
              </p>
              <h1 className="mt-1 truncate text-base font-semibold text-foreground">
                {isIDE ? `IDE${fileId ? ` · ${breadcrumbLabel}` : ""}` : breadcrumbLabel}
              </h1>
            </div>
            <div className="flex items-center gap-2">
              {isIDE ? (
                <>
                  <button
                    type="button"
                    className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                    onClick={() => setIdeExplorerOpen((v) => !v)}
                    aria-pressed={ideExplorerOpen}
                  >
                    <PanelLeft className="h-4 w-4 opacity-80" />
                    {ideExplorerOpen ? "Hide explorer" : "Show explorer"}
                  </button>
                  {ideHtmlPreviewEligible ? (
                    <button
                      type="button"
                      className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                      onClick={() => openIdePreview()}
                      aria-pressed={ideMainView === "preview"}
                    >
                      <Eye className="h-4 w-4 opacity-80" />
                      Preview
                    </button>
                  ) : null}
                  <button
                    type="button"
                    className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                    onClick={() => setIdeAIPanelOpen((v) => !v)}
                    aria-pressed={ideAIPanelOpen}
                  >
                    <PanelRight className="h-4 w-4 opacity-80" />
                    {ideAIPanelOpen ? "Hide AI" : "Show AI"}
                  </button>
                </>
              ) : null}
              {!fileId ? (
                <>
                  {parentId ? (
                    <button
                      type="button"
                      className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                      onClick={goFolderBack}
                      title="Back"
                      aria-label="Back"
                    >
                      <ArrowLeft className="h-4 w-4 opacity-80" />
                      Back
                    </button>
                  ) : null}
                  <div className="relative">
                    <Search className="pointer-events-none absolute left-2 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <input
                      className="w-56 rounded-sm border border-input bg-background py-1.5 pr-2 pl-8 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2"
                      placeholder={scope === "space" ? "Search this space" : "Search this folder"}
                      value={q}
                      onChange={(e) => setQ(e.target.value)}
                    />
                  </div>
                  <select
                    className="rounded-sm border border-input bg-background px-2 py-1.5 text-sm text-foreground"
                    value={scope}
                    onChange={(e) => setScope(e.target.value as "folder" | "space")}
                    aria-label="Search scope"
                  >
                    <option value="folder">This folder</option>
                    <option value="space">This space</option>
                  </select>
                  <button
                    type="button"
                    className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                    onClick={() => createTextFile.mutate()}
                    disabled={createTextFile.isPending}
                  >
                    <FilePlus2 className="h-4 w-4 opacity-80" />
                    New file
                  </button>
                  <button
                    type="button"
                    className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                    onClick={() => setNewFolderOpen(true)}
                  >
                    <Folder className="h-4 w-4 opacity-80" />
                    New folder
                  </button>
                  <button
                    type="button"
                    className="inline-flex items-center gap-2 rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                    onClick={() => fileInputRef.current?.click()}
                    disabled={initUpload.isPending}
                  >
                    <Upload className="h-4 w-4" />
                    Upload
                  </button>
                  <button
                    type="button"
                    className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                    title="Download all space files as a zip (manifest included)"
                    onClick={async () => {
                      if (!orgId || !projectId) return;
                      const res = await apiFetch(
                        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/export.zip`,
                        { method: "POST" },
                      );
                      if (!res.ok) return;
                      const blob = await res.blob();
                      const url = URL.createObjectURL(blob);
                      const a = document.createElement("a");
                      a.href = url;
                      a.download = "space-export.zip";
                      a.click();
                      URL.revokeObjectURL(url);
                    }}
                  >
                    <Download className="h-4 w-4 opacity-80" />
                    Export zip
                  </button>
                  <button
                    type="button"
                    className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                    onClick={() => zipImportRef.current?.click()}
                  >
                    <Upload className="h-4 w-4 opacity-80" />
                    Import zip
                  </button>
                  <input
                    ref={zipImportRef}
                    type="file"
                    accept=".zip,application/zip"
                    className="hidden"
                    onChange={async (e) => {
                      const f = e.target.files?.[0];
                      e.target.value = "";
                      if (!f || !orgId || !projectId) return;
                      const fd = new FormData();
                      fd.set("archive", f);
                      if (parentId) fd.set("parent_id", parentId);
                      const res = await apiFetch(
                        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/import.zip`,
                        { method: "POST", body: fd },
                      );
                      if (!res.ok) return;
                      void qc.invalidateQueries({ queryKey: ["file-nodes", projectId] });
                      void qc.invalidateQueries({ queryKey: ["file-tree", projectId] });
                    }}
                  />
                </>
              ) : (
                <>
                  <button
                    type="button"
                    className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                    onClick={closeEditor}
                  >
                    <ArrowLeft className="h-4 w-4 opacity-80" />
                    Back
                  </button>
                  {isIDE ? (
                    <div className="text-xs text-muted-foreground">
                      {saveState === "saving"
                        ? "Saving…"
                        : saveState === "saved"
                          ? "Saved"
                          : saveState === "error"
                            ? "Save failed"
                            : fileTextQ.isPending
                              ? "Loading…"
                              : " "}
                    </div>
                  ) : (
                    <button
                      type="button"
                      className="rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                      onClick={() =>
                        void navigate(
                          `/o/${orgId}/p/${projectId}/ide?${new URLSearchParams({
                            ...(parentId ? { parentId } : {}),
                            ...(fileId ? { fileId } : {}),
                          }).toString()}`,
                        )
                      }
                    >
                      Open in IDE
                    </button>
                  )}
                </>
              )}
              <input
                ref={fileInputRef}
                type="file"
                multiple
                className="hidden"
                onChange={(e) => void onPickUpload(e.target.files)}
              />
            </div>
          </div>
          {!fileId ? (
            <p className="mt-2 text-xs text-muted-foreground">
              Drag & drop to upload. {nodesQ.isPending ? "Loading…" : `${nodes.length} items`}
            </p>
          ) : null}
        </header>

        <div className="flex min-h-0 flex-1">
          {isIDE ? (
            <aside className="hidden w-12 shrink-0 border-r border-border bg-card/80 md:flex md:flex-col md:items-center md:gap-3 md:pt-3">
              <button
                type="button"
                className={[
                  "rounded-sm p-2",
                  ideExplorerOpen && ideSidebarView === "explorer"
                    ? "bg-accent/50 text-foreground"
                    : "text-muted-foreground hover:bg-accent/30",
                ].join(" ")}
                title={ideExplorerOpen ? "Hide Explorer" : "Show Explorer"}
                aria-pressed={ideExplorerOpen && ideSidebarView === "explorer"}
                aria-label={ideExplorerOpen ? "Hide Explorer" : "Show Explorer"}
                onClick={() => {
                  setIdeSidebarView("explorer");
                  setIdeExplorerOpen((v) => !v);
                }}
              >
                <Files className="h-4 w-4" />
              </button>
              <button
                type="button"
                className={[
                  "rounded-sm p-2",
                  ideAIPanelOpen ? "bg-accent/50 text-foreground" : "text-muted-foreground hover:bg-accent/30",
                ].join(" ")}
                title={ideAIPanelOpen ? "Hide AI Assistant" : "Show AI Assistant"}
                aria-pressed={ideAIPanelOpen}
                aria-label={ideAIPanelOpen ? "Hide AI Assistant" : "Show AI Assistant"}
                onClick={() => setIdeAIPanelOpen((v) => !v)}
              >
                <Bot className="h-4 w-4" />
              </button>
              <button
                type="button"
                className={[
                  "rounded-sm p-2",
                  ideExplorerOpen && ideSidebarView === "source-control"
                    ? "bg-accent/50 text-foreground"
                    : "text-muted-foreground hover:bg-accent/30",
                ].join(" ")}
                title="Source Control"
                aria-pressed={ideExplorerOpen && ideSidebarView === "source-control"}
                aria-label="Source Control"
                onClick={() => {
                  setIdeSidebarView("source-control");
                  setIdeExplorerOpen(true);
                }}
              >
                <GitBranch className="h-4 w-4" />
              </button>
              {ideHtmlPreviewEligible ? (
                <button
                  type="button"
                  className={[
                    "rounded-sm p-2",
                    ideMainView === "preview" ? "bg-accent/50 text-foreground" : "text-muted-foreground hover:bg-accent/30",
                  ].join(" ")}
                  title="HTML preview"
                  aria-pressed={ideMainView === "preview"}
                  aria-label="HTML preview"
                  onClick={() => openIdePreview()}
                >
                  <Play className="h-4 w-4" />
                </button>
              ) : (
                <button
                  type="button"
                  className="rounded-sm p-2 text-muted-foreground/50 hover:bg-accent/30"
                  title="HTML preview (open an .html file)"
                  disabled
                  aria-disabled
                >
                  <Play className="h-4 w-4" />
                </button>
              )}
              <button
                type="button"
                className={[
                  "rounded-sm p-2",
                  ideExplorerOpen && ideSidebarView === "problems"
                    ? "bg-accent/50 text-foreground"
                    : "text-muted-foreground hover:bg-accent/30",
                ].join(" ")}
                title="Problems"
                aria-pressed={ideExplorerOpen && ideSidebarView === "problems"}
                aria-label="Problems"
                onClick={() => {
                  setIdeSidebarView("problems");
                  setIdeExplorerOpen(true);
                }}
              >
                <Bug className="h-4 w-4" />
              </button>
            </aside>
          ) : null}
          {isIDE && ideExplorerOpen ? (
            <aside className="hidden w-72 min-h-0 shrink-0 border-r border-border bg-card/50 lg:flex lg:flex-col">
              <div className="flex items-center justify-between border-b border-border px-3 py-2">
                <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  {ideSidebarView === "explorer" && "Explorer"}
                  {ideSidebarView === "source-control" && "Source Control"}
                  {ideSidebarView === "problems" && "Problems"}
                </div>
                <div className="flex items-center gap-1">
                  <button
                    type="button"
                    className="rounded-sm p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                    title="Collapse Explorer"
                    aria-label="Collapse Explorer"
                    onClick={() => setIdeExplorerOpen(false)}
                  >
                    <PanelLeft className="h-4 w-4" />
                  </button>
                  <Code2 className="h-4 w-4 text-muted-foreground" aria-hidden />
                </div>
              </div>
              <div className="min-h-0 flex-1 overflow-y-auto p-2">
                {ideSidebarView === "source-control" ? (
                  orgId && projectId ? (
                    <IdeSourceControlPanel orgId={orgId} spaceId={projectId} />
                  ) : (
                    <div className="rounded-sm border border-border bg-card p-3 text-sm text-muted-foreground">
                      Open a space to configure Git.
                    </div>
                  )
                ) : ideSidebarView === "problems" ? (
                  <div className="rounded-sm border border-border bg-card p-3 text-sm text-muted-foreground">
                    <p className="font-medium text-foreground">Problems</p>
                    <p className="mt-2">
                      No diagnostics wired yet. When language server support lands, issues will appear here.
                    </p>
                  </div>
                ) : !projectFolderId ? (
                  <div className="rounded-sm border border-border bg-card p-3">
                    <p className="text-sm font-medium text-foreground">Open project folder</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      Select a folder to scope the IDE. Root-level files are hidden in IDE mode.
                    </p>
                    <div className="mt-2 max-h-60 space-y-1 overflow-y-auto">
                      {(foldersInSpaceQ.data ?? [])
                        .filter((f) => !f.parent_id)
                        .map((f) => (
                          <button
                            key={`project-folder-${f.id}`}
                            type="button"
                            className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent/40"
                            onClick={() => selectProjectFolder(f.id)}
                          >
                            <Folder className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                            <span className="min-w-0 truncate">{f.name}</span>
                          </button>
                        ))}
                    </div>
                  </div>
                ) : (
                  <>
                {parentId && parentId !== projectFolderId ? (
                  <button
                    type="button"
                    className="mb-2 inline-flex items-center gap-1 rounded-sm border border-border px-2 py-1 text-xs hover:bg-accent"
                    onClick={() => openFolder(folderQ.data?.parent_id ?? null)}
                  >
                    <ArrowLeft className="h-3 w-3" />
                    Up
                  </button>
                ) : null}
                {projectFolderId ? (
                  <button
                    type="button"
                    className="mb-2 ml-2 inline-flex items-center gap-1 rounded-sm border border-border px-2 py-1 text-xs hover:bg-accent"
                    onClick={() => {
                      setSp((prev) => {
                        const next = new URLSearchParams(prev);
                        next.delete("projectFolderId");
                        next.delete("parentId");
                        next.delete("fileId");
                        return next;
                      });
                      void navigate(`/o/${orgId}/p/${projectId}/ide`);
                    }}
                  >
                    Change folder
                  </button>
                ) : null}
                <ul className="space-y-1">
                  {[...folders, ...files].map((n) => (
                    <li key={`ide-node-${n.id}`}>
                      <button
                        type="button"
                        className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent/40"
                        onDoubleClick={() => {
                          if (n.kind === "folder") {
                            openFolder(n.id);
                            return;
                          }
                          openFileForEditing(n.id);
                        }}
                        onClick={() => {
                          if (n.kind === "file") openFileForEditing(n.id);
                        }}
                      >
                        {n.kind === "folder" ? (
                          <Folder className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                        ) : (
                          <File className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                        )}
                        <span className="min-w-0 truncate">{n.name}</span>
                      </button>
                    </li>
                  ))}
                </ul>
                  </>
                )}
              </div>
            </aside>
          ) : null}
          <div className="flex min-h-0 min-w-0 flex-1">
            <main
              className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden p-4"
              onKeyDown={onEditorKeyDown}
            >
            {fileId ? (
                <div className="flex min-h-0 flex-1 flex-col">
                  {isIDE ? (
                    <div className="mb-2 flex items-center gap-1 overflow-x-auto rounded-sm border border-border bg-card/60 p-1">
                      {ideHtmlPreviewEligible && !idePreviewTabDismissed ? (
                        <div
                          className={[
                            "flex min-w-0 max-w-[240px] shrink-0 items-center gap-2 rounded-sm border px-2 py-1 text-xs",
                            ideMainView === "preview"
                              ? "border-border bg-background text-foreground"
                              : "border-transparent bg-transparent text-muted-foreground hover:bg-accent/30",
                          ].join(" ")}
                        >
                          <button
                            type="button"
                            className="min-w-0 flex-1 truncate text-left font-medium"
                            onClick={() => openIdePreview()}
                            title="HTML preview"
                          >
                            Preview
                          </button>
                          <button
                            type="button"
                            className="rounded-sm px-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                            aria-label="Close preview"
                            title="Close preview"
                            onClick={(e) => {
                              e.stopPropagation();
                              setIdeMainView("editor");
                              setIdePreviewTabDismissed(true);
                            }}
                          >
                            ×
                          </button>
                        </div>
                      ) : null}
                      {ideTabs.map((t) => {
                        const active = t.id === fileId && ideMainView === "editor";
                        return (
                          <div
                            key={`ide-tab-${t.id}`}
                            className={[
                              "flex min-w-0 max-w-[260px] items-center gap-2 rounded-sm border px-2 py-1 text-xs",
                              active
                                ? "border-border bg-background text-foreground"
                                : "border-transparent bg-transparent text-muted-foreground hover:bg-accent/30",
                            ].join(" ")}
                          >
                            <button
                              type="button"
                              className="min-w-0 flex-1 truncate text-left"
                              onClick={() => openIDEFileTab(t.id)}
                              title={t.name}
                            >
                              {t.name}
                            </button>
                            <button
                              type="button"
                              className="rounded-sm px-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                              aria-label={`Close ${t.name}`}
                              onClick={(e) => {
                                e.stopPropagation();
                                closeIDEFileTab(t.id);
                              }}
                            >
                              ×
                            </button>
                          </div>
                        );
                      })}
                    </div>
                  ) : null}
                {isIDE && ideMainView === "preview" && ideHtmlPreviewEligible ? (
                  <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-sm border border-border bg-card">
                    <div className="shrink-0 border-b border-border px-3 py-2 text-xs text-muted-foreground">
                      <span className="font-medium text-foreground">{activeFileNameForKind}</span>
                      <span className="ml-2">· Preview</span>
                    </div>
                    <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border bg-muted/20 px-2 py-1.5 text-xs">
                      <span className="text-muted-foreground">Preview source</span>
                      <button
                        type="button"
                        className={[
                          "rounded-sm px-2 py-0.5",
                          idePreviewMode === "blob"
                            ? "bg-accent text-foreground"
                            : "text-muted-foreground hover:bg-accent/50",
                        ].join(" ")}
                        onClick={() => switchIdePreviewMode("blob")}
                      >
                        Editor (instant)
                      </button>
                      <button
                        type="button"
                        className={[
                          "rounded-sm px-2 py-0.5",
                          idePreviewMode === "server"
                            ? "bg-accent text-foreground"
                            : "text-muted-foreground hover:bg-accent/50",
                        ].join(" ")}
                        onClick={() => switchIdePreviewMode("server")}
                      >
                        Space snapshot (API)
                      </button>
                    </div>
                    <IdeHtmlPreview
                      html={contentDraft}
                      visible
                      fillHeight
                      className="min-h-0 flex-1 border-0"
                      mode={idePreviewMode === "server" ? "server" : "blob"}
                      serverUrl={ideServerPreviewUrl}
                      serverLoading={ideServerLoading}
                      serverError={ideServerError}
                    />
                  </div>
                ) : (
                <>
                <div>
                  <input
                    className={[
                      "w-full rounded-sm border border-input bg-background px-3 py-3 text-2xl font-semibold outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2",
                      fileNameIsPlaceholder ? "text-muted-foreground" : "text-foreground",
                    ].join(" ")}
                    value={fileNameDraft}
                    onChange={(e) => {
                      setFileNameDraft(e.target.value);
                      setFileNameTouched(true);
                    }}
                    onBlur={() => {
                      const next = fileNameDraft.trim();
                      if (!next) {
                        setFileNameDraft(defaultNewFileName);
                        setFileNameTouched(false);
                        return;
                      }
                      if (fileTextQ.data?.node?.name && next === fileTextQ.data.node.name) {
                        return;
                      }
                      renameActiveFile.mutate(next);
                    }}
                    spellCheck={false}
                  />
                </div>

                {Object.keys(collab.peers).length > 0 ? (
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    <span className="font-medium text-foreground">Viewing:</span>
                    {Object.values(collab.peers).map((p) => (
                      <span
                        key={p.userId}
                        className="rounded-full border border-border bg-muted/30 px-2 py-0.5 text-foreground"
                        title={p.userId}
                      >
                        {p.name}
                      </span>
                    ))}
                  </div>
                ) : null}

                {enableYjsCollab ? (
                  <p className="mt-1 text-xs text-amber-600/90">
                    Yjs collaboration is on (VITE_YJS_COLLAB). Saves still go to the server; refresh picks up
                    last saved content.
                    {!isIDE && !filesPageUseCodeEditor
                      ? " Switch to Code editor for shared cursors in this tab."
                      : null}
                  </p>
                ) : null}

                {pendingProposals.length > 0 ? (
                  <div
                    role="status"
                    className="mt-2 rounded-sm border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm text-foreground"
                  >
                    {pendingProposals.length} pending AI edit
                    {pendingProposals.length === 1 ? "" : "s"} — review below. Saving your edits may
                    invalidate proposals if the file changes.
                  </div>
                ) : null}

                {pendingProposals.length > 0 ? (
                  <div className="mt-4 space-y-3 rounded-sm border border-border bg-card p-3">
                    <div className="text-sm font-semibold text-foreground">Review AI proposals</div>
                    {pendingProposals.map((p) => (
                      <div key={p.id} className="rounded-sm border border-border p-2">
                        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                          <span>Proposal {p.id.slice(0, 8)}…</span>
                          <button
                            type="button"
                            className="text-link underline"
                            onClick={() =>
                              setReviewProposal((cur) => (cur?.id === p.id ? null : p))
                            }
                          >
                            {reviewProposal?.id === p.id ? "Hide diff" : "Show diff"}
                          </button>
                        </div>
                        {reviewProposal?.id === p.id ? (
                          <div className="mt-2 overflow-hidden rounded-sm border border-border">
                            <MonacoDiffViewer
                              instanceKey={`${fileId ?? "file"}-proposal-${p.id}`}
                              height="280px"
                              language="plaintext"
                              theme="vs-dark"
                              original={p.base_content != null ? p.base_content : (fileTextQ.data?.content ?? "")}
                              modified={p.proposed_content}
                              options={{ readOnly: true, renderSideBySide: true }}
                            />
                          </div>
                        ) : null}
                        <div className="mt-2 flex flex-wrap gap-2">
                          <button
                            type="button"
                            className="rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                            disabled={acceptProposal.isPending}
                            onClick={() => acceptProposal.mutate(p.id)}
                          >
                            Accept
                          </button>
                          <button
                            type="button"
                            className="rounded-sm border border-border px-3 py-1.5 text-sm hover:bg-accent"
                            disabled={rejectProposal.isPending}
                            onClick={() => rejectProposal.mutate(p.id)}
                          >
                            Reject
                          </button>
                        </div>
                        {acceptProposal.isError ? (
                          <p className="mt-2 text-xs text-red-600">
                            {(acceptProposal.error as Error)?.message ?? "Could not accept"}
                          </p>
                        ) : null}
                      </div>
                    ))}
                  </div>
                ) : null}

                {filesPageShowCodeEditorToggle ? (
                  <div className="mt-4 flex flex-wrap items-center gap-2">
                    <button
                      type="button"
                      className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-1.5 text-sm text-foreground hover:bg-accent"
                      onClick={() => setFilesPageUseCodeEditor((v) => !v)}
                    >
                      <Code2 className="h-4 w-4 opacity-80" aria-hidden />
                      {filesPageUseCodeEditor ? "Simple text view" : "Code editor"}
                    </button>
                    <span className="text-xs text-muted-foreground">
                      {filesPageUseCodeEditor
                        ? "Syntax highlighting and IDE-style editing."
                        : usePlainTextarea
                          ? "Plain text for CSV, TSV, logs, JSON, and other structured files."
                          : "Rich document for notes and prose (headings, lists, tables, images)."}
                    </span>
                  </div>
                ) : null}

                <div
                  className={[
                    "flex min-h-0 flex-1 flex-col overflow-hidden",
                    "min-h-[14rem] rounded-sm border border-border bg-card",
                    filesPageShowCodeEditorToggle ? "mt-2" : "mt-4",
                  ].join(" ")}
                >
                  <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
                  {useMonacoForBody ? (
                    <Editor
                      key={
                        enableYjsCollab
                          ? `${fileId}-yjs-${fileTextQ.dataUpdatedAt}-${isIDE ? "ide" : filesPageUseCodeEditor ? "code" : "simple"}`
                          : `${fileId ?? "none"}-${isIDE ? "ide" : filesPageUseCodeEditor ? "code" : "simple"}`
                      }
                      height="100%"
                      defaultLanguage={monacoLanguageFromFileName(activeFileNameForKind)}
                      theme="vs-dark"
                      {...(enableYjsCollab
                        ? { defaultValue: fileTextQ.data?.content ?? "" }
                        : { value: contentDraft })}
                      onChange={(v) => {
                        setContentDraft(v ?? "");
                        setSaveState("idle");
                        scheduleSave(v ?? "");
                      }}
                      onMount={(ed, m) => setMonacoCtx({ editor: ed, monaco: m })}
                      options={{
                        automaticLayout: true,
                        minimap: { enabled: false },
                        wordWrap: "on",
                        fontSize: 14,
                      }}
                      loading={<div className="p-3 text-sm text-muted-foreground">Loading editor…</div>}
                    />
                  ) : usePlainTextarea ? (
                    <textarea
                      className="min-h-0 flex-1 resize-none border-0 bg-transparent p-3 font-mono text-sm leading-relaxed text-foreground outline-none ring-0 placeholder:text-muted-foreground focus:ring-0"
                      value={contentDraft}
                      onChange={(e) => {
                        const v = e.target.value;
                        setContentDraft(v);
                        setSaveState("idle");
                        scheduleSave(v);
                      }}
                      spellCheck={false}
                      aria-label="File contents"
                    />
                  ) : orgId && projectId && fileId ? (
                    <FilesRichEditor
                      key={fileId}
                      fileId={fileId}
                      value={contentDraft}
                      onChange={(html) => {
                        setContentDraft(html);
                        setSaveState("idle");
                        scheduleSave(html);
                      }}
                      orgId={orgId}
                      spaceId={projectId}
                      onUploadImage={uploadImageForRichEditor}
                      className="min-h-0 flex-1"
                    />
                  ) : (
                    <textarea
                      className="min-h-0 flex-1 resize-none border-0 bg-transparent p-3 font-mono text-sm leading-relaxed text-foreground outline-none ring-0 placeholder:text-muted-foreground focus:ring-0"
                      value={contentDraft}
                      onChange={(e) => {
                        const v = e.target.value;
                        setContentDraft(v);
                        setSaveState("idle");
                        scheduleSave(v);
                      }}
                      spellCheck={false}
                      aria-label="File contents"
                    />
                  )}
                  </div>
                </div>

                <div className="mt-2 text-xs text-muted-foreground">
                  Tip: Ctrl/Cmd+S to save immediately.
                </div>
                <div className="mt-2 flex items-center justify-between border-t border-border bg-card/60 px-2 py-1 text-[11px] text-muted-foreground">
                  <span>{fileTextQ.data?.node?.mime_type ?? "text/plain"}</span>
                  <span>
                    {saveState === "saving"
                      ? "Saving…"
                      : saveState === "saved"
                        ? "Saved"
                        : saveState === "error"
                          ? "Save failed"
                          : "Ready"}
                  </span>
                </div>
                </>
                )}
                </div>
            ) : (
              <div className="min-h-0 flex-1 overflow-y-auto">
            {isIDE ? (
              <div className="rounded-sm border border-border bg-card p-4">
                {!projectFolderId ? (
                  <>
                    <div className="text-sm font-medium text-foreground">Open project folder</div>
                    <p className="mt-1 text-sm text-muted-foreground">
                      Choose a project folder in Explorer before opening files.
                    </p>
                  </>
                ) : (
                  <>
                    <div className="text-sm font-medium text-foreground">No file open</div>
                    <p className="mt-1 text-sm text-muted-foreground">
                      Open a file from Explorer on the left to start coding.
                    </p>
                  </>
                )}
              </div>
            ) : (
              <>
                {newFolderOpen ? (
                  <div className="mb-4 rounded-sm border border-border bg-card p-3">
                    <div className="flex items-center gap-2">
                      <input
                        className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2"
                        placeholder="Folder name"
                        value={newFolderName}
                        onChange={(e) => setNewFolderName(e.target.value)}
                        autoFocus
                      />
                      <button
                        type="button"
                        className="rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                        onClick={() => createFolder.mutate()}
                        disabled={!newFolderName.trim() || createFolder.isPending}
                      >
                        Create
                      </button>
                      <button
                        type="button"
                        className="rounded-sm border border-border px-3 py-2 text-sm hover:bg-accent"
                        onClick={() => {
                          setNewFolderOpen(false);
                          setNewFolderName("");
                        }}
                      >
                        Cancel
                      </button>
                    </div>
                  </div>
                ) : null}

                <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
                  {[...folders, ...files].map((n) => {
                    const icon =
                      n.kind === "folder" ? (
                        <Folder className="h-6 w-6 text-muted-foreground" />
                      ) : (
                        <File className="h-6 w-6 text-muted-foreground" />
                      );
                    const type =
                      n.kind === "folder"
                        ? "Folder"
                        : (n.mime_type || "text/plain").replace("application/", "");
                    const isSelected = selected?.id === n.id;
                    return (
                      <div
                        key={n.id}
                        className={[
                          "group relative cursor-pointer rounded-sm border border-border bg-card p-3",
                          "hover:bg-accent/20",
                          isSelected ? "ring-2 ring-ring" : "",
                        ].join(" ")}
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
                          moveNode.mutate({ nodeId: dragged.id, parentId: n.id });
                        }}
                        onClick={() => setSelected(n)}
                        onDoubleClick={() => {
                          if (n.kind === "folder") {
                            setSelected(null);
                            setSp((prev) => {
                              const next = new URLSearchParams(prev);
                              next.set("parentId", n.id);
                              return next;
                            });
                            void navigate(`${pageBase}?parentId=${n.id}`);
                            return;
                          }
                          if (n.kind === "file") {
                            setSelected(null);
                            setSp((prev) => {
                              const next = new URLSearchParams(prev);
                              next.set("fileId", n.id);
                              return next;
                            });
                            const fqs = new URLSearchParams();
                            if (parentId) fqs.set("parentId", parentId);
                            fqs.set("fileId", n.id);
                            void navigate(`${pageBase}?${fqs.toString()}`);
                          }
                        }}
                      >
                        <div className="flex items-start justify-between gap-2">
                          <div className="flex min-w-0 items-center gap-2">
                            <div className="flex h-10 w-10 items-center justify-center rounded-sm bg-background">
                              {icon}
                            </div>
                            <div className="min-w-0">
                              <div className="truncate text-sm font-medium text-foreground">
                                {n.name}
                              </div>
                              <div className="mt-0.5 truncate text-xs text-muted-foreground">
                                {type}
                              </div>
                            </div>
                          </div>
                        </div>

                        <div className="mt-3 text-[11px] text-muted-foreground">
                          {new Date(n.updated_at).toLocaleString()}
                        </div>

                        <div className="mt-2 hidden flex-wrap gap-1 group-hover:flex">
                          {n.kind === "file" ? (
                            <button
                              type="button"
                              className="rounded-sm border border-border bg-background px-2 py-1 text-xs hover:bg-accent"
                              onClick={async (e) => {
                                e.stopPropagation();
                                const res = await apiFetch(
                                  `/api/v1/organizations/${orgId}/spaces/${projectId}/files/${n.id}/download`,
                                );
                                if (!res.ok) return;
                                const j = (await res.json()) as { download_url: string };
                                window.open(j.download_url, "_blank", "noreferrer");
                              }}
                            >
                              Download
                            </button>
                          ) : null}
                          <button
                            type="button"
                            className="rounded-sm border border-border bg-background px-2 py-1 text-xs hover:bg-accent"
                            onClick={(e) => {
                              e.stopPropagation();
                              const next = window.prompt("Rename to…", n.name);
                              if (next && next.trim()) {
                                renameNode.mutate({ nodeId: n.id, name: next.trim() });
                              }
                            }}
                          >
                            Rename
                          </button>
                          <button
                            type="button"
                            className="rounded-sm border border-border bg-background px-2 py-1 text-xs hover:bg-accent"
                            onClick={(e) => {
                              e.stopPropagation();
                              setMoveOpen(n);
                              setMoveDest("root");
                            }}
                          >
                            Move
                          </button>
                          <button
                            type="button"
                            className="rounded-sm border border-border bg-background px-2 py-1 text-xs hover:bg-accent"
                            onClick={(e) => {
                              e.stopPropagation();
                              if (window.confirm("Delete this item?")) {
                                deleteNode.mutate(n.id);
                              }
                            }}
                          >
                            Delete
                          </button>
                        </div>
                      </div>
                    );
                  })}
                </div>

                {nodes.length === 0 && !nodesQ.isPending ? (
                  <div className="rounded-sm border border-dashed border-border bg-card px-3 py-10 text-center text-sm text-muted-foreground">
                    No files yet. Upload something or create a folder.
                  </div>
                ) : null}
              </>
            )}
              </div>
            )}
            </main>
            {isIDE && ideAIPanelOpen ? (
              <aside className="hidden w-96 shrink-0 border-l border-border bg-card/40 xl:flex xl:flex-col">
                <div className="flex flex-col gap-2 border-b border-border px-3 py-2">
                  <div className="flex items-center justify-between">
                    <div className="inline-flex items-center gap-2 text-sm font-semibold text-foreground">
                      <Bot className="h-4 w-4 text-muted-foreground" />
                      AI Assistant
                    </div>
                    <div className="flex items-center gap-1">
                      <button
                        type="button"
                        className="rounded-sm p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                        title="Collapse AI Assistant"
                        aria-label="Collapse AI Assistant"
                        onClick={() => setIdeAIPanelOpen(false)}
                      >
                        <ChevronRight className="h-4 w-4" />
                      </button>
                      <Sparkles className="h-4 w-4 text-muted-foreground" aria-hidden />
                    </div>
                  </div>
                  <div
                    className="flex rounded-sm border border-border bg-muted/20 p-0.5 text-[11px] font-medium"
                    role="tablist"
                    aria-label="AI mode"
                  >
                    {(
                      [
                        { id: "ask" as const, label: "Ask" },
                        { id: "plan" as const, label: "Plan" },
                        { id: "agent" as const, label: "Agent" },
                      ] as const
                    ).map(({ id, label }) => (
                      <button
                        key={id}
                        type="button"
                        role="tab"
                        aria-selected={ideChatMode === id}
                        className={[
                          "min-w-0 flex-1 rounded-sm px-2 py-1.5 transition-colors",
                          ideChatMode === id
                            ? "bg-background text-foreground shadow-sm"
                            : "text-muted-foreground hover:text-foreground",
                        ].join(" ")}
                        disabled={idePanelDisabled}
                        onClick={() => setIdeChatMode(id)}
                      >
                        {label}
                      </button>
                    ))}
                  </div>
                </div>
                <div className="min-h-0 flex-1 space-y-2 overflow-y-auto p-3">
                  {ideAIMessages.length === 0 ? (
                    <div className="rounded-sm border border-border bg-card p-3 text-xs text-muted-foreground">
                      One chat composer uses your prompt and IDE context. <strong>Ask</strong> and{" "}
                      <strong>Plan</strong> are read-only; <strong>Agent</strong> can create proposals and edits.
                    </div>
                  ) : null}
                  {ideAIMessages.map((m) => {
                    const card = m.card;
                    return (
                      <div
                        key={m.id}
                        className={[
                          "rounded-sm border p-2 text-sm",
                          m.role === "user"
                            ? "border-blue-500/30 bg-blue-500/10 text-foreground"
                            : m.variant === "plan"
                              ? "border-violet-500/30 bg-violet-500/10 text-foreground"
                              : "border-border bg-card text-foreground",
                        ].join(" ")}
                      >
                      <div className="mb-1 text-[11px] uppercase tracking-[0.1em] text-muted-foreground">
                        {m.role === "user" ? "You" : "Assistant"}
                      </div>
                      <pre className="whitespace-pre-wrap break-words font-sans">{m.text}</pre>
                      {card?.kind === "proposal" ? (
                        <div className="mt-2">
                          <button
                            type="button"
                            className="rounded-sm border border-border px-2 py-1 text-xs hover:bg-accent"
                            onClick={() => {
                              const proposal = pendingProposals.find((p) => p.id === card.proposalId);
                              if (proposal) setReviewProposal(proposal);
                            }}
                          >
                            Open proposal review
                          </button>
                        </div>
                      ) : null}
                      {card?.kind === "direct-apply" && ideChatMode === "agent" && !me?.service_account ? (
                        <div className="mt-2 rounded-sm border border-amber-500/40 bg-amber-500/10 p-2">
                          <div className="text-xs text-foreground">
                            Human-only direct apply is available after a confirm step.
                          </div>
                          <button
                            type="button"
                            className="mt-2 rounded-sm border border-border px-2 py-1 text-xs hover:bg-accent"
                            onClick={() => setPendingDirectApply({ prompt: card.prompt, content: card.content })}
                          >
                            Review diff before apply
                          </button>
                        </div>
                      ) : null}
                      </div>
                    );
                  })}
                </div>
                <div className="border-t border-border p-3">
                  {invokeFailure ? (
                    <div className="mb-2 rounded-sm border border-rose-500/40 bg-rose-500/10 p-2 text-xs text-foreground">
                      <div>{invokeFailure.message}</div>
                      {invokeFailure.retryable ? (
                        <button
                          type="button"
                          className="mt-2 rounded-sm border border-border px-2 py-1 text-[11px] hover:bg-accent"
                          onClick={() => void onSendToAIPanel(invokeFailure.prompt)}
                          disabled={ideAISending}
                        >
                          Retry last prompt
                        </button>
                      ) : null}
                    </div>
                  ) : null}
                  <div className="mb-2 flex items-center justify-end gap-2">
                    <button
                      type="button"
                      className="rounded-sm border border-border px-2 py-1.5 text-xs hover:bg-accent"
                      onClick={() => setIdeAIMessages([])}
                    >
                      Clear
                    </button>
                  </div>
                  {ideChatMode === "agent" && !me?.service_account ? (
                    <label className="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
                      <input
                        type="checkbox"
                        checked={enableDirectApply}
                        onChange={(e) => setEnableDirectApply(e.target.checked)}
                      />
                      Show direct-apply option (human only)
                    </label>
                  ) : null}
                  <textarea
                    className="h-24 w-full rounded-sm border border-input bg-background px-2 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2 disabled:opacity-60"
                    placeholder={
                      idePanelDisabled
                        ? "Pick a project folder first to enable AI panel."
                        : ideChatMode === "ask"
                          ? "Ask questions; we read files or list folders from IDE context."
                          : ideChatMode === "plan"
                            ? "Explore and plan; read-only tools only."
                            : "Agent can list, read, and create edit proposals from the editor."
                    }
                    value={ideAIPrompt}
                    onChange={(e) => setIdeAIPrompt(e.target.value)}
                    disabled={idePanelDisabled || ideAISending}
                  />
                  <div className="mt-2 flex items-center justify-between">
                    <div className="text-[11px] text-muted-foreground">
                      {idePanelDisabled
                        ? "Folder-first guardrail is active."
                        : `${ideChatMode.toUpperCase()} · ${
                            fileId ? `file: ${fileNameDraft}` : "no file open"
                          }`}
                    </div>
                    <button
                      type="button"
                      className="inline-flex items-center gap-2 rounded-sm bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                      onClick={() => void onSendToAIPanel()}
                      disabled={idePanelDisabled || ideAISending || !ideAIPrompt.trim()}
                    >
                      <Send className="h-3.5 w-3.5" />
                      {ideAISending ? "Sending…" : "Send"}
                    </button>
                  </div>
                </div>
              </aside>
            ) : null}
          </div>
        </div>
      </div>

      {pendingDirectApply && !me?.service_account ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-5xl rounded-sm border border-border bg-card p-4 shadow-lg">
            <div className="text-sm font-semibold text-foreground">Confirm direct apply</div>
            <p className="mt-1 text-xs text-muted-foreground">
              You are applying changes directly as a human user. This bypasses proposal review.
            </p>
            <div className="mt-3 overflow-hidden rounded-sm border border-border">
              <MonacoDiffViewer
                instanceKey={`${fileId ?? "file"}-direct-apply`}
                height="420px"
                language="plaintext"
                theme="vs-dark"
                original={fileTextQ.data?.content ?? ""}
                modified={pendingDirectApply.content}
                options={{ readOnly: true, renderSideBySide: true }}
              />
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <button
                type="button"
                className="rounded-sm border border-border px-3 py-2 text-sm hover:bg-accent"
                onClick={() => setPendingDirectApply(null)}
              >
                Cancel
              </button>
              <button
                type="button"
                className="rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                disabled={saveActiveFile.isPending}
                onClick={() => {
                  saveActiveFile.mutate(persistBodyContent(pendingDirectApply.content));
                  setContentDraft(pendingDirectApply.content);
                  setIdeAIMessages((prev) => [
                    ...prev,
                    {
                      id: `${Date.now()}-direct-applied`,
                      role: "assistant",
                      text: "Direct apply submitted.",
                    },
                  ]);
                  setPendingDirectApply(null);
                }}
              >
                Confirm and apply
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {moveOpen ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-md rounded-sm border border-border bg-card p-4 shadow-lg">
            <div className="text-sm font-semibold text-foreground">Move “{moveOpen.name}”</div>
            <div className="mt-3">
              <label className="text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
                Destination
              </label>
              <select
                className="mt-2 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                value={moveDest}
                onChange={(e) => setMoveDest(e.target.value as UUID | "root")}
              >
                <option value="root">Root</option>
                {(foldersInSpaceQ.data ?? []).map((f) => (
                  <option key={f.id} value={f.id}>
                    {f.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <button
                type="button"
                className="rounded-sm border border-border px-3 py-2 text-sm hover:bg-accent"
                onClick={() => setMoveOpen(null)}
              >
                Cancel
              </button>
              <button
                type="button"
                className="rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                onClick={() => {
                  const dest = moveDest === "root" ? null : (moveDest as UUID);
                  moveNode.mutate({ nodeId: moveOpen.id, parentId: dest });
                  setMoveOpen(null);
                }}
                disabled={moveNode.isPending}
              >
                Move
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
