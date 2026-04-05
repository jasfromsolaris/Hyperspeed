import { mergeAttributes, Node } from "@tiptap/core";
import { NodeViewWrapper, ReactNodeViewRenderer } from "@tiptap/react";
import type { NodeViewProps } from "@tiptap/core";
import { useCallback, useEffect, useState } from "react";
import { apiFetch } from "../api/http";

const PRESIGN_REFRESH_MS = 8 * 60 * 1000;

export type HyperspeedImageOptions = {
  orgId: string;
  spaceId: string;
};

function HyperspeedImageView(props: NodeViewProps) {
  const { node, extension } = props;
  const { orgId, spaceId } = extension.options as HyperspeedImageOptions;
  const fileId = node.attrs.fileId as string | null;
  const alt = (node.attrs.alt as string) || "";
  const [src, setSrc] = useState<string | null>(null);
  const [broken, setBroken] = useState(false);

  const refresh = useCallback(async () => {
    if (!orgId || !spaceId || !fileId) return;
    setBroken(false);
    try {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${spaceId}/files/${fileId}/download`,
      );
      if (!res.ok) throw new Error("download");
      const j = (await res.json()) as { download_url?: string };
      if (j.download_url) setSrc(j.download_url);
      else throw new Error("no url");
    } catch {
      setBroken(true);
      setSrc(null);
    }
  }, [orgId, spaceId, fileId]);

  useEffect(() => {
    void refresh();
    const id = window.setInterval(() => void refresh(), PRESIGN_REFRESH_MS);
    const onVis = () => {
      if (document.visibilityState === "visible") void refresh();
    };
    document.addEventListener("visibilitychange", onVis);
    return () => {
      window.clearInterval(id);
      document.removeEventListener("visibilitychange", onVis);
    };
  }, [refresh]);

  return (
    <NodeViewWrapper className="my-2" data-drag-handle="">
      {!fileId ? (
        <p className="text-xs text-muted-foreground">Missing image reference</p>
      ) : broken ? (
        <p className="text-xs text-red-600">Could not load image</p>
      ) : src ? (
        <img
          src={src}
          alt={alt}
          className="max-h-96 max-w-full rounded border border-border object-contain"
        />
      ) : (
        <p className="text-xs text-muted-foreground">Loading image…</p>
      )}
    </NodeViewWrapper>
  );
}

export const HyperspeedImage = Node.create({
  name: "hyperspeedImage",
  group: "block",
  atom: true,
  draggable: true,

  addOptions() {
    return {
      orgId: "",
      spaceId: "",
    } satisfies HyperspeedImageOptions;
  },

  addAttributes() {
    return {
      fileId: {
        default: null,
        parseHTML: (el) => el.getAttribute("data-hyperspeed-file"),
      },
      alt: {
        default: "",
        parseHTML: (el) => el.getAttribute("alt") ?? "",
      },
    };
  },

  parseHTML() {
    return [
      {
        tag: "img[data-hyperspeed-file]",
        getAttrs: (el) => {
          if (!(el instanceof HTMLElement)) return false;
          const id = el.getAttribute("data-hyperspeed-file");
          if (!id) return false;
          return {
            fileId: id,
            alt: el.getAttribute("alt") ?? "",
          };
        },
      },
    ];
  },

  renderHTML({ node }) {
    const fileId = node.attrs.fileId as string | null;
    if (!fileId) return ["div", { class: "hyperspeed-image-missing" }];
    const alt = (node.attrs.alt as string) || "";
    return ["img", mergeAttributes({ "data-hyperspeed-file": fileId }, alt ? { alt } : {})];
  },

  addNodeView() {
    return ReactNodeViewRenderer(HyperspeedImageView);
  },
});
