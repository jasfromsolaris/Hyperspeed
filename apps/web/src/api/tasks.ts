import { apiFetch } from "./http";
import type {
  MyAssignedTask,
  Task,
  TaskDeliverableFile,
  TaskMessage,
} from "./types";

export async function fetchMyAssignedTasks(orgId: string): Promise<MyAssignedTask[]> {
  const res = await apiFetch(
    `/api/v1/me/tasks?org_id=${encodeURIComponent(orgId)}`,
  );
  if (!res.ok) throw new Error("my tasks");
  const j = (await res.json()) as { tasks: MyAssignedTask[] };
  return j.tasks ?? [];
}

export async function fetchTaskMessages(
  orgId: string,
  spaceId: string,
  taskId: string,
): Promise<TaskMessage[]> {
  const res = await apiFetch(
    `/api/v1/organizations/${orgId}/spaces/${spaceId}/tasks/${taskId}/messages`,
  );
  if (!res.ok) throw new Error("task messages");
  const j = (await res.json()) as { messages: TaskMessage[] };
  return j.messages ?? [];
}

export async function postTaskMessage(
  orgId: string,
  spaceId: string,
  taskId: string,
  content: string,
): Promise<TaskMessage> {
  const res = await apiFetch(
    `/api/v1/organizations/${orgId}/spaces/${spaceId}/tasks/${taskId}/messages`,
    { method: "POST", json: { content } },
  );
  if (!res.ok) throw new Error("post task message");
  return res.json() as Promise<TaskMessage>;
}

export async function fetchTaskDeliverables(
  orgId: string,
  spaceId: string,
  taskId: string,
): Promise<TaskDeliverableFile[]> {
  const res = await apiFetch(
    `/api/v1/organizations/${orgId}/spaces/${spaceId}/tasks/${taskId}/deliverables`,
  );
  if (!res.ok) throw new Error("task deliverables");
  const j = (await res.json()) as { deliverables: TaskDeliverableFile[] };
  return j.deliverables ?? [];
}

export async function linkTaskDeliverable(
  orgId: string,
  spaceId: string,
  taskId: string,
  fileNodeId: string,
): Promise<void> {
  const res = await apiFetch(
    `/api/v1/organizations/${orgId}/spaces/${spaceId}/tasks/${taskId}/deliverables`,
    { method: "POST", json: { file_node_id: fileNodeId } },
  );
  if (!res.ok) throw new Error("link deliverable");
}

export async function unlinkTaskDeliverable(
  orgId: string,
  spaceId: string,
  taskId: string,
  fileNodeId: string,
): Promise<void> {
  const res = await apiFetch(
    `/api/v1/organizations/${orgId}/spaces/${spaceId}/tasks/${taskId}/deliverables/${fileNodeId}`,
    { method: "DELETE" },
  );
  if (!res.ok) throw new Error("unlink deliverable");
}

export type CreateTaskInput = {
  title: string;
  description: string;
  column_id: string;
  assignee_user_id?: string | null;
  /** RFC3339 */
  due_at?: string;
  deliverable_required?: boolean;
  deliverable_instructions?: string;
};

export async function createTask(
  orgId: string,
  spaceId: string,
  input: CreateTaskInput,
): Promise<Task> {
  const payload: Record<string, unknown> = {
    title: input.title,
    description: input.description,
    column_id: input.column_id,
  };
  if (input.assignee_user_id) {
    payload.assignee_user_id = input.assignee_user_id;
  }
  if (input.due_at) {
    payload.due_at = input.due_at;
  }
  if (input.deliverable_required) {
    payload.deliverable_required = true;
    payload.deliverable_instructions = input.deliverable_instructions ?? "";
  }
  const res = await apiFetch(
    `/api/v1/organizations/${orgId}/spaces/${spaceId}/tasks`,
    { method: "POST", json: payload },
  );
  if (!res.ok) throw new Error("create task");
  return res.json() as Promise<Task>;
}
