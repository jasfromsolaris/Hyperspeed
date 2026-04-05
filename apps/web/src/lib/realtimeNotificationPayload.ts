/**
 * notification.created WebSocket payloads differ by source:
 * - Chat wraps `{ chat_room_id, payload: { user_id, notification } }`
 * - Tasks/overdue often send flat `{ user_id, notification }`
 */

export function notificationRecipientUserId(payload: unknown): string | undefined {
  if (!payload || typeof payload !== "object") return undefined;
  const o = payload as Record<string, unknown>;
  if (typeof o.user_id === "string") return o.user_id;
  const inner = o.payload;
  if (inner && typeof inner === "object") {
    const u = (inner as Record<string, unknown>).user_id;
    if (typeof u === "string") return u;
  }
  return undefined;
}

export function notificationRecordFromPayload(payload: unknown): unknown {
  if (!payload || typeof payload !== "object") return undefined;
  const o = payload as Record<string, unknown>;
  if ("notification" in o) return o.notification;
  const inner = o.payload;
  if (inner && typeof inner === "object" && "notification" in inner) {
    return (inner as Record<string, unknown>).notification;
  }
  return undefined;
}
