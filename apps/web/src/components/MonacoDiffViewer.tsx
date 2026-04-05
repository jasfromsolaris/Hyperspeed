import { DiffEditor, type DiffEditorProps, type Monaco } from "@monaco-editor/react";
import { useEffect, useRef } from "react";

type Props = Omit<DiffEditorProps, "originalModelPath" | "modifiedModelPath" | "keepCurrentOriginalModel" | "keepCurrentModifiedModel"> & {
  /**
   * Stable id per diff instance (e.g. proposal id). Used for in-memory model URIs and
   * deferred cleanup so Monaco does not dispose TextModels before the diff widget finishes unmounting.
   */
  instanceKey: string;
};

/**
 * Wraps @monaco-editor/react DiffEditor with model paths + keepCurrent* flags to avoid the
 * "TextModel got disposed before DiffEditorWidget model got reset" crash on unmount/hide.
 */
export function MonacoDiffViewer({ instanceKey, onMount, ...rest }: Props) {
  const monacoRef = useRef<Monaco | null>(null);
  const mountKeyRef = useRef<string | null>(null);
  if (mountKeyRef.current === null) {
    mountKeyRef.current = `${instanceKey}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 9)}`;
  }
  const mk = mountKeyRef.current;
  const originalPath = `inmemory:/hs-diff/${encodeURIComponent(mk)}/original`;
  const modifiedPath = `inmemory:/hs-diff/${encodeURIComponent(mk)}/modified`;

  useEffect(() => {
    return () => {
      const monaco = monacoRef.current;
      monacoRef.current = null;
      const op = originalPath;
      const mp = modifiedPath;
      if (!monaco) return;
      queueMicrotask(() => {
        try {
          monaco.editor.getModel(monaco.Uri.parse(op))?.dispose();
          monaco.editor.getModel(monaco.Uri.parse(mp))?.dispose();
        } catch {
          /* editor or models may already be gone */
        }
      });
    };
  }, [originalPath, modifiedPath]);

  return (
    <DiffEditor
      {...rest}
      originalModelPath={originalPath}
      modifiedModelPath={modifiedPath}
      keepCurrentOriginalModel
      keepCurrentModifiedModel
      onMount={(ed, monaco) => {
        monacoRef.current = monaco;
        onMount?.(ed, monaco);
      }}
    />
  );
}
