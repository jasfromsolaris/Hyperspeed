import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { getAccessToken, wsUrl } from "../api/http";
import type { RealtimeEnvelope, Task } from "../api/types";
import {
  notificationRecipientUserId,
  notificationRecordFromPayload,
} from "../lib/realtimeNotificationPayload";
import { chatDebug, isChatDebugEnabled } from "../chatDebug";

export function useOrgRealtime(orgId: string | undefined, enabled: boolean) {
  const qc = useQueryClient();
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!orgId || !enabled) {
      return;
    }
    const token = getAccessToken();
    if (!token) {
      if (isChatDebugEnabled()) {
        console.warn("[Hyperspeed chat] org realtime skipped: no access token");
      }
      return;
    }
    const url = wsUrl(`/api/v1/organizations/${orgId}/ws?token=${encodeURIComponent(token)}`);
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      chatDebug("WebSocket open", { orgId });
    };
    ws.onerror = () => {
      console.warn("[Hyperspeed chat] WebSocket error (check network / API / Redis)");
    };
    ws.onclose = (ev) => {
      chatDebug("WebSocket closed", { code: ev.code, reason: ev.reason || undefined });
    };

    ws.onmessage = (ev) => {
      try {
        const env = JSON.parse(ev.data as string) as RealtimeEnvelope;
        if (env.type?.startsWith("chat.")) {
          chatDebug("realtime", env.type, env.project_id, env.payload);
        }
        if (env.type?.startsWith("task.")) {
          const pid = env.project_id;
          if (pid) {
            void qc.invalidateQueries({ queryKey: ["tasks", pid] });
            void qc.invalidateQueries({ queryKey: ["board"] });
            void qc.invalidateQueries({ queryKey: ["my-tasks"] });
          }
          if (env.type === "task.message.created" && pid) {
            const p = env.payload as { task_id?: string } | undefined;
            if (p?.task_id) {
              void qc.invalidateQueries({
                queryKey: ["task-messages", pid, p.task_id],
              });
            }
          }
        }
        if (env.type === "project.updated") {
          void qc.invalidateQueries({ queryKey: ["projects", orgId] });
        }
        if (env.type?.startsWith("chat.")) {
          const pid = env.project_id;
          const payload = env.payload as { chat_room_id?: string } | undefined;
          const roomId = payload?.chat_room_id;
          if (roomId) {
            void qc.invalidateQueries({ queryKey: ["chat-messages", roomId] });
            void qc.invalidateQueries({ queryKey: ["chat-search", roomId] });
          }
          if (pid) {
            void qc.invalidateQueries({ queryKey: ["chat-rooms", pid] });
          }
        }
        if (env.type?.startsWith("notification.")) {
          void qc.invalidateQueries({ queryKey: ["notifications", orgId] });
          void qc.invalidateQueries({ queryKey: ["notifications-unread", orgId] });

          const userId = notificationRecipientUserId(env.payload);
          if (userId) {
            window.dispatchEvent(
              new CustomEvent("hs:notification", {
                detail: {
                  orgId,
                  userId,
                  notification: notificationRecordFromPayload(env.payload),
                },
              }),
            );
          }
        }
        if (env.type === "task.updated" || env.type === "task.created") {
          const t = env.payload as Task;
          if (t?.id && env.project_id) {
            qc.setQueryData<Task | undefined>(["task", env.project_id, t.id], t);
          }
        }
      } catch (parseErr) {
        chatDebug("failed to parse WebSocket message", parseErr);
      }
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [orgId, enabled, qc]);
}
