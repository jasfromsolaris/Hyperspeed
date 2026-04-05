import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { apiFetch } from "../api/http";
import { fetchOrganizationsList } from "../api/orgs";
import type { ChatRoom, Notification, OrgMemberWithUser, Project } from "../api/types";

type MentionPayload = {
  space_id?: string;
  chat_room_id?: string;
  message_id?: string;
  from_user_id?: string;
};

type TaskAssignedPayload = {
  space_id?: string;
  board_id?: string;
  task_id?: string;
  title?: string;
  assigned_by_user_id?: string;
};

function asMentionPayload(p: unknown): MentionPayload {
  if (!p || typeof p !== "object") return {};
  const o = p as Record<string, unknown>;
  return {
    space_id: typeof o.space_id === "string" ? o.space_id : undefined,
    chat_room_id: typeof o.chat_room_id === "string" ? o.chat_room_id : undefined,
    message_id: typeof o.message_id === "string" ? o.message_id : undefined,
    from_user_id: typeof o.from_user_id === "string" ? o.from_user_id : undefined,
  };
}

function asTaskAssignedPayload(p: unknown): TaskAssignedPayload {
  if (!p || typeof p !== "object") return {};
  const o = p as Record<string, unknown>;
  return {
    space_id: typeof o.space_id === "string" ? o.space_id : undefined,
    board_id: typeof o.board_id === "string" ? o.board_id : undefined,
    task_id: typeof o.task_id === "string" ? o.task_id : undefined,
    title: typeof o.title === "string" ? o.title : undefined,
    assigned_by_user_id:
      typeof o.assigned_by_user_id === "string"
        ? o.assigned_by_user_id
        : undefined,
  };
}

export default function InboxPage() {
  const qc = useQueryClient();
  const nav = useNavigate();

  const orgsQ = useQuery({
    queryKey: ["orgs"],
    queryFn: fetchOrganizationsList,
  });

  /** Inbox API is org-scoped; use the user's first workspace. */
  const orgId = orgsQ.data?.organizations?.[0]?.id ?? "";

  const notificationsQ = useQuery({
    queryKey: ["notifications", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/me/notifications?org_id=${encodeURIComponent(orgId)}`);
      if (!res.ok) throw new Error("notifications");
      return res.json() as Promise<{ notifications: Notification[]; unread_count: number }>;
    },
  });

  const markRead = useMutation({
    mutationFn: async (ids: string[]) => {
      const res = await apiFetch("/api/v1/me/notifications/mark-read", {
        method: "POST",
        json: { org_id: orgId, ids },
      });
      if (!res.ok) throw new Error("mark read");
      return res.json() as Promise<{ updated: number }>;
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["notifications", orgId] });
      await qc.invalidateQueries({ queryKey: ["notifications-unread", orgId] });
    },
  });

  const deleteNotifs = useMutation({
    mutationFn: async (ids: string[]) => {
      const res = await apiFetch("/api/v1/me/notifications/delete", {
        method: "POST",
        json: { org_id: orgId, ids },
      });
      if (!res.ok) throw new Error("delete notification");
      return res.json() as Promise<{ deleted: number }>;
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["notifications", orgId] });
      await qc.invalidateQueries({ queryKey: ["notifications-unread", orgId] });
    },
  });

  const unread = notificationsQ.data?.unread_count ?? 0;
  const list = notificationsQ.data?.notifications ?? [];

  const membersQ = useQuery({
    queryKey: ["org-members", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/members`);
      if (!res.ok) throw new Error("members");
      const j = (await res.json()) as { members: OrgMemberWithUser[] };
      return j.members ?? [];
    },
  });

  const fromLabelById = useMemo(() => {
    const m = new Map<string, string>();
    for (const mem of membersQ.data ?? []) {
      const label =
        mem.display_name?.trim() || mem.email.split("@")[0] || mem.email;
      m.set(mem.user_id, label);
    }
    return m;
  }, [membersQ.data]);

  const items = useMemo(() => {
    return list.map((n) => {
      const mentionPayload = asMentionPayload(n.payload);
      const taskPayload = asTaskAssignedPayload(n.payload);
      const isMention = n.type === "chat.mention";
      const isTaskAssigned = n.type === "task.assigned";
      const isTaskOverdue = n.type === "task.overdue";
      let target: string | null = null;
      if (isMention && mentionPayload.space_id && mentionPayload.chat_room_id) {
        target = `/o/${n.organization_id}/p/${mentionPayload.space_id}/c/${mentionPayload.chat_room_id}`;
      } else if (
        (isTaskAssigned || isTaskOverdue) &&
        taskPayload.space_id &&
        taskPayload.board_id &&
        taskPayload.task_id
      ) {
        const q = new URLSearchParams({ task: taskPayload.task_id });
        target = `/o/${n.organization_id}/p/${taskPayload.space_id}/b/${taskPayload.board_id}?${q.toString()}`;
      }
      return {
        n,
        mentionPayload,
        taskPayload,
        isMention,
        isTaskAssigned,
        isTaskOverdue,
        target,
      };
    });
  }, [list]);

  const mentionTargets = useMemo(() => {
    const out: { spaceId: string; roomId: string }[] = [];
    const seen = new Set<string>();
    for (const it of items) {
      if (!it.isMention) continue;
      const spaceId = it.mentionPayload.space_id;
      const roomId = it.mentionPayload.chat_room_id;
      if (!spaceId || !roomId) continue;
      const k = `${spaceId}:${roomId}`;
      if (seen.has(k)) continue;
      seen.add(k);
      out.push({ spaceId, roomId });
    }
    return out;
  }, [items]);

  const spaceQs = useQueries({
    queries: mentionTargets.map((t) => ({
      queryKey: ["project", orgId, t.spaceId] as const,
      enabled: !!orgId && !!t.spaceId,
      queryFn: async () => {
        const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${t.spaceId}`);
        if (!res.ok) throw new Error("space");
        return res.json() as Promise<Project>;
      },
    })),
  });

  const chatRoomsQs = useQueries({
    queries: mentionTargets.map((t) => ({
      queryKey: ["chat-rooms", t.spaceId] as const,
      enabled: !!orgId && !!t.spaceId,
      queryFn: async () => {
        const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${t.spaceId}/chat-rooms`);
        if (!res.ok) throw new Error("chat rooms");
        const j = (await res.json()) as { chat_rooms: ChatRoom[] };
        return j.chat_rooms ?? [];
      },
    })),
  });

  const spaceNameById = useMemo(() => {
    const m = new Map<string, string>();
    for (let i = 0; i < mentionTargets.length; i++) {
      const t = mentionTargets[i];
      const sp = spaceQs[i]?.data;
      if (t?.spaceId && sp?.name) m.set(t.spaceId, sp.name);
    }
    return m;
  }, [mentionTargets, spaceQs]);

  const roomNameByKey = useMemo(() => {
    const m = new Map<string, string>();
    for (let i = 0; i < mentionTargets.length; i++) {
      const t = mentionTargets[i];
      const rooms = chatRoomsQs[i]?.data ?? [];
      const r = rooms.find((x) => x.id === t.roomId);
      if (r?.name) m.set(`${t.spaceId}:${t.roomId}`, r.name);
    }
    return m;
  }, [mentionTargets, chatRoomsQs]);

  return (
    <div className="min-h-0 flex-1 overflow-y-auto px-4 py-8 md:px-8">
      <div className="mx-auto max-w-2xl">
        <div className="flex items-end justify-between gap-3">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
              Inbox
            </p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">
              Notifications{" "}
              {unread ? (
                <span className="ml-2 rounded-full bg-accent px-2 py-0.5 text-sm font-medium text-foreground">
                  {unread}
                </span>
              ) : null}
            </h1>
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              className="rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent disabled:opacity-60"
              onClick={() => markRead.mutate([])}
              disabled={!orgId || markRead.isPending || unread === 0}
              title="Mark all as read"
            >
              Mark all read
            </button>
          </div>
        </div>

        <div className="mt-5 space-y-2">
          {notificationsQ.isPending ? (
            <div className="text-sm text-muted-foreground">Loading…</div>
          ) : null}

          {items.length === 0 && !notificationsQ.isPending ? (
            <div className="rounded-sm border border-border bg-card px-3 py-3 text-sm text-muted-foreground">
              No notifications yet.
            </div>
          ) : null}

          {items.map(
            ({
              n,
              mentionPayload,
              taskPayload,
              isMention,
              isTaskAssigned,
              isTaskOverdue,
              target,
            }) => {
              const unread = !n.read_at;
              const title = isMention
                ? "Mention"
                : isTaskAssigned
                  ? "Task assigned"
                  : isTaskOverdue
                    ? "Task overdue"
                    : n.type;
              const fromMention = mentionPayload.from_user_id
                ? fromLabelById.get(mentionPayload.from_user_id)
                : null;
              const fromTask = taskPayload.assigned_by_user_id
                ? fromLabelById.get(taskPayload.assigned_by_user_id)
                : null;
              const spaceNameMention = mentionPayload.space_id
                ? spaceNameById.get(mentionPayload.space_id)
                : null;
              const roomName =
                mentionPayload.space_id && mentionPayload.chat_room_id
                  ? roomNameByKey.get(
                      `${mentionPayload.space_id}:${mentionPayload.chat_room_id}`,
                    )
                  : null;
              return (
                <div
                  key={n.id}
                  className={[
                    "flex w-full items-stretch gap-0 overflow-hidden rounded-sm border border-border",
                    unread ? "bg-card" : "bg-card/60 opacity-90",
                  ].join(" ")}
                >
                  <button
                    type="button"
                    className="min-w-0 flex-1 px-3 py-3 text-left hover:bg-accent/30"
                    onClick={async () => {
                      if (unread) {
                        markRead.mutate([n.id]);
                      }
                      if (target) {
                        nav(target);
                      }
                    }}
                  >
                    <div className="flex items-center justify-between gap-3">
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium text-foreground">
                          {title}
                        </div>
                      {isMention ? (
                        <div className="mt-1 truncate text-xs text-muted-foreground">
                          {fromMention ? (
                            <span className="text-foreground">{fromMention}</span>
                          ) : (
                            "Someone"
                          )}{" "}
                          mentioned you
                          {spaceNameMention || roomName ? (
                            <>
                              {" "}
                              in{" "}
                              {spaceNameMention ? (
                                <span className="text-foreground">
                                  {spaceNameMention}
                                </span>
                              ) : (
                                "a space"
                              )}
                              {roomName ? (
                                <>
                                  {" "}
                                  /{" "}
                                  <span className="text-foreground">
                                    #{roomName}
                                  </span>
                                </>
                              ) : null}
                            </>
                          ) : null}
                        </div>
                      ) : null}
                      {isTaskAssigned ? (
                        <div className="mt-1 truncate text-xs text-muted-foreground">
                          {fromTask ? (
                            <span className="text-foreground">{fromTask}</span>
                          ) : (
                            "Someone"
                          )}{" "}
                          assigned you
                          {taskPayload.title ? (
                            <>
                              :{" "}
                              <span className="text-foreground">
                                {taskPayload.title}
                              </span>
                            </>
                          ) : null}
                        </div>
                      ) : null}
                      {isTaskOverdue ? (
                        <div className="mt-1 truncate text-xs text-muted-foreground">
                          Past due
                          {taskPayload.title ? (
                            <>
                              :{" "}
                              <span className="text-foreground">
                                {taskPayload.title}
                              </span>
                            </>
                          ) : null}
                        </div>
                      ) : null}
                      <div className="mt-1 truncate text-xs text-muted-foreground">
                        {new Date(n.created_at).toLocaleString()}
                      </div>
                    </div>
                    {unread ? (
                      <span className="h-2 w-2 shrink-0 rounded-full bg-primary" />
                    ) : null}
                  </div>
                  </button>
                  <button
                    type="button"
                    className="flex shrink-0 items-center justify-center border-l border-border px-3 text-muted-foreground hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
                    title="Remove from inbox"
                    aria-label="Remove from inbox"
                    disabled={!orgId || deleteNotifs.isPending}
                    onClick={(e) => {
                      e.stopPropagation();
                      deleteNotifs.mutate([n.id]);
                    }}
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              );
            },
          )}
        </div>
      </div>
    </div>
  );
}
