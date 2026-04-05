import { useEffect, useMemo, useState } from "react";

/**
 * Phase 1 embedded preview: blob URL in a sandboxed iframe.
 * Phase 2: optional `serverUrl` — cross-origin API-hosted static snapshot (`?token=`).
 */

export type IdeHtmlPreviewProps = {
  html: string;
  /** When false, the iframe is unmounted and any blob URL is revoked. */
  visible: boolean;
  className?: string;
  /** Phase 1 (default): blob from `html`. Phase 2: set `serverUrl` and `mode="server"`. */
  mode?: "blob" | "server";
  /** Phase 2: full preview URL including token (from POST /preview/sessions). */
  serverUrl?: string | null;
  serverLoading?: boolean;
  serverError?: string | null;
  /** Fill parent flex column (IDE preview tab) instead of a fixed viewport band. */
  fillHeight?: boolean;
};

const BLOB_SANDBOX = "allow-scripts allow-same-origin";
/** Cross-origin API preview: scripts only; framed document origin is the API host. */
const SERVER_SANDBOX = "allow-scripts allow-forms";

export function IdeHtmlPreview({
  html,
  visible,
  className,
  mode = "blob",
  serverUrl,
  serverLoading,
  serverError,
  fillHeight = false,
}: IdeHtmlPreviewProps) {
  const [objectUrl, setObjectUrl] = useState<string | null>(null);
  const [refreshNonce, setRefreshNonce] = useState(0);

  const serverIframeSrc = useMemo(() => {
    if (!serverUrl) return null;
    try {
      const u = new URL(serverUrl);
      u.searchParams.set("_", String(refreshNonce));
      return u.toString();
    } catch {
      const sep = serverUrl.includes("?") ? "&" : "?";
      return `${serverUrl}${sep}_=${refreshNonce}`;
    }
  }, [serverUrl, refreshNonce]);

  useEffect(() => {
    if (mode === "server") {
      setObjectUrl((prev) => {
        if (prev) URL.revokeObjectURL(prev);
        return null;
      });
      return;
    }
    if (!visible) {
      setObjectUrl((prev) => {
        if (prev) URL.revokeObjectURL(prev);
        return null;
      });
      return;
    }
    const blob = new Blob([html], { type: "text/html;charset=utf-8" });
    const next = URL.createObjectURL(blob);
    setObjectUrl((prev) => {
      if (prev) URL.revokeObjectURL(prev);
      return next;
    });
    return () => {
      URL.revokeObjectURL(next);
    };
  }, [html, visible, refreshNonce, mode]);

  if (!visible) {
    return null;
  }

  const iframeFrame = fillHeight
    ? "min-h-0 w-full flex-1 border-0 bg-background"
    : "h-[min(50vh,28rem)] min-h-[12rem] w-full flex-1 border-0 bg-background";
  const loadingFrame = fillHeight
    ? "flex min-h-0 flex-1 items-center justify-center"
    : "flex min-h-[12rem] items-center justify-center";
  const rootBorder = fillHeight ? "border-0" : "border-t border-border";

  if (mode === "server") {
    return (
      <div
        className={[
          "flex min-h-0 flex-col bg-card",
          fillHeight ? "min-h-0 flex-1" : "shrink-0",
          rootBorder,
          className ?? "",
        ].join(" ")}
        data-testid="ide-html-preview"
      >
        <div className="flex items-center justify-between gap-2 border-b border-border px-2 py-1.5">
          <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Preview (server)
          </span>
          <div className="flex items-center gap-2">
            <span className="text-[11px] text-muted-foreground">Space snapshot</span>
            <button
              type="button"
              className="rounded-sm px-2 py-0.5 text-xs text-foreground hover:bg-accent"
              onClick={() => setRefreshNonce((n) => n + 1)}
              disabled={!serverUrl || serverLoading}
            >
              Refresh
            </button>
          </div>
        </div>
        {serverLoading ? (
          <div className={`text-sm text-muted-foreground ${loadingFrame}`}>Starting preview…</div>
        ) : serverError ? (
          <div
            className={[
              "whitespace-pre-wrap p-3 text-sm text-red-600",
              fillHeight ? "min-h-0 flex-1 overflow-auto" : "min-h-[12rem]",
            ].join(" ")}
          >
            {serverError}
          </div>
        ) : serverIframeSrc ? (
          <iframe
            title="HTML preview (server)"
            className={iframeFrame}
            src={serverIframeSrc}
            sandbox={SERVER_SANDBOX}
            referrerPolicy="no-referrer"
          />
        ) : (
          <div className={fillHeight ? "min-h-0 flex-1 p-3 text-sm text-muted-foreground" : "min-h-[12rem] p-3 text-sm text-muted-foreground"}>
            No preview URL.
          </div>
        )}
      </div>
    );
  }

  if (!objectUrl) {
    return null;
  }

  return (
    <div
      className={[
        "flex min-h-0 flex-col bg-card",
        fillHeight ? "min-h-0 flex-1" : "shrink-0 border-t border-border",
        className ?? "",
      ].join(" ")}
      data-testid="ide-html-preview"
    >
      <div className="flex shrink-0 items-center justify-between gap-2 border-b border-border px-2 py-1.5">
        <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Preview</span>
        <button
          type="button"
          className="rounded-sm px-2 py-0.5 text-xs text-foreground hover:bg-accent"
          onClick={() => setRefreshNonce((n) => n + 1)}
        >
          Refresh
        </button>
      </div>
      <iframe title="HTML preview" className={iframeFrame} src={objectUrl} sandbox={BLOB_SANDBOX} />
    </div>
  );
}
