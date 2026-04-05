import { apiFetch } from "./http";
import type { AgentChatMode, UUID } from "./types";

export class AgentInvokeError extends Error {
  status: number;
  code: string;
  retryable: boolean;

  constructor(message: string, status: number, code = "invoke_error", retryable = false) {
    super(message);
    this.name = "AgentInvokeError";
    this.status = status;
    this.code = code;
    this.retryable = retryable;
  }
}

export async function invokeAgentTool<T = unknown>(
  orgId: UUID,
  body: {
    tool: string;
    arguments: Record<string, unknown>;
    session_id?: string;
    mode?: AgentChatMode;
  },
): Promise<T> {
  let res: Response;
  try {
    res = await apiFetch(`/api/v1/organizations/${orgId}/agent-tools/invoke`, {
      method: "POST",
      json: body,
    });
  } catch (err) {
    throw new AgentInvokeError(
      (err as Error)?.message || "network error while invoking tool",
      0,
      "network",
      true,
    );
  }
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    const payload = (() => {
      try {
        return JSON.parse(text) as { error?: string; code?: string };
      } catch {
        return null;
      }
    })();
    const message = payload?.error || text || "tool invoke failed";
    const code = payload?.code || (res.status === 403 ? "forbidden" : "invoke_error");
    const retryable = res.status >= 500 || res.status === 429 || code === "network" || code === "timeout";
    throw new AgentInvokeError(message, res.status, code, retryable);
  }
  const j = (await res.json()) as { result: T };
  return j.result;
}
