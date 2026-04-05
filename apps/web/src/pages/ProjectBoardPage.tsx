import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { apiFetch } from "../api/http";
import {
  createTask,
  fetchTaskDeliverables,
  fetchTaskMessages,
  linkTaskDeliverable,
  postTaskMessage,
  unlinkTaskDeliverable,
} from "../api/tasks";
import type {
  Board,
  BoardColumn,
  FileNode,
  OrgMemberWithUser,
  Project,
  Task,
} from "../api/types";
import { Trash2 } from "lucide-react";
import {
  fromDatetimeLocalToISO,
  isTaskOverdue,
  toDatetimeLocalValue,
} from "../lib/taskDueDate";
import { useOrgRealtime } from "../hooks/useOrgRealtime";

function findDeliverablesFolderId(nodes: FileNode[]): string | null {
  for (const n of nodes) {
    if (
      n.kind === "folder" &&
      !n.parent_id &&
      n.name.toLowerCase() === "deliverables"
    ) {
      return n.id;
    }
  }
  return null;
}

/** Board column name → status dot (matches default column titles). */
function columnStatusDotClass(columnName: string): string {
  const n = columnName.trim().toLowerCase();
  if (n === "to do") return "bg-muted-foreground";
  if (n === "in progress") return "bg-yellow-500";
  if (n === "done") return "bg-green-500";
  if (n === "overdue") return "bg-red-500";
  return "bg-muted-foreground";
}

export default function ProjectBoardPage() {
  const { orgId, projectId, boardId } = useParams<{
    orgId: string;
    projectId: string;
    boardId: string;
  }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const qc = useQueryClient();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [createColumnId, setCreateColumnId] = useState<string | null>(null);

  const [createTitle, setCreateTitle] = useState("");
  const [createDescription, setCreateDescription] = useState("");
  const [createAssignee, setCreateAssignee] = useState<string>("");
  const [createDeliverable, setCreateDeliverable] = useState(false);
  const [createDeliverableNotes, setCreateDeliverableNotes] = useState("");
  const [createDueAt, setCreateDueAt] = useState("");

  const [editTitle, setEditTitle] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [editAssignee, setEditAssignee] = useState<string>("");
  const [editColumnId, setEditColumnId] = useState<string>("");
  const [editDueAt, setEditDueAt] = useState("");
  const [messageDraft, setMessageDraft] = useState("");
  const [linkFileId, setLinkFileId] = useState<string>("");

  useOrgRealtime(orgId, !!orgId);

  const projectQ = useQuery({
    queryKey: ["project", orgId, projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${projectId}`);
      if (!res.ok) throw new Error("project");
      return res.json() as Promise<Project>;
    },
  });

  const boardQ = useQuery({
    queryKey: ["board", orgId, projectId, boardId],
    enabled: !!orgId && !!projectId && !!boardId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/boards/${boardId}`,
      );
      if (!res.ok) throw new Error("board");
      return res.json() as Promise<{ board: Board; columns: BoardColumn[] }>;
    },
  });

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

  const tasksQ = useQuery({
    queryKey: ["tasks", projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/tasks`,
      );
      if (!res.ok) throw new Error("tasks");
      const j = (await res.json()) as { tasks: Task[] };
      return j.tasks;
    },
  });

  const selected = useMemo(() => {
    if (!selectedId || !tasksQ.data) return null;
    return tasksQ.data.find((t) => t.id === selectedId) ?? null;
  }, [selectedId, tasksQ.data]);

  useEffect(() => {
    const tid = searchParams.get("task");
    if (!tid) {
      setSelectedId(null);
      return;
    }
    if (tasksQ.data?.some((t) => t.id === tid)) {
      setSelectedId(tid);
    }
  }, [searchParams, tasksQ.data]);

  useEffect(() => {
    if (!selected) return;
    setEditTitle(selected.title);
    setEditDescription(selected.description);
    setEditAssignee(selected.assignee_user_id ?? "");
    setEditColumnId(selected.column_id);
    setEditDueAt(toDatetimeLocalValue(selected.due_at));
  }, [selected?.id, selected?.version, selected?.due_at]);

  const fileTreeQ = useQuery({
    queryKey: ["file-tree", orgId, projectId],
    enabled:
      !!orgId &&
      !!projectId &&
      !!selected?.deliverable_required,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files/tree`,
      );
      if (!res.ok) throw new Error("tree");
      return res.json() as Promise<{ nodes: FileNode[] }>;
    },
  });

  const deliverablesFolderId = useMemo(
    () => findDeliverablesFolderId(fileTreeQ.data?.nodes ?? []),
    [fileTreeQ.data?.nodes],
  );

  const deliverableFilesQ = useQuery({
    queryKey: ["deliverables-folder-files", orgId, projectId, deliverablesFolderId],
    enabled: !!orgId && !!projectId && !!deliverablesFolderId,
    queryFn: async () => {
      const qs = new URLSearchParams({ parentId: deliverablesFolderId! });
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/files?${qs}`,
      );
      if (!res.ok) throw new Error("files");
      const j = (await res.json()) as { nodes: FileNode[] };
      return j.nodes.filter((n) => n.kind === "file");
    },
  });

  const messagesQ = useQuery({
    queryKey: ["task-messages", projectId, selectedId],
    enabled: !!orgId && !!projectId && !!selectedId,
    queryFn: () =>
      fetchTaskMessages(orgId!, projectId!, selectedId!),
  });

  const deliverablesQ = useQuery({
    queryKey: ["task-deliverables", projectId, selectedId],
    enabled: !!orgId && !!projectId && !!selectedId && !!selected?.deliverable_required,
    queryFn: () =>
      fetchTaskDeliverables(orgId!, projectId!, selectedId!),
  });

  const tasksByColumn = useMemo(() => {
    const m = new Map<string, Task[]>();
    const list = (tasksQ.data ?? []).filter((t) => t.board_id === boardId);
    for (const t of list) {
      const arr = m.get(t.column_id) ?? [];
      arr.push(t);
      m.set(t.column_id, arr);
    }
    for (const [, arr] of m) {
      arr.sort(
        (a, b) =>
          a.position - b.position || a.created_at.localeCompare(b.created_at),
      );
    }
    return m;
  }, [tasksQ.data, boardId]);

  const createTaskMut = useMutation({
    mutationFn: async () => {
      if (!orgId || !projectId || !createColumnId) throw new Error("ctx");
      const dueISO = fromDatetimeLocalToISO(createDueAt);
      return createTask(orgId, projectId, {
        title: createTitle.trim(),
        description: createDescription.trim(),
        column_id: createColumnId,
        assignee_user_id: createAssignee || null,
        ...(dueISO ? { due_at: dueISO } : {}),
        deliverable_required: createDeliverable,
        deliverable_instructions: createDeliverableNotes.trim(),
      });
    },
    onSuccess: () => {
      setCreateColumnId(null);
      setCreateTitle("");
      setCreateDescription("");
      setCreateAssignee("");
      setCreateDeliverable(false);
      setCreateDeliverableNotes("");
      setCreateDueAt("");
      void qc.invalidateQueries({ queryKey: ["tasks", projectId] });
    },
  });

  const patchTask = useMutation({
    mutationFn: async (patch: {
      task: Task;
      body: Record<string, unknown>;
    }) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/tasks/${patch.task.id}`,
        { method: "PATCH", json: patch.body },
      );
      if (!res.ok) throw new Error("patch");
      return res.json() as Promise<Task>;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["tasks", projectId] });
    },
  });

  const postMessageMut = useMutation({
    mutationFn: async () => {
      if (!orgId || !projectId || !selectedId || !messageDraft.trim()) {
        throw new Error("message");
      }
      return postTaskMessage(orgId, projectId, selectedId, messageDraft.trim());
    },
    onSuccess: () => {
      setMessageDraft("");
      void qc.invalidateQueries({ queryKey: ["task-messages", projectId, selectedId] });
    },
  });

  const linkDelMut = useMutation({
    mutationFn: async () => {
      if (!orgId || !projectId || !selectedId || !linkFileId) throw new Error("link");
      return linkTaskDeliverable(orgId, projectId, selectedId, linkFileId);
    },
    onSuccess: () => {
      setLinkFileId("");
      void qc.invalidateQueries({ queryKey: ["task-deliverables", projectId, selectedId] });
    },
  });

  const unlinkDelMut = useMutation({
    mutationFn: async (fileNodeId: string) => {
      if (!orgId || !projectId || !selectedId) throw new Error("unlink");
      return unlinkTaskDeliverable(orgId, projectId, selectedId, fileNodeId);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["task-deliverables", projectId, selectedId] });
    },
  });

  const deleteTaskMut = useMutation({
    mutationFn: async (taskId: string) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/tasks/${taskId}`,
        { method: "DELETE" },
      );
      if (!res.ok) throw new Error("delete task");
    },
    onSuccess: (_data, taskId) => {
      setSelectedId((prev) => (prev === taskId ? null : prev));
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        if (next.get("task") === taskId) next.delete("task");
        return next;
      }, { replace: true });
      void qc.invalidateQueries({ queryKey: ["tasks", projectId] });
    },
  });

  function openTask(t: Task) {
    setSelectedId(t.id);
    const next = new URLSearchParams(searchParams);
    next.set("task", t.id);
    setSearchParams(next, { replace: false });
  }

  function closeTask() {
    setSelectedId(null);
    const next = new URLSearchParams(searchParams);
    next.delete("task");
    setSearchParams(next, { replace: true });
  }

  function saveDetail(e: FormEvent) {
    e.preventDefault();
    if (!selected) return;
    const body: Record<string, unknown> = {
      title: editTitle.trim(),
      description: editDescription.trim(),
      column_id: editColumnId,
      assignee_user_id: editAssignee || null,
    };
    const dueISO = fromDatetimeLocalToISO(editDueAt);
    if (dueISO) {
      body.due_at = dueISO;
    } else if (selected.due_at) {
      body.clear_due_at = true;
    }
    patchTask.mutate({
      task: selected,
      body,
    });
  }

  const cols = boardQ.data?.columns ?? [];

  const memberLabel = (m: OrgMemberWithUser) => {
    const base =
      m.display_name?.trim() || m.email.split("@")[0] || m.email;
    if (m.is_service_account) return `${base} (AI)`;
    return base;
  };

  const messageAuthorLabel = (userId: string | null | undefined) => {
    if (!userId) return "System";
    const mem = membersQ.data?.find((x) => x.user_id === userId);
    return mem ? memberLabel(mem) : "Member";
  };

  if (!orgId || !projectId || !boardId) {
    return null;
  }

  const filesLink = `/o/${orgId}/p/${projectId}/files`;

  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-background pb-24">
      <header className="border-b border-border px-4 py-4">
        <div className="mx-auto max-w-[1400px]">
          <Link
            to={`/o/${orgId}`}
            className="text-xs text-link hover:underline"
          >
            ← {projectQ.data?.name ?? "Space"}
          </Link>
          <p className="mt-2 text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
            Task board
          </p>
          <h1 className="mt-1 text-xl font-semibold tracking-tight text-foreground">
            {boardQ.data?.board.name ?? "…"}
          </h1>
        </div>
      </header>

      <div className="mx-auto max-w-[1400px] px-4 pt-6">
        <div className="flex gap-4 overflow-x-auto pb-4">
          {cols.map((c) => (
            <div
              key={c.id}
              className="w-72 shrink-0 rounded-sm border border-border bg-card"
            >
              <div className="flex items-center justify-between border-b border-border px-3 py-2">
                <span className="text-sm font-medium text-muted-foreground">
                  {c.name}
                </span>
                <button
                  type="button"
                  className="rounded-sm px-2 py-0.5 text-xs text-link hover:underline"
                  onClick={() => {
                    setCreateColumnId(c.id);
                    setCreateTitle("");
                    setCreateDescription("");
                    setCreateAssignee("");
                    setCreateDeliverable(false);
                    setCreateDeliverableNotes("");
                    setCreateDueAt("");
                  }}
                >
                  New task
                </button>
              </div>
              <div className="space-y-2 p-2">
                {(tasksByColumn.get(c.id) ?? []).map((t) => (
                  <div
                    key={t.id}
                    className="group flex items-stretch rounded-sm border border-border bg-background transition-colors hover:border-muted-foreground/50"
                  >
                    <button
                      type="button"
                      onClick={() => openTask(t)}
                      className="min-w-0 flex-1 px-3 py-2 text-left text-sm text-foreground"
                    >
                      <div className="font-medium">{t.title}</div>
                      {t.assignee_user_id ? (
                        <div className="mt-1 text-xs text-muted-foreground">
                          Assignee:{" "}
                          {(() => {
                            const m = membersQ.data?.find(
                              (x) => x.user_id === t.assignee_user_id,
                            );
                            return m ? memberLabel(m) : "Member";
                          })()}
                        </div>
                      ) : null}
                      {t.due_at ? (
                        <div
                          className={
                            isTaskOverdue(t.due_at)
                              ? "mt-1 text-xs font-medium text-destructive"
                              : "mt-1 text-xs text-muted-foreground"
                          }
                        >
                          Due{" "}
                          {new Date(t.due_at).toLocaleString(undefined, {
                            dateStyle: "medium",
                            timeStyle: "short",
                          })}
                        </div>
                      ) : null}
                    </button>
                    <div className="flex shrink-0 items-center gap-1 py-2 pr-2">
                      <span
                        className={`h-2 w-2 shrink-0 rounded-full ${columnStatusDotClass(c.name)}`}
                        title={c.name}
                        aria-hidden
                      />
                      <button
                        type="button"
                        title="Delete task"
                        disabled={deleteTaskMut.isPending}
                        className="rounded-sm p-1.5 text-muted-foreground opacity-0 transition-opacity hover:bg-destructive/15 hover:text-destructive group-hover:opacity-100 disabled:opacity-40"
                        onClick={(e) => {
                          e.stopPropagation();
                          if (
                            !window.confirm(
                              `Delete “${t.title}”? This cannot be undone.`,
                            )
                          ) {
                            return;
                          }
                          deleteTaskMut.mutate(t.id);
                        }}
                      >
                        <Trash2 className="h-3.5 w-3.5" aria-hidden />
                        <span className="sr-only">Delete task</span>
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>

      {createColumnId && (
        <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/60 sm:items-center">
          <div className="max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-t-sm border border-border bg-card p-6 sm:rounded-sm">
            <h3 className="text-lg font-semibold text-card-foreground">
              New task
            </h3>
            <form
              className="mt-4 space-y-3"
              onSubmit={(e) => {
                e.preventDefault();
                if (!createTitle.trim()) return;
                createTaskMut.mutate();
              }}
            >
              <div>
                <label className="text-xs text-muted-foreground">Title</label>
                <input
                  required
                  className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                  value={createTitle}
                  onChange={(e) => setCreateTitle(e.target.value)}
                />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Description</label>
                <textarea
                  className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                  rows={3}
                  value={createDescription}
                  onChange={(e) => setCreateDescription(e.target.value)}
                />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Assignee</label>
                <select
                  className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                  value={createAssignee}
                  onChange={(e) => setCreateAssignee(e.target.value)}
                >
                  <option value="">—</option>
                  {(membersQ.data ?? []).map((m) => (
                    <option key={m.user_id} value={m.user_id}>
                      {memberLabel(m)}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Due date</label>
                <input
                  type="datetime-local"
                  className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                  value={createDueAt}
                  onChange={(e) => setCreateDueAt(e.target.value)}
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  Optional — leave empty for no due date.
                </p>
              </div>
              <div className="flex items-center gap-2">
                <input
                  id="del-req"
                  type="checkbox"
                  checked={createDeliverable}
                  onChange={(e) => setCreateDeliverable(e.target.checked)}
                />
                <label htmlFor="del-req" className="text-sm text-foreground">
                  Ask for a deliverable (files in Deliverables folder)
                </label>
              </div>
              {createDeliverable ? (
                <div>
                  <label className="text-xs text-muted-foreground">
                    Deliverable instructions
                  </label>
                  <textarea
                    className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                    rows={2}
                    value={createDeliverableNotes}
                    onChange={(e) => setCreateDeliverableNotes(e.target.value)}
                    placeholder="What should be submitted?"
                  />
                </div>
              ) : null}
              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  className="rounded-sm border border-border bg-transparent px-4 py-2 text-sm text-foreground hover:bg-accent"
                  onClick={() => setCreateColumnId(null)}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={createTaskMut.isPending}
                  className="rounded-sm bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-60"
                >
                  Create
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {selected && (
        <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/60 sm:items-center">
          <div className="max-h-[92vh] w-full max-w-2xl overflow-y-auto rounded-t-sm border border-border bg-card p-6 sm:rounded-sm">
            <div className="flex items-start justify-between gap-3">
              <h3 className="text-lg font-semibold text-card-foreground">
                Task
              </h3>
              <button
                type="button"
                className="text-sm text-muted-foreground hover:text-foreground"
                onClick={closeTask}
              >
                Close
              </button>
            </div>
            <form className="mt-4 space-y-3" onSubmit={saveDetail}>
              <div>
                <label className="text-xs text-muted-foreground">Title</label>
                <input
                  className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                  value={editTitle}
                  onChange={(e) => setEditTitle(e.target.value)}
                />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Description</label>
                <textarea
                  className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                  rows={3}
                  value={editDescription}
                  onChange={(e) => setEditDescription(e.target.value)}
                />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Status (column)</label>
                <select
                  className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                  value={editColumnId}
                  onChange={(e) => setEditColumnId(e.target.value)}
                >
                  {cols.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.name}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Assignee</label>
                <select
                  className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                  value={editAssignee}
                  onChange={(e) => setEditAssignee(e.target.value)}
                >
                  <option value="">—</option>
                  {(membersQ.data ?? []).map((m) => (
                    <option key={m.user_id} value={m.user_id}>
                      {memberLabel(m)}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Due date</label>
                <input
                  type="datetime-local"
                  className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
                  value={editDueAt}
                  onChange={(e) => setEditDueAt(e.target.value)}
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  Clear the field and save to remove the due date.
                </p>
              </div>
              <div className="flex justify-end gap-2 pt-1">
                <button
                  type="submit"
                  disabled={patchTask.isPending}
                  className="rounded-sm bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-60"
                >
                  Save changes
                </button>
              </div>
            </form>

            {selected.deliverable_required ? (
              <section className="mt-6 border-t border-border pt-4">
                <h4 className="text-xs font-semibold uppercase tracking-[0.15em] text-muted-foreground">
                  Deliverable
                </h4>
                {selected.deliverable_instructions ? (
                  <p className="mt-2 text-sm text-foreground">
                    {selected.deliverable_instructions}
                  </p>
                ) : null}
                <p className="mt-2 text-xs text-muted-foreground">
                  Upload files into the{" "}
                  <Link to={filesLink} className="text-link hover:underline">
                    Deliverables
                  </Link>{" "}
                  folder in Files, then link them here.
                </p>
                <div className="mt-3 flex flex-wrap gap-2">
                  <select
                    className="min-w-[12rem] rounded-sm border border-input bg-background px-2 py-1.5 text-sm"
                    value={linkFileId}
                    onChange={(e) => setLinkFileId(e.target.value)}
                  >
                    <option value="">Link a file from Deliverables…</option>
                    {(deliverableFilesQ.data ?? []).map((f) => (
                      <option key={f.id} value={f.id}>
                        {f.name}
                      </option>
                    ))}
                  </select>
                  <button
                    type="button"
                    disabled={!linkFileId || linkDelMut.isPending}
                    className="rounded-sm bg-secondary px-3 py-1.5 text-sm text-secondary-foreground hover:bg-muted disabled:opacity-50"
                    onClick={() => linkDelMut.mutate()}
                  >
                    Link
                  </button>
                </div>
                {!deliverablesFolderId && fileTreeQ.isFetched ? (
                  <p className="mt-2 text-xs text-amber-600 dark:text-amber-400">
                    No Deliverables folder yet — it is created when you first ask for a
                    deliverable, or open Files and create a folder named Deliverables.
                  </p>
                ) : null}
                <ul className="mt-3 space-y-2">
                  {(deliverablesQ.data ?? []).map((d) => (
                    <li
                      key={d.file_node_id}
                      className="flex items-center justify-between gap-2 rounded-sm border border-border px-2 py-1.5 text-sm"
                    >
                      <span className="truncate text-foreground">{d.file_name}</span>
                      <button
                        type="button"
                        className="shrink-0 text-xs text-muted-foreground hover:text-destructive"
                        onClick={() => unlinkDelMut.mutate(d.file_node_id)}
                      >
                        Remove
                      </button>
                    </li>
                  ))}
                </ul>
              </section>
            ) : null}

            <section className="mt-6 border-t border-border pt-4">
              <h4 className="text-xs font-semibold uppercase tracking-[0.15em] text-muted-foreground">
                Discussion
              </h4>
              <div className="mt-2 max-h-48 space-y-2 overflow-y-auto rounded-sm border border-border bg-background p-2">
                {(messagesQ.data ?? []).length === 0 ? (
                  <p className="text-xs text-muted-foreground">No messages yet.</p>
                ) : (
                  (messagesQ.data ?? []).map((m) => (
                    <div key={m.id} className="text-sm">
                      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0">
                        <span className="font-medium text-foreground">
                          {messageAuthorLabel(m.author_user_id)}
                        </span>
                        <span className="text-xs text-muted-foreground">
                          {new Date(m.created_at).toLocaleString()}
                        </span>
                      </div>
                      <p className="mt-1 whitespace-pre-wrap text-foreground">
                        {m.content}
                      </p>
                    </div>
                  ))
                )}
              </div>
              <div className="mt-2 flex gap-2">
                <textarea
                  className="min-h-[72px] flex-1 rounded-sm border border-input bg-background px-2 py-1.5 text-sm"
                  placeholder="Write a message…"
                  value={messageDraft}
                  onChange={(e) => setMessageDraft(e.target.value)}
                />
                <button
                  type="button"
                  disabled={!messageDraft.trim() || postMessageMut.isPending}
                  className="self-end rounded-sm bg-primary px-3 py-2 text-sm text-primary-foreground disabled:opacity-50"
                  onClick={() => postMessageMut.mutate()}
                >
                  Send
                </button>
              </div>
            </section>
          </div>
        </div>
      )}
    </div>
  );
}
