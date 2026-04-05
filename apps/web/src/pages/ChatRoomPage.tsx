import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Paperclip, Search, SmilePlus, Trash2 } from "lucide-react";
import {
  FormEvent,
  Fragment,
  KeyboardEvent,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useParams } from "react-router-dom";
import { apiFetch } from "../api/http";
import type {
  ChatMessage,
  ChatMessageAgentRunMeta,
  ChatMessageAttachment,
  ChatMessageReaction,
  FileNode,
  OrgMemberWithUser,
  Project,
  RoleWithPermissions,
} from "../api/types";
import { ChatFileProposalCard } from "../components/ChatFileProposalCard";
import { useAuth } from "../auth/AuthContext";
import { usePresencePing } from "../hooks/usePresencePing";
import { useOrgRealtime } from "../hooks/useOrgRealtime";
import { chatDebug, warnIfNoUserMentionTokens } from "../chatDebug";

type MentionCandidate =
  | { kind: "user"; id: string; label: string; meta?: string }
  | { kind: "role"; id: string; label: string; meta?: string }
  | { kind: "file"; id: string; label: string; meta?: string };

/** Build `folder/name` paths from a flat tree response (e.g. /files/tree). */
function filePathLabelsById(nodes: FileNode[]): Map<string, string> {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const pathFor = (id: string): string => {
    const parts: string[] = [];
    let cur: FileNode | undefined = byId.get(id);
    const seen = new Set<string>();
    while (cur && !seen.has(cur.id)) {
      seen.add(cur.id);
      parts.unshift(cur.name);
      if (!cur.parent_id) break;
      cur = byId.get(cur.parent_id);
    }
    return parts.join("/");
  };
  const m = new Map<string, string>();
  for (const n of nodes) {
    if (n.kind === "file" && !n.deleted_at) {
      m.set(n.id, pathFor(n.id));
    }
  }
  return m;
}

function displayNameForMember(m: OrgMemberWithUser) {
  return m.display_name?.trim() || m.email.split("@")[0] || m.email;
}

function shortRepoUrlForSidebar(url: string, max = 36): string {
  const u = url.trim();
  if (u.length <= max) return u;
  return `${u.slice(0, max - 1)}…`;
}

/** Second line under name for AI staff: always "AI agent …" plus model or Cursor context. */
function aiStaffModelSubtitle(m: OrgMemberWithUser): string | null {
  if (!m.is_service_account) return null;

  const model = m.openrouter_model?.trim();
  const repo = m.cursor_default_repo_url?.trim();
  const pRaw = m.service_account_provider;
  const p = pRaw === "openrouter" || pRaw === "cursor" ? pRaw : null;

  if (p === "cursor") {
    return repo
      ? `AI agent Cursor · ${shortRepoUrlForSidebar(repo)}`
      : "AI agent Cursor Cloud Agents";
  }
  // OpenRouter, or provider omitted / empty but we still have a model id from the API
  if (p === "openrouter" || model) {
    return `AI agent ${model || "OpenRouter"}`;
  }
  if (repo) {
    return `AI agent Cursor · ${shortRepoUrlForSidebar(repo)}`;
  }
  return "AI agent";
}

function displayNameForRole(r: RoleWithPermissions) {
  return r.name?.trim() || "Role";
}

function findMentionQuery(text: string, caret: number): { start: number; query: string } | null {
  const before = text.slice(0, caret);
  const at = before.lastIndexOf("@");
  if (at < 0) return null;
  // Trigger only at word boundary: start OR preceded by whitespace/punctuation.
  if (at > 0 && /[A-Za-z0-9_]/.test(before[at - 1]!)) return null;
  // If there's whitespace between @ and caret, we don't consider it an active mention.
  const afterAt = before.slice(at + 1);
  // Support empty query (just \"@\").
  if (afterAt.length === 0) return { start: at, query: "" };
  // Close on whitespace.
  if (/\\s/.test(afterAt)) return null;
  // Close on punctuation that typically ends a mention query (allow `/` for file paths).
  if (/[^A-Za-z0-9_.\-/]/.test(afterAt)) return null;
  return { start: at, query: afterAt };
}

function insertMentionToken(args: {
  text: string;
  caret: number;
  mentionStart: number;
  candidate: MentionCandidate;
}): { nextText: string; nextCaret: number } {
  const { text, caret, mentionStart, candidate } = args;
  const token =
    candidate.kind === "user"
      ? `<@${candidate.id}>`
      : candidate.kind === "role"
        ? `<@&${candidate.id}>`
        : `<#${candidate.id}>`;
  const next = text.slice(0, mentionStart) + token + " " + text.slice(caret);
  const nextCaret = mentionStart + token.length + 1;
  return { nextText: next, nextCaret };
}

/**
 * Models often emit bullet lists on one line (`here: - first - second`).
 * Turn those into real newlines for display (conservative patterns to avoid `Jan. - Feb` style breaks).
 */
function normalizeModelBulletsToNewlines(text: string): string {
  return text
    .replace(/:\s+-\s+/g, ":\n- ")
    .replace(/\u201d\s+-\s+/g, "\u201d\n- ")
    .replace(/(?<=[a-z]{3,})\.\s+-\s+/g, ".\n- ");
}

/** Renders `**bold**` as <strong>; leaves other text unchanged (no raw HTML). */
function renderInlineMarkdownFragments(text: string, keyPrefix: string): ReactNode[] {
  const boldRe = /\*\*([^*]+)\*\*/g;
  const parts: ReactNode[] = [];
  let last = 0;
  let n = 0;
  let m: RegExpExecArray | null;
  while ((m = boldRe.exec(text)) !== null) {
    if (m.index > last) {
      parts.push(
        <span key={`${keyPrefix}-t-${n++}`}>{text.slice(last, m.index)}</span>,
      );
    }
    parts.push(
      <strong key={`${keyPrefix}-b-${n++}`} className="font-semibold">
        {m[1]}
      </strong>,
    );
    last = m.index + m[0].length;
  }
  if (last < text.length) {
    parts.push(<span key={`${keyPrefix}-t-${n++}`}>{text.slice(last)}</span>);
  }
  return parts.length > 0 ? parts : [<span key={`${keyPrefix}-t`}>{text}</span>];
}

function renderMentions(args: {
  content: string;
  userLabelById: Map<string, string>;
  roleLabelById: Map<string, string>;
  fileLabelById: Map<string, string>;
}): ReactNode {
  const { userLabelById, roleLabelById, fileLabelById } = args;
  const content = normalizeModelBulletsToNewlines(args.content);
  const re = /<#([0-9a-fA-F-]{36})>|<@&([0-9a-fA-F-]{36})>|<@([0-9a-fA-F-]{36})>/g;
  const out: ReactNode[] = [];
  let last = 0;
  let mdKey = 0;
  for (;;) {
    const m = re.exec(content);
    if (!m) break;
    if (m.index > last) {
      const seg = content.slice(last, m.index);
      if (seg) out.push(...renderInlineMarkdownFragments(seg, `md-${mdKey++}`));
    }
    const fileId = m[1];
    const roleId = m[2];
    const userId = m[3];
    if (fileId) {
      const label = fileLabelById.get(fileId) ?? "file";
      out.push(
        <span
          key={`f:${fileId}:${m.index}`}
          className="inline-flex items-center gap-1 rounded-sm border border-border bg-muted/50 px-1.5 py-0.5 font-medium text-foreground"
          title={label}
        >
          <FileText className="h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden />
          <span className="max-w-[14rem] truncate">{label}</span>
        </span>,
      );
    } else if (roleId) {
      const label = roleLabelById.get(roleId) ?? "role";
      out.push(
        <span
          key={`r:${roleId}:${m.index}`}
          className="rounded-sm bg-accent px-1.5 py-0.5 font-medium text-foreground"
          title={`@${label}`}
        >
          @{label}
        </span>,
      );
    } else if (userId) {
      const label = userLabelById.get(userId) ?? "user";
      out.push(
        <span
          key={`u:${userId}:${m.index}`}
          className="rounded-sm bg-accent px-1.5 py-0.5 font-medium text-foreground"
          title={`@${label}`}
        >
          @{label}
        </span>,
      );
    } else {
      out.push(m[0]);
    }
    last = m.index + m[0].length;
  }
  if (last < content.length) {
    const seg = content.slice(last);
    if (seg) out.push(...renderInlineMarkdownFragments(seg, `md-${mdKey++}`));
  }
  if (out.length === 0) {
    return renderInlineMarkdownFragments(content, "md-0");
  }
  return out;
}

/** User mention tokens `<@uuid>` only (not role `<@&…>`). */
function extractUserMentionUserIds(content: string): string[] {
  const re = /<@([0-9a-fA-F-]{36})>/g;
  const ids: string[] = [];
  let m: RegExpExecArray | null;
  while ((m = re.exec(content)) !== null) {
    ids.push(m[1]);
  }
  return ids;
}

function AgentThinkingPlaceholderRow({
  pendingAgentUserIds,
  userLabelById,
  spaceMembers,
}: {
  pendingAgentUserIds: string[];
  userLabelById: Map<string, string>;
  spaceMembers: OrgMemberWithUser[];
}) {
  const label =
    pendingAgentUserIds.length === 1
      ? (() => {
          const id = pendingAgentUserIds[0];
          const mem = spaceMembers.find((x) => x.user_id === id);
          return userLabelById.get(id) ?? (mem ? displayNameForMember(mem) : "AI agent");
        })()
      : `${pendingAgentUserIds.length} agents`;
  return (
    <div
      className="rounded-sm px-2 py-2 hover:bg-accent/20"
      aria-live="polite"
      aria-busy="true"
    >
      <div className="flex items-start gap-3">
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="text-sm font-medium text-foreground">{label}</span>
            <span className="text-xs text-muted-foreground">…</span>
          </div>
          <div className="mt-1 text-sm italic text-muted-foreground animate-pulse">Thinking…</div>
        </div>
      </div>
    </div>
  );
}

function AgentRunCard({ meta }: { meta: ChatMessageAgentRunMeta }) {
  const label = meta.display_name ?? "AI agent run";
  const status = meta.status || "…";
  return (
    <div className="mb-2 rounded-sm border border-border bg-muted/30 px-3 py-2 text-xs text-foreground">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-semibold uppercase tracking-wide text-muted-foreground">{label}</span>
        <span className="rounded-sm bg-accent px-1.5 py-0.5 font-mono text-[11px]">{status}</span>
        {meta.provider === "cursor" ? (
          <span className="text-muted-foreground">Cursor Cloud Agents</span>
        ) : null}
      </div>
      {meta.url ? (
        <a
          className="mt-1 block truncate text-link hover:underline"
          href={meta.url}
          target="_blank"
          rel="noreferrer"
        >
          Open run
        </a>
      ) : null}
      {meta.external_id ? (
        <div className="mt-1 font-mono text-[11px] text-muted-foreground">{meta.external_id}</div>
      ) : null}
    </div>
  );
}

export default function ChatRoomPage() {
  const { state } = useAuth();
  const qc = useQueryClient();
  const { orgId, projectId, chatRoomId } = useParams<{
    orgId: string;
    projectId: string;
    chatRoomId: string;
  }>();

  useOrgRealtime(orgId, !!orgId);
  const { localPresence } = usePresencePing(state.status === "authenticated");

  const [draft, setDraft] = useState("");
  const [attachUrl, setAttachUrl] = useState("");
  const [attachOpen, setAttachOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [toast, setToast] = useState<string | null>(null);
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  /** After @mentioning AI staff, show "Thinking…" until each agent replies (or timeout). */
  const [agentReplyPending, setAgentReplyPending] = useState<{
    userMessageId: string;
    userMessageCreatedAt: string;
    pendingAgentUserIds: string[];
  } | null>(null);
  const agentThinkingTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const composerRef = useRef<HTMLTextAreaElement | null>(null);
  const composerOverlayRef = useRef<HTMLDivElement | null>(null);

  const [mentionOpen, setMentionOpen] = useState(false);
  const [mentionStart, setMentionStart] = useState<number | null>(null);
  const [mentionQuery, setMentionQuery] = useState("");
  const [mentionActiveIdx, setMentionActiveIdx] = useState(0);
  const suppressMentionRef = useRef<{ untilCaret: number; untilText: string } | null>(null);

  const projectQ = useQuery({
    queryKey: ["project", orgId, projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${projectId}`);
      if (!res.ok) {
        throw new Error("project");
      }
      return res.json() as Promise<Project>;
    },
  });

  const spaceMembersQ = useQuery({
    queryKey: ["space-accessible-members", orgId, projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/accessible-members`,
      );
      if (!res.ok) {
        throw new Error("accessible-members");
      }
      return (await res.json()) as {
        members: OrgMemberWithUser[];
        allowed_role_ids: string[];
        space_has_allowlist: boolean;
      };
    },
  });

  const spaceMembers = spaceMembersQ.data?.members ?? [];

  const rolesQ = useQuery({
    queryKey: ["roles", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/roles`);
      if (!res.ok) {
        throw new Error("roles");
      }
      const j = (await res.json()) as { roles: RoleWithPermissions[] };
      return j.roles;
    },
  });

  const rolesForMentions = useMemo(() => {
    const all = rolesQ.data ?? [];
    if (!spaceMembersQ.data?.space_has_allowlist) {
      return all;
    }
    const allow = new Set(spaceMembersQ.data.allowed_role_ids);
    return all.filter((r) => allow.has(r.id));
  }, [rolesQ.data, spaceMembersQ.data]);

  const filesTreeQ = useQuery({
    queryKey: ["chat-files-tree", orgId, projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/tree`,
      );
      if (!res.ok) {
        // No file picker if the user lacks files.read; chat still works.
        return [] as FileNode[];
      }
      const j = (await res.json()) as { nodes: FileNode[] };
      return j.nodes ?? [];
    },
    staleTime: 30_000,
  });

  const messagesQ = useQuery({
    queryKey: ["chat-messages", chatRoomId],
    enabled: !!orgId && !!projectId && !!chatRoomId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/chat-rooms/${chatRoomId}/messages`,
      );
      if (!res.ok) {
        throw new Error("messages");
      }
      const j = (await res.json()) as {
        messages: ChatMessage[];
        reactions: ChatMessageReaction[];
        attachments: ChatMessageAttachment[];
      };
      return j;
    },
  });

  const searchQ = useQuery({
    queryKey: ["chat-search", chatRoomId, search],
    enabled: !!orgId && !!projectId && !!chatRoomId && search.trim().length > 0,
    queryFn: async () => {
      const q = encodeURIComponent(search.trim());
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/chat-rooms/${chatRoomId}/search?q=${q}`,
      );
      if (!res.ok) {
        throw new Error("search");
      }
      const j = (await res.json()) as { messages: ChatMessage[] };
      return j.messages;
    },
  });

  const createMessage = useMutation({
    mutationFn: async () => {
      warnIfNoUserMentionTokens(draft);
      const userMentions = draft.match(/<@[0-9a-fA-F-]{36}>/g)?.length ?? 0;
      chatDebug("sending chat message", {
        chatRoomId,
        contentLength: draft.length,
        userMentionTokens: userMentions,
        fileRefTokens: (draft.match(/<#[0-9a-fA-F-]{36}>/g) ?? []).length,
      });
      const body: {
        content: string;
        attachments: { name: string; mime: string; size_bytes: number; url: string }[];
      } = { content: draft, attachments: [] };
      if (attachUrl.trim()) {
        body.attachments.push({
          name: attachUrl.trim(),
          mime: "",
          size_bytes: 0,
          url: attachUrl.trim(),
        });
      }
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/chat-rooms/${chatRoomId}/messages`,
        { method: "POST", json: body },
      );
      if (!res.ok) {
        const errText = await res.text().catch(() => "");
        throw new Error(errText || `create message HTTP ${res.status}`);
      }
      return res.json() as Promise<{ message: ChatMessage }>;
    },
    onSuccess: (data) => {
      chatDebug("message created", { id: data?.message?.id });
      setDraft("");
      setAttachUrl("");
      const msg = data.message;
      const mentioned = extractUserMentionUserIds(msg.content);
      const agentIds = [
        ...new Set(
          mentioned.filter((id) =>
            spaceMembers.some((mem) => mem.user_id === id && mem.is_service_account),
          ),
        ),
      ];
      if (agentIds.length > 0) {
        setAgentReplyPending({
          userMessageId: msg.id,
          userMessageCreatedAt: msg.created_at,
          pendingAgentUserIds: agentIds,
        });
      }
      void qc.invalidateQueries({ queryKey: ["chat-messages", chatRoomId] });
    },
    onError: (err) => {
      console.error("[Hyperspeed chat] Failed to send message", err);
    },
  });

  const deleteMessage = useMutation({
    mutationFn: async (messageId: string) => {
      if (!orgId?.trim() || !projectId?.trim() || !chatRoomId?.trim()) {
        throw new Error("Missing org, space, or chat room in URL");
      }
      const path = `/api/v1/organizations/${encodeURIComponent(orgId)}/spaces/${encodeURIComponent(projectId)}/chat-rooms/${encodeURIComponent(chatRoomId)}/messages/${encodeURIComponent(messageId)}`;
      const res = await apiFetch(path, { method: "DELETE" });
      if (!res.ok) {
        throw new Error("delete message");
      }
      return true;
    },
    onSuccess: () => {
      setToast("Message deleted");
      void qc.invalidateQueries({ queryKey: ["chat-messages", chatRoomId] });
    },
  });

  const mentionUnreadIdsQ = useQuery({
    queryKey: ["notifications-ids", orgId, projectId, chatRoomId],
    enabled: !!orgId && !!projectId && !!chatRoomId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/me/notifications?org_id=${encodeURIComponent(orgId!)}&type=chat.mention&unread_only=1&space_id=${encodeURIComponent(projectId!)}&chat_room_id=${encodeURIComponent(chatRoomId!)}&limit=200`,
      );
      if (!res.ok) throw new Error("notifications");
      const j = (await res.json()) as { ids?: string[] };
      return j.ids ?? [];
    },
    staleTime: 10_000,
  });

  const markRead = useMutation({
    mutationFn: async (ids: string[]) => {
      if (!orgId) return 0;
      const res = await apiFetch("/api/v1/me/notifications/mark-read", {
        method: "POST",
        json: { org_id: orgId, ids },
      });
      if (!res.ok) throw new Error("mark read");
      const j = (await res.json()) as { updated?: number };
      return j.updated ?? 0;
    },
    onSuccess: async () => {
      if (!orgId) return;
      await qc.invalidateQueries({ queryKey: ["notifications", orgId] });
      await qc.invalidateQueries({ queryKey: ["notifications-unread", orgId] });
      await qc.invalidateQueries({ queryKey: ["notifications-ids", orgId, projectId, chatRoomId] });
    },
  });

  const addReaction = useMutation({
    mutationFn: async (vars: { messageId: string; emoji: string }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/chat-rooms/${chatRoomId}/messages/${vars.messageId}/reactions`,
        { method: "POST", json: { emoji: vars.emoji } },
      );
      if (!res.ok) {
        throw new Error("reaction");
      }
      return res.json() as Promise<ChatMessageReaction>;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["chat-messages", chatRoomId] });
    },
  });

  const messages = messagesQ.data?.messages ?? [];
  const reactions = messagesQ.data?.reactions ?? [];
  const attachments = messagesQ.data?.attachments ?? [];

  const visibleMessages = useMemo(() => {
    // Gold standard: deleted messages should not appear in the list at all.
    return messages.filter((m) => !m.deleted_at);
  }, [messages]);

  useEffect(() => {
    setAgentReplyPending(null);
  }, [chatRoomId]);

  useEffect(() => {
    setAgentReplyPending((prev) => {
      if (!prev) return null;
      const userTs = Date.parse(prev.userMessageCreatedAt);
      const still = prev.pendingAgentUserIds.filter((agentId) => {
        const replied = visibleMessages.some((msg) => {
          if (!msg.author_user_id || msg.author_user_id !== agentId) return false;
          if (msg.id === prev.userMessageId) return false;
          return Date.parse(msg.created_at) > userTs;
        });
        return !replied;
      });
      if (still.length === 0) return null;
      if (still.length === prev.pendingAgentUserIds.length) return prev;
      return { ...prev, pendingAgentUserIds: still };
    });
  }, [visibleMessages]);

  useEffect(() => {
    if (agentThinkingTimeoutRef.current) {
      clearTimeout(agentThinkingTimeoutRef.current);
      agentThinkingTimeoutRef.current = null;
    }
    if (!agentReplyPending) return undefined;
    agentThinkingTimeoutRef.current = window.setTimeout(() => {
      agentThinkingTimeoutRef.current = null;
      setAgentReplyPending(null);
    }, 3 * 60 * 1000);
    return () => {
      if (agentThinkingTimeoutRef.current) {
        clearTimeout(agentThinkingTimeoutRef.current);
        agentThinkingTimeoutRef.current = null;
      }
    };
  }, [agentReplyPending?.userMessageId]);

  useEffect(() => {
    const ids = mentionUnreadIdsQ.data ?? [];
    if (!ids.length) return;
    if (markRead.isPending) return;
    markRead.mutate(ids);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mentionUnreadIdsQ.data, orgId, projectId, chatRoomId]);

  useEffect(() => {
    return () => {
      if (toastTimerRef.current) {
        clearTimeout(toastTimerRef.current);
        toastTimerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    if (!toast) {
      return;
    }
    if (toastTimerRef.current) {
      clearTimeout(toastTimerRef.current);
    }
    toastTimerRef.current = setTimeout(() => {
      toastTimerRef.current = null;
      setToast(null);
    }, 2000);
  }, [toast]);

  const reactionsByMessage = useMemo(() => {
    const m = new Map<string, ChatMessageReaction[]>();
    for (const r of reactions) {
      const arr = m.get(r.message_id) ?? [];
      arr.push(r);
      m.set(r.message_id, arr);
    }
    return m;
  }, [reactions]);

  const attachmentsByMessage = useMemo(() => {
    const m = new Map<string, ChatMessageAttachment[]>();
    for (const a of attachments) {
      const arr = m.get(a.message_id) ?? [];
      arr.push(a);
      m.set(a.message_id, arr);
    }
    return m;
  }, [attachments]);

  const me =
    state.status === "authenticated"
      ? {
          id: state.user.id,
          label:
            state.user.display_name?.trim() ||
            state.user.email?.split("@")[0] ||
            "You",
        }
      : null;

  const fileLabelById = useMemo(
    () => filePathLabelsById(filesTreeQ.data ?? []),
    [filesTreeQ.data],
  );

  const mentionCandidates = useMemo((): MentionCandidate[] => {
    const q = mentionQuery.trim().toLowerCase();
    const users: MentionCandidate[] = spaceMembers.map((m) => {
      const label = displayNameForMember(m);
      return { kind: "user", id: m.user_id, label, meta: m.email };
    });
    const roles: MentionCandidate[] = rolesForMentions.map((r) => {
      const label = displayNameForRole(r);
      return { kind: "role", id: r.id, label, meta: r.is_system ? "system" : "role" };
    });
    const files: MentionCandidate[] = (filesTreeQ.data ?? [])
      .filter((n) => n.kind === "file" && !n.deleted_at)
      .map((n) => ({
        kind: "file" as const,
        id: n.id,
        label: fileLabelById.get(n.id) ?? n.name,
        meta: "file",
      }));
    const all = [...users, ...roles, ...files];
    const limit = 20;
    if (!q) return all.slice(0, limit);
    return all
      .filter((c) => {
        const hay = `${c.label} ${c.meta ?? ""}`.toLowerCase();
        return hay.includes(q);
      })
      .slice(0, limit);
  }, [spaceMembers, rolesForMentions, mentionQuery, filesTreeQ.data, fileLabelById]);

  const userLabelById = useMemo(() => {
    const m = new Map<string, string>();
    for (const mem of spaceMembers) {
      m.set(mem.user_id, displayNameForMember(mem));
    }
    return m;
  }, [spaceMembers]);

  const roleLabelById = useMemo(() => {
    const m = new Map<string, string>();
    for (const r of rolesQ.data ?? []) {
      m.set(r.id, displayNameForRole(r));
    }
    return m;
  }, [rolesQ.data]);

  useEffect(() => {
    if (!mentionOpen) return;
    setMentionActiveIdx((i) => {
      if (mentionCandidates.length === 0) return 0;
      return Math.min(Math.max(i, 0), mentionCandidates.length - 1);
    });
  }, [mentionOpen, mentionCandidates.length]);

  const presenceByUserId = useMemo(() => {
    const now = Date.now();
    const out = new Map<string, "online" | "away" | "offline">();
    for (const m of spaceMembers) {
      const last = Date.parse(m.last_seen_at);
      const ageMs = Number.isFinite(last) ? now - last : Number.POSITIVE_INFINITY;
      if (ageMs <= 30000) {
        out.set(m.user_id, "online");
      } else if (ageMs <= 5 * 60 * 1000) {
        out.set(m.user_id, "away");
      } else {
        out.set(m.user_id, "offline");
      }
    }
    // Self: if idle locally, show away even if last_seen looks online.
    if (me?.id && localPresence === "away") {
      out.set(me.id, "away");
    }
    return out;
  }, [spaceMembers, me?.id, localPresence]);

  function avatarInitialFor(member: { display_name?: string | null; email: string }) {
    const base =
      member.display_name?.trim() ||
      member.email.split("@")[0] ||
      member.email;
    return (base[0] || "?").toUpperCase();
  }

  function formatMinute(ts: string) {
    const d = new Date(ts);
    return d.toLocaleString(undefined, {
      year: "numeric",
      month: "numeric",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
  }

  function authorDisplayName(m: ChatMessage) {
    if (!m.author_user_id) {
      return "Unknown";
    }
    if (m.author_user_id === me?.id) {
      return me?.label ?? "You";
    }
    const fromMap = userLabelById.get(m.author_user_id);
    if (fromMap) {
      return fromMap;
    }
    const mem = spaceMembers.find((x) => x.user_id === m.author_user_id);
    if (mem) {
      return displayNameForMember(mem);
    }
    return "Someone";
  }

  function onSend(e: FormEvent) {
    e.preventDefault();
    if (createMessage.isPending) {
      return;
    }
    if (!draft.trim() && !attachUrl.trim()) {
      return;
    }
    createMessage.mutate();
  }

  function onComposerKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (mentionOpen) {
      if (e.key === "Escape") {
        e.preventDefault();
        setMentionOpen(false);
        setMentionStart(null);
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setMentionActiveIdx((i) =>
          mentionCandidates.length ? (i + 1) % mentionCandidates.length : 0,
        );
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setMentionActiveIdx((i) =>
          mentionCandidates.length ? (i - 1 + mentionCandidates.length) % mentionCandidates.length : 0,
        );
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        const idx = mentionActiveIdx;
        const c = mentionCandidates[idx];
        const start = mentionStart;
        const caret = e.currentTarget.selectionStart ?? draft.length;
        if (c && start !== null) {
          e.preventDefault();
          const { nextText, nextCaret } = insertMentionToken({
            text: draft,
            caret,
            mentionStart: start,
            candidate: c,
          });
          setDraft(nextText);
          setMentionOpen(false);
          setMentionStart(null);
          suppressMentionRef.current = { untilCaret: nextCaret, untilText: nextText };
          // Move caret after render.
          queueMicrotask(() => {
            const el = composerRef.current;
            if (!el) return;
            el.focus();
            el.setSelectionRange(nextCaret, nextCaret);
          });
          return;
        }
      }
    }

    if (e.key !== "Enter") {
      return;
    }
    if (e.shiftKey) {
      return;
    }
    e.preventDefault();
    if (!draft.trim() && !attachUrl.trim()) {
      return;
    }
    if (createMessage.isPending) {
      return;
    }
    createMessage.mutate();
  }

  function onComposerChange(next: string, caret: number) {
    setDraft(next);
    const sup = suppressMentionRef.current;
    if (sup && sup.untilCaret === caret && sup.untilText === next) {
      suppressMentionRef.current = null;
      return;
    }
    const mq = findMentionQuery(next, caret);
    if (!mq) {
      setMentionOpen(false);
      setMentionStart(null);
      setMentionQuery("");
      return;
    }
    setMentionOpen(true);
    setMentionStart(mq.start);
    setMentionQuery(mq.query);
  }

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-background">
      <div className="flex h-full min-h-0 min-w-0 flex-1 overflow-hidden">
        <main className="flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <header className="shrink-0 border-b border-border px-4 py-3">
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <p className="text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
                  {projectQ.data?.name ?? "Space"}
                </p>
                <h1 className="mt-1 truncate text-base font-semibold text-foreground">
                  Chat
                </h1>
              </div>
              <div className="flex items-center gap-2">
                <div className="relative">
                  <Search className="pointer-events-none absolute left-2 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <input
                    className="w-56 rounded-sm border border-input bg-background py-1.5 pr-2 pl-8 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2"
                    placeholder="Search messages"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                  />
                </div>
              </div>
            </div>
          </header>

          <section className="flex min-h-0 flex-1 flex-col overflow-hidden">
            {toast ? (
              <div className="shrink-0 px-4 pt-3">
                <div className="inline-flex items-center rounded-sm border border-border bg-card px-3 py-2 text-sm text-muted-foreground">
                  {toast}
                </div>
              </div>
            ) : null}
            <div className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-4 py-4">
              {messagesQ.isPending ? (
                <div className="text-sm text-muted-foreground">Loading messages…</div>
              ) : null}

              {search.trim() ? (
                <div className="rounded-sm border border-border bg-card p-3">
                  <div className="text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
                    Search results
                  </div>
                  <div className="mt-2 space-y-2">
                    {(searchQ.data ?? [])
                      .filter((m) => !m.deleted_at)
                      .map((m) => (
                      <div key={m.id} className="rounded-sm border border-border px-3 py-2">
                        <div className="text-xs text-muted-foreground">
                          {formatMinute(m.created_at)}
                        </div>
                        <div className="mt-1 whitespace-pre-wrap break-words text-sm text-foreground">
                          {renderMentions({
                            content: m.content,
                            userLabelById,
                            roleLabelById,
                            fileLabelById,
                          })}
                          {(() => {
                            const refs = m.metadata?.file_edit_proposals;
                            if (!refs?.length || !orgId || !projectId || !chatRoomId) return null;
                            return refs.map((ref) => (
                              <ChatFileProposalCard
                                key={ref.proposal_id}
                                orgId={orgId}
                                spaceId={projectId}
                                chatRoomId={chatRoomId}
                                proposalId={ref.proposal_id}
                                nodeId={ref.node_id}
                                fileNameFallback={ref.file_name ?? ""}
                              />
                            ));
                          })()}
                        </div>
                      </div>
                    ))}
                    {searchQ.data?.length === 0 && !searchQ.isPending ? (
                      <div className="text-sm text-muted-foreground">No matches.</div>
                    ) : null}
                  </div>
                </div>
              ) : (
                <>
                  {messages.length === 0 && !messagesQ.isPending ? (
                    <div className="rounded-sm border border-border bg-card px-3 py-3 text-sm text-muted-foreground">
                      No messages yet — send the first one.
                    </div>
                  ) : null}
                  {visibleMessages.map((m, idx) => {
                    const prev = idx > 0 ? visibleMessages[idx - 1] : undefined;
                    const sameAuthor =
                      !!prev &&
                      (prev.author_user_id ?? "") === (m.author_user_id ?? "") &&
                      prev.author_user_id !== null &&
                      m.author_user_id !== null;
                    const sameMinute =
                      !!prev &&
                      Math.floor(Date.parse(prev.created_at) / 60000) ===
                        Math.floor(Date.parse(m.created_at) / 60000);
                    const showMeta = !(sameAuthor && sameMinute);
                    const rxs = reactionsByMessage.get(m.id) ?? [];
                    const ats = attachmentsByMessage.get(m.id) ?? [];
                    const byEmoji = new Map<string, number>();
                    for (const r of rxs) {
                      byEmoji.set(r.emoji, (byEmoji.get(r.emoji) ?? 0) + 1);
                    }
                    const compact = !showMeta;
                    const rowPad = compact ? "py-0.5" : "py-2";
                    const rowTop = compact ? "" : idx === 0 ? "" : "mt-1.5";
                    const contentTop = compact ? "mt-0" : "mt-1";

                    return (
                      <Fragment key={m.id}>
                      <div
                        className={[
                          "group rounded-sm px-2 hover:bg-accent/20",
                          rowPad,
                          rowTop,
                        ].join(" ")}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0">
                            {showMeta ? (
                              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                                <span className="text-sm font-medium text-foreground">
                                  {authorDisplayName(m)}
                                </span>
                                <span className="text-xs text-muted-foreground">
                                  {formatMinute(m.created_at)}
                                  {m.edited_at ? " (edited)" : ""}
                                </span>
                              </div>
                            ) : null}
                            <div
                              className={`${contentTop} whitespace-pre-wrap break-words text-sm text-foreground`}
                            >
                              {m.metadata?.ai_agent_run ? (
                                <AgentRunCard meta={m.metadata.ai_agent_run} />
                              ) : null}
                              {renderMentions({
                                content: m.content,
                                userLabelById,
                                roleLabelById,
                                fileLabelById,
                              })}
                              {(() => {
                                const refs = m.metadata?.file_edit_proposals;
                                if (!refs?.length || !orgId || !projectId || !chatRoomId) return null;
                                return refs.map((ref) => (
                                  <ChatFileProposalCard
                                    key={ref.proposal_id}
                                    orgId={orgId}
                                    spaceId={projectId}
                                    chatRoomId={chatRoomId}
                                    proposalId={ref.proposal_id}
                                    nodeId={ref.node_id}
                                    fileNameFallback={ref.file_name ?? ""}
                                  />
                                ));
                              })()}
                            </div>
                            {ats.length > 0 ? (
                              <div className="mt-2 space-y-1">
                                {ats.map((a) => (
                                  <a
                                    key={a.id}
                                    className="block truncate text-sm text-link hover:underline"
                                    href={a.url}
                                    target="_blank"
                                    rel="noreferrer"
                                  >
                                    {a.name || a.url}
                                  </a>
                                ))}
                              </div>
                            ) : null}
                            {byEmoji.size > 0 ? (
                              <div className="mt-2 flex flex-wrap gap-1.5">
                                {[...byEmoji.entries()].map(([emoji, count]) => (
                                  <button
                                    key={emoji}
                                    type="button"
                                    className="rounded-sm border border-border bg-card px-2 py-1 text-xs text-foreground hover:bg-accent"
                                    onClick={() => addReaction.mutate({ messageId: m.id, emoji })}
                                    title="Add reaction"
                                  >
                                    {emoji} {count}
                                  </button>
                                ))}
                              </div>
                            ) : null}
                          </div>

                          <div className="mt-1 hidden items-center gap-1 group-hover:flex">
                            <button
                              type="button"
                              className="rounded-sm p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                              title="React"
                              onClick={() => addReaction.mutate({ messageId: m.id, emoji: "👍" })}
                            >
                              <SmilePlus className="h-4 w-4" />
                            </button>
                            <button
                              type="button"
                              className="rounded-sm p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                              title="Delete"
                              onClick={() => deleteMessage.mutate(m.id)}
                              disabled={deleteMessage.isPending}
                            >
                              <Trash2 className="h-4 w-4" />
                            </button>
                          </div>
                        </div>
                      </div>
                      {agentReplyPending?.userMessageId === m.id &&
                      agentReplyPending.pendingAgentUserIds.length > 0 ? (
                        <AgentThinkingPlaceholderRow
                          pendingAgentUserIds={agentReplyPending.pendingAgentUserIds}
                          userLabelById={userLabelById}
                          spaceMembers={spaceMembers}
                        />
                      ) : null}
                      </Fragment>
                    );
                  })}
                </>
              )}
            </div>

            <div className="shrink-0 border-t border-border p-3">
              <div className="mx-auto max-w-3xl">
                <form onSubmit={onSend} className="relative">
                  {attachOpen ? (
                    <div className="absolute bottom-[calc(100%+8px)] left-0 right-0 rounded-sm border border-border bg-card p-2 shadow-md">
                      <div className="flex items-center gap-2">
                        <Paperclip className="h-4 w-4 text-muted-foreground" />
                        <input
                          className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2"
                          placeholder="Paste attachment URL"
                          value={attachUrl}
                          onChange={(e) => setAttachUrl(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === "Escape") {
                              setAttachOpen(false);
                            }
                          }}
                          autoFocus
                        />
                        <button
                          type="button"
                          className="rounded-sm border border-border px-3 py-2 text-sm hover:bg-accent"
                          onClick={() => setAttachOpen(false)}
                        >
                          Done
                        </button>
                      </div>
                      {attachUrl.trim() ? (
                        <div className="mt-2 flex items-center justify-between gap-2 rounded-sm border border-border bg-background px-3 py-2">
                          <div className="min-w-0 truncate text-xs text-muted-foreground">
                            Attached: <span className="text-foreground">{attachUrl.trim()}</span>
                          </div>
                          <button
                            type="button"
                            className="text-xs text-muted-foreground hover:text-foreground"
                            onClick={() => setAttachUrl("")}
                          >
                            Remove
                          </button>
                        </div>
                      ) : null}
                    </div>
                  ) : null}

                  <div className="relative flex items-end gap-2 rounded-sm border border-input bg-background px-2 py-2 focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-2 focus-within:ring-offset-background">
                    <button
                      type="button"
                      className="rounded-sm p-2 text-muted-foreground hover:bg-accent hover:text-foreground"
                      title="Attach"
                      onClick={() => setAttachOpen((v) => !v)}
                    >
                      <Paperclip className="h-4 w-4" />
                    </button>

                    <div className="relative w-full">
                      <div
                        ref={composerOverlayRef}
                        className="pointer-events-none absolute inset-0 min-h-[40px] overflow-hidden px-1 py-1 text-sm text-foreground"
                        style={{ whiteSpace: "pre-wrap", overflowWrap: "break-word" }}
                        aria-hidden
                      >
                        {draft
                          ? renderMentions({
                              content: draft,
                              userLabelById,
                              roleLabelById,
                              fileLabelById,
                            })
                          : null}
                      </div>

                      <textarea
                        ref={composerRef}
                        className="relative min-h-[40px] w-full resize-none bg-transparent px-1 py-1 text-sm text-transparent outline-none placeholder:text-muted-foreground"
                        style={{ caretColor: "hsl(var(--foreground))" }}
                        placeholder="Message — @ members, roles, or files (Enter to send, Shift+Enter newline)"
                        value={draft}
                        onScroll={(e) => {
                          const top = e.currentTarget.scrollTop;
                          if (composerOverlayRef.current) {
                            composerOverlayRef.current.scrollTop = top;
                          }
                        }}
                        onChange={(e) =>
                          onComposerChange(
                            e.target.value,
                            e.target.selectionStart ?? e.target.value.length,
                          )
                        }
                        onClick={(e) => {
                          const el = e.currentTarget;
                          onComposerChange(el.value, el.selectionStart ?? el.value.length);
                        }}
                        onKeyUp={(e) => {
                          const el = e.currentTarget;
                          onComposerChange(el.value, el.selectionStart ?? el.value.length);
                        }}
                        onKeyDown={onComposerKeyDown}
                        rows={1}
                      />
                    </div>

                    {mentionOpen ? (
                      <div className="absolute bottom-[calc(100%+8px)] left-10 right-2 z-20 max-h-64 overflow-auto rounded-sm border border-border bg-card shadow-md">
                        {mentionCandidates.length === 0 ? (
                          <div className="px-3 py-2 text-sm text-muted-foreground">
                            No matches.
                          </div>
                        ) : (
                          <div className="py-1">
                            {mentionCandidates.map((c, idx) => {
                              const active = idx === mentionActiveIdx;
                              return (
                                <button
                                  key={`${c.kind}:${c.id}`}
                                  type="button"
                                  className={[
                                    "flex w-full items-center justify-between gap-3 px-3 py-2 text-left text-sm",
                                    active ? "bg-accent text-foreground" : "text-foreground hover:bg-accent/60",
                                  ].join(" ")}
                                  onMouseEnter={() => setMentionActiveIdx(idx)}
                                  onMouseDown={(e) => {
                                    // Prevent textarea blur.
                                    e.preventDefault();
                                  }}
                                  onClick={() => {
                                    const start = mentionStart;
                                    const el = composerRef.current;
                                    const caret = el?.selectionStart ?? draft.length;
                                    if (start === null) return;
                                    const { nextText, nextCaret } = insertMentionToken({
                                      text: draft,
                                      caret,
                                      mentionStart: start,
                                      candidate: c,
                                    });
                                    setDraft(nextText);
                                    setMentionOpen(false);
                                    setMentionStart(null);
                                    suppressMentionRef.current = { untilCaret: nextCaret, untilText: nextText };
                                    queueMicrotask(() => {
                                      const el2 = composerRef.current;
                                      if (!el2) return;
                                      el2.focus();
                                      el2.setSelectionRange(nextCaret, nextCaret);
                                    });
                                  }}
                                >
                                  <span className="flex min-w-0 items-center gap-1.5 truncate">
                                    {c.kind === "file" ? (
                                      <FileText className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden />
                                    ) : (
                                      <span className="text-muted-foreground">@</span>
                                    )}
                                    <span className="min-w-0 truncate">{c.label}</span>
                                  </span>
                                  <span className="shrink-0 text-xs text-muted-foreground">
                                    {c.kind === "user"
                                      ? "user"
                                      : c.kind === "role"
                                        ? "role"
                                        : "file"}
                                  </span>
                                </button>
                              );
                            })}
                          </div>
                        )}
                      </div>
                    ) : null}
                  </div>

                  {attachUrl.trim() ? (
                    <div className="mt-2 flex items-center justify-between gap-2 rounded-sm border border-border bg-card px-3 py-2">
                      <div className="min-w-0 truncate text-xs text-muted-foreground">
                        Attached: <span className="text-foreground">{attachUrl.trim()}</span>
                      </div>
                      <button
                        type="button"
                        className="text-xs text-muted-foreground hover:text-foreground"
                        onClick={() => setAttachUrl("")}
                      >
                        Remove
                      </button>
                    </div>
                  ) : null}
                </form>
              </div>
            </div>
          </section>
        </main>

        <aside className="hidden h-full min-h-0 w-64 shrink-0 border-l border-border bg-card md:flex md:flex-col md:overflow-hidden">
          <div className="shrink-0 border-b border-border px-4 py-3">
            <p className="text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
              Members
            </p>
            <p className="mt-1 text-xs text-muted-foreground">
              {spaceMembersQ.isPending ? "Loading…" : `${spaceMembers.length} members`}
            </p>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-2">
            {spaceMembers.map((m) => {
              const status = presenceByUserId.get(m.user_id) ?? "offline";
              const dotClass =
                status === "online"
                  ? "bg-emerald-500"
                  : status === "away"
                    ? "bg-yellow-500"
                    : "bg-muted-foreground/50";
              const display =
                m.display_name?.trim() || m.email.split("@")[0] || m.email;
              const modelLine = aiStaffModelSubtitle(m);
              return (
                <div
                  key={m.user_id}
                  className="flex items-start gap-2 rounded-sm px-2 py-2 text-sm text-foreground hover:bg-accent/30"
                >
                  <div className="relative shrink-0 pt-0.5">
                    <div className="flex h-8 w-8 items-center justify-center rounded-full bg-secondary text-sm font-medium text-secondary-foreground">
                      {avatarInitialFor(m)}
                    </div>
                    <span
                      className={`absolute -right-0.5 -bottom-0.5 h-3 w-3 rounded-full border-2 border-card ${dotClass}`}
                      aria-hidden
                      title={status}
                    />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium">{display}</div>
                    {modelLine ? (
                      <div
                        className="mt-0.5 truncate font-mono text-[10px] leading-snug text-muted-foreground"
                        title={modelLine}
                      >
                        {modelLine}
                      </div>
                    ) : null}
                  </div>
                </div>
              );
            })}
          </div>
        </aside>
      </div>
    </div>
  );
}
