import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { apiFetch } from "../api/http";
import type { Board, ChatRoom } from "../api/types";

/**
 * Legacy URL `/o/:orgId/p/:projectId` — the space hub UI was removed.
 * Sends the user to the first board, first chat, or the IDE page.
 */
export default function SpaceHomeRedirect() {
  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId: string;
  }>();
  const navigate = useNavigate();

  const boardsQ = useQuery({
    queryKey: ["boards", projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/boards`,
      );
      if (!res.ok) {
        throw new Error("boards");
      }
      const j = (await res.json()) as { boards: Board[] };
      return j.boards;
    },
  });

  const chatsQ = useQuery({
    queryKey: ["chat-rooms", projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/chat-rooms`,
      );
      if (!res.ok) {
        throw new Error("chat rooms");
      }
      const j = (await res.json()) as { chat_rooms: ChatRoom[] };
      return j.chat_rooms;
    },
  });

  useEffect(() => {
    if (!orgId || !projectId) {
      return;
    }
    if (boardsQ.isPending || chatsQ.isPending) {
      return;
    }
    const boards = boardsQ.data ?? [];
    const chats = chatsQ.data ?? [];
    if (boards.length > 0) {
      void navigate(`/o/${orgId}/p/${projectId}/b/${boards[0]!.id}`, {
        replace: true,
      });
      return;
    }
    if (chats.length > 0) {
      void navigate(`/o/${orgId}/p/${projectId}/c/${chats[0]!.id}`, {
        replace: true,
      });
      return;
    }
    void navigate(`/o/${orgId}/p/${projectId}/ide`, { replace: true });
  }, [
    orgId,
    projectId,
    boardsQ.isPending,
    boardsQ.data,
    chatsQ.isPending,
    chatsQ.data,
    navigate,
  ]);

  return <div className="min-h-0 flex-1 bg-background" aria-hidden />;
}
