import * as Y from "yjs";
import { MonacoBinding } from "y-monaco";
import { useCallback, useEffect, useRef, useState } from "react";
import type * as Monaco from "monaco-editor";
import { getAccessToken, wsUrl } from "../api/http";

export type CollabCursor = {
  lineNumber: number;
  column: number;
};

export type CollabPeer = {
  userId: string;
  name: string;
  cursor?: CollabCursor;
  selection?: {
    startLineNumber: number;
    startColumn: number;
    endLineNumber: number;
    endColumn: number;
  };
};

function u8ToB64(u: Uint8Array): string {
  let s = "";
  u.forEach((b) => {
    s += String.fromCharCode(b);
  });
  return btoa(s);
}

function b64ToU8(b: string): Uint8Array {
  return Uint8Array.from(atob(b), (c) => c.charCodeAt(0));
}

/**
 * Live presence + cursor relay and optional Yjs binding over the space file collab WebSocket.
 * When enableYjs is true, use an uncontrolled Monaco defaultValue (see SpaceFilesPage) so the binding matches file content.
 */
export function useSpaceFileCollab(opts: {
  orgId: string | undefined;
  spaceId: string | undefined;
  fileId: string | null;
  selfUserId: string | undefined;
  displayName: string;
  editor: Monaco.editor.IStandaloneCodeEditor | null;
  monaco: typeof Monaco | null;
  enableYjs: boolean;
  /** Wait until file text has loaded before attaching Yjs (avoids empty-doc races). */
  yjsContentReady: boolean;
}) {
  const [peers, setPeers] = useState<Record<string, CollabPeer>>({});
  const wsRef = useRef<WebSocket | null>(null);
  const ydocRef = useRef<Y.Doc | null>(null);
  const bindingRef = useRef<MonacoBinding | null>(null);
  const cursorTimerRef = useRef<number | null>(null);

  useEffect(() => {
    if (!opts.orgId || !opts.spaceId || !opts.fileId || !opts.selfUserId) {
      return;
    }
    const token = getAccessToken();
    if (!token) return;

    const path = `/api/v1/organizations/${opts.orgId}/spaces/${opts.spaceId}/files/collab/ws?token=${encodeURIComponent(token)}&file_id=${encodeURIComponent(opts.fileId)}&name=${encodeURIComponent(opts.displayName)}`;
    const ws = new WebSocket(wsUrl(path));
    wsRef.current = ws;

    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data as string) as Record<string, unknown>;
        const type = msg.type as string;
        const uid = msg.user_id as string | undefined;
        if (!uid || uid === opts.selfUserId) return;

        if (type === "presence") {
          const event = msg.event as string | undefined;
          if (event === "leave") {
            setPeers((prev) => {
              const next = { ...prev };
              delete next[uid];
              return next;
            });
            return;
          }
          const name = (msg.name as string) || "Peer";
          const cursor = msg.cursor as CollabCursor | undefined;
          const selection = msg.selection as CollabPeer["selection"] | undefined;
          setPeers((prev) => ({
            ...prev,
            [uid]: { userId: uid, name, cursor, selection },
          }));
        } else if (type === "yjs_update" && opts.enableYjs && ydocRef.current) {
          const b64 = msg.update as string | undefined;
          if (!b64) return;
          try {
            Y.applyUpdate(ydocRef.current, b64ToU8(b64), "remote");
          } catch {
            /* ignore corrupt */
          }
        }
      } catch {
        /* ignore */
      }
    };

    return () => {
      ws.close();
      if (wsRef.current === ws) wsRef.current = null;
    };
  }, [opts.orgId, opts.spaceId, opts.fileId, opts.selfUserId, opts.displayName, opts.enableYjs]);

  const sendJSON = useCallback((obj: object) => {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(obj));
    }
  }, []);

  useEffect(() => {
    if (!opts.editor || !opts.fileId) return;
    const editor = opts.editor;
    const flush = () => {
      cursorTimerRef.current = null;
      const pos = editor.getPosition();
      const sel = editor.getSelection();
      if (!pos || !sel) return;
      sendJSON({
        type: "presence",
        cursor: { lineNumber: pos.lineNumber, column: pos.column },
        selection: {
          startLineNumber: sel.startLineNumber,
          startColumn: sel.startColumn,
          endLineNumber: sel.endLineNumber,
          endColumn: sel.endColumn,
        },
      });
    };
    const sub = editor.onDidChangeCursorPosition(() => {
      if (cursorTimerRef.current != null) return;
      cursorTimerRef.current = window.setTimeout(flush, 120);
    });
    return () => {
      sub.dispose();
      if (cursorTimerRef.current != null) window.clearTimeout(cursorTimerRef.current);
    };
  }, [opts.editor, opts.fileId, sendJSON]);

  useEffect(() => {
    if (!opts.enableYjs || !opts.editor || !opts.monaco || !opts.fileId || !opts.yjsContentReady) {
      return;
    }
    const model = opts.editor.getModel();
    if (!model) return;

    const ydoc = new Y.Doc();
    ydocRef.current = ydoc;
    const ytext = ydoc.getText("monaco");
    const seed = model.getValue();
    if (seed.length > 0) {
      ytext.insert(0, seed);
    }

    const binding = new MonacoBinding(ytext, model, new Set([opts.editor]), null);
    bindingRef.current = binding;

    const onUpdate = (update: Uint8Array, origin: unknown) => {
      if (origin === "remote") return;
      sendJSON({ type: "yjs_update", update: u8ToB64(update) });
    };
    ydoc.on("update", onUpdate);

    return () => {
      ydoc.off("update", onUpdate);
      binding.destroy();
      bindingRef.current = null;
      ydoc.destroy();
      if (ydocRef.current === ydoc) ydocRef.current = null;
    };
  }, [opts.enableYjs, opts.editor, opts.monaco, opts.fileId, opts.yjsContentReady, sendJSON]);

  return { peers, sendJSON };
}
