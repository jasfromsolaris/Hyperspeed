import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { getAccessToken, wsUrl } from "../api/http";
import type { RealtimeEnvelope, Task } from "../api/types";
import {
  notificationRecipientUserId,
  notificationRecordFromPayload,
} from "../lib/realtimeNotificationPayload";

export function useMultiOrgRealtime(orgIds: string[], enabled: boolean) {
  const qc = useQueryClient();
  const socketsRef = useRef<Map<string, WebSocket>>(new Map());

  useEffect(() => {
    if (!enabled) {
      for (const ws of socketsRef.current.values()) {
        ws.close();
      }
      socketsRef.current.clear();
      return;
    }
    const token = getAccessToken();
    if (!token) {
      return;
    }

    const MAX_SOCKETS = 20;
    const wantList = orgIds.filter(Boolean).slice(0, MAX_SOCKETS);
    const want = new Set(wantList);

    const attachHandlers = (oid: string, ws: WebSocket, wantNow: Set<string>) => {
      ws.onmessage = (ev) => {
        try {
          const env = JSON.parse(ev.data as string) as RealtimeEnvelope;
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
            void qc.invalidateQueries({ queryKey: ["projects", oid] });
          }
          if (env.type === "file.tree.updated") {
            const pid = env.project_id;
            if (pid) {
              void qc.invalidateQueries({ queryKey: ["file-tree", pid] });
              void qc.invalidateQueries({ queryKey: ["file-nodes", pid], exact: false });
              void qc.invalidateQueries({ queryKey: ["file-folders", pid] });
            }
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
            void qc.invalidateQueries({ queryKey: ["notifications", oid] });
            void qc.invalidateQueries({ queryKey: ["notifications-unread", oid] });
            const userId = notificationRecipientUserId(env.payload);
            if (userId) {
              window.dispatchEvent(
                new CustomEvent("hs:notification", {
                  detail: {
                    orgId: oid,
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
        } catch {
          /* ignore */
        }
      };

      let reconnectAttempt = 0;
      ws.onclose = () => {
        // Basic backoff reconnect, only if still desired and enabled.
        if (!enabled) return;
        if (!wantNow.has(oid)) return;
        if (reconnectAttempt >= 5) return;
        const delay = Math.min(30_000, 1000 * Math.pow(2, reconnectAttempt));
        reconnectAttempt++;
        window.setTimeout(() => {
          if (!enabled) return;
          if (!wantNow.has(oid)) return;
          if (socketsRef.current.get(oid) !== ws) return;
          const url2 = wsUrl(
            `/api/v1/organizations/${oid}/ws?token=${encodeURIComponent(token)}`,
          );
          const ws2 = new WebSocket(url2);
          socketsRef.current.set(oid, ws2);
          attachHandlers(oid, ws2, wantNow);
        }, delay);
      };
    };

    // Close sockets for orgs we no longer want.
    for (const [oid, ws] of socketsRef.current.entries()) {
      if (!want.has(oid)) {
        ws.close();
        socketsRef.current.delete(oid);
      }
    }

    // Open sockets for new orgs (capped).
    for (const oid of wantList) {
      if (socketsRef.current.has(oid)) continue;
      const url = wsUrl(`/api/v1/organizations/${oid}/ws?token=${encodeURIComponent(token)}`);
      const ws = new WebSocket(url);
      socketsRef.current.set(oid, ws);
      attachHandlers(oid, ws, want);
    }

    return () => {
      for (const ws of socketsRef.current.values()) {
        ws.close();
      }
      socketsRef.current.clear();
    };
  }, [enabled, qc, orgIds.join("|")]);
}

