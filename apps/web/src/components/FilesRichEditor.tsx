import { Color } from "@tiptap/extension-color";
import { FontFamily } from "@tiptap/extension-font-family";
import { Highlight } from "@tiptap/extension-highlight";
import { Placeholder } from "@tiptap/extension-placeholder";
import { Table } from "@tiptap/extension-table";
import { TableCell } from "@tiptap/extension-table-cell";
import { TableHeader } from "@tiptap/extension-table-header";
import { TableRow } from "@tiptap/extension-table-row";
import { TextAlign } from "@tiptap/extension-text-align";
import { FontSize, TextStyle } from "@tiptap/extension-text-style";
import { EditorContent, useEditor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import {
  AlignCenter,
  AlignLeft,
  AlignRight,
  Bold,
  Highlighter,
  ImagePlus,
  Italic,
  Link2,
  List,
  ListOrdered,
  Minus,
  Palette,
  Quote,
  Redo2,
  Strikethrough,
  Table as TableIcon,
  Underline,
  Undo2,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef } from "react";
import type { UUID } from "../api/types";
import { prepareEditorHtmlFromStorage } from "../lib/richEditorSanitize";
import { HyperspeedImage } from "./tiptapHyperspeedImage";

export type FilesRichEditorProps = {
  fileId: UUID;
  value: string;
  onChange: (html: string) => void;
  disabled?: boolean;
  className?: string;
  orgId: string;
  spaceId: string;
  /** Upload image to space files; return new file node id. */
  onUploadImage: (file: File) => Promise<UUID>;
};

function ToolbarButton({
  onClick,
  active,
  disabled,
  title,
  children,
}: {
  onClick: () => void;
  active?: boolean;
  disabled?: boolean;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      title={title}
      aria-label={title}
      aria-pressed={active ?? false}
      disabled={disabled}
      onClick={onClick}
      className={[
        "inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-sm border border-transparent text-foreground hover:bg-accent",
        active ? "bg-accent" : "",
        disabled ? "pointer-events-none opacity-40" : "",
      ].join(" ")}
    >
      {children}
    </button>
  );
}

export function FilesRichEditor({
  fileId,
  value,
  onChange,
  disabled,
  className,
  orgId,
  spaceId,
  onUploadImage,
}: FilesRichEditorProps) {
  const imageInputRef = useRef<HTMLInputElement>(null);
  const lastEmitted = useRef<string | null>(null);
  /** Avoid pushing empty doc HTML to parent during TipTap destroy (e.g. switching to Monaco). */
  const suppressOnChangeRef = useRef(false);

  const extensions = useMemo(
    () => [
      StarterKit.configure({
        heading: { levels: [1, 2, 3] },
        link: {
          openOnClick: false,
          HTMLAttributes: { rel: "noopener noreferrer", target: "_blank" },
        },
      }),
      TextStyle,
      FontSize,
      FontFamily.configure({
        types: ["textStyle"],
      }),
      Color.configure({ types: ["textStyle"] }),
      Highlight.configure({ multicolor: true }),
      TextAlign.configure({ types: ["heading", "paragraph"] }),
      Table.configure({ resizable: true, allowTableNodeSelection: true }),
      TableRow,
      TableHeader,
      TableCell,
      Placeholder.configure({ placeholder: "Write something…" }),
      HyperspeedImage.configure({ orgId, spaceId }),
    ],
    [orgId, spaceId],
  );

  const initialContent = useMemo(() => prepareEditorHtmlFromStorage(value), [value, fileId]);

  const editor = useEditor(
    {
      extensions,
      content: initialContent,
      editable: !disabled,
      onCreate: () => {
        suppressOnChangeRef.current = false;
      },
      onDestroy: () => {
        suppressOnChangeRef.current = true;
      },
      editorProps: {
        attributes: {
          class: [
            "min-h-[12rem] flex-1 px-3 py-2 text-sm text-foreground outline-none focus:outline-none max-w-none",
            "[&_p]:my-2 [&_ul]:my-2 [&_ol]:my-2 [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5",
            "[&_h1]:text-2xl [&_h1]:font-semibold [&_h1]:my-3",
            "[&_h2]:text-xl [&_h2]:font-semibold [&_h2]:my-2",
            "[&_h3]:text-lg [&_h3]:font-semibold [&_h3]:my-2",
            "[&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground",
            "[&_table]:w-full [&_table]:border-collapse [&_td]:border [&_td]:border-border [&_td]:p-1.5 [&_th]:border [&_th]:border-border [&_th]:p-1.5 [&_th]:bg-muted/40",
          ].join(" "),
        },
      },
      onUpdate: ({ editor: ed }) => {
        if (suppressOnChangeRef.current) return;
        const html = ed.getHTML();
        lastEmitted.current = html;
        onChange(html);
      },
    },
    [extensions, fileId],
  );

  useEffect(() => {
    lastEmitted.current = null;
  }, [fileId]);

  useEffect(() => {
    if (editor) {
      editor.setEditable(!disabled);
    }
  }, [editor, disabled]);

  useEffect(() => {
    if (!editor || editor.isDestroyed) return;
    const prepared = prepareEditorHtmlFromStorage(value);
    const cur = editor.getHTML();
    if (cur === prepared) return;
    if (lastEmitted.current != null && prepared === lastEmitted.current) return;
    editor.commands.setContent(prepared, { emitUpdate: false });
  }, [editor, value]);

  const run = useCallback(
    (fn: () => boolean) => {
      if (!editor || disabled) return;
      fn();
    },
    [editor, disabled],
  );

  const onPickImage = useCallback(
    async (list: FileList | null) => {
      const f = list?.[0];
      if (!f || !editor || disabled) return;
      if (!f.type.startsWith("image/")) return;
      try {
        const nodeId = await onUploadImage(f);
        editor
          .chain()
          .focus()
          .insertContent({
            type: "hyperspeedImage",
            attrs: { fileId: nodeId, alt: f.name },
          })
          .run();
      } catch {
        /* toast optional */
      }
      if (imageInputRef.current) imageInputRef.current.value = "";
    },
    [editor, disabled, onUploadImage],
  );

  if (!editor) {
    return (
      <div className={["flex min-h-[12rem] flex-1 items-center justify-center text-sm text-muted-foreground", className].filter(Boolean).join(" ")}>
        Loading editor…
      </div>
    );
  }

  return (
    <div className={["flex min-h-0 flex-1 flex-col overflow-hidden", className].filter(Boolean).join(" ")}>
      <input
        ref={imageInputRef}
        type="file"
        accept="image/*"
        className="hidden"
        aria-hidden
        onChange={(e) => void onPickImage(e.target.files)}
      />
      <div
        className="flex shrink-0 flex-wrap items-center gap-0.5 border-b border-border bg-card/80 px-1 py-1"
        role="toolbar"
        aria-label="Rich text formatting"
      >
        <select
          className="mr-1 h-8 max-w-[7rem] rounded-sm border border-border bg-background px-1 text-xs text-foreground"
          aria-label="Block type"
          disabled={disabled}
          value={
            editor.isActive("heading", { level: 1 })
              ? "h1"
              : editor.isActive("heading", { level: 2 })
                ? "h2"
                : editor.isActive("heading", { level: 3 })
                  ? "h3"
                  : "p"
          }
          onChange={(e) => {
            const v = e.target.value;
            run(() => {
              if (v === "p") return editor.chain().focus().setParagraph().run();
              if (v === "h1") return editor.chain().focus().toggleHeading({ level: 1 }).run();
              if (v === "h2") return editor.chain().focus().toggleHeading({ level: 2 }).run();
              if (v === "h3") return editor.chain().focus().toggleHeading({ level: 3 }).run();
              return false;
            });
          }}
        >
          <option value="p">Paragraph</option>
          <option value="h1">Heading 1</option>
          <option value="h2">Heading 2</option>
          <option value="h3">Heading 3</option>
        </select>
        <ToolbarButton
          title="Bold"
          active={editor.isActive("bold")}
          disabled={disabled}
          onClick={() => run(() => editor.chain().focus().toggleBold().run())}
        >
          <Bold className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          title="Italic"
          active={editor.isActive("italic")}
          disabled={disabled}
          onClick={() => run(() => editor.chain().focus().toggleItalic().run())}
        >
          <Italic className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          title="Underline"
          active={editor.isActive("underline")}
          disabled={disabled}
          onClick={() => run(() => editor.chain().focus().toggleUnderline().run())}
        >
          <Underline className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          title="Strikethrough"
          active={editor.isActive("strike")}
          disabled={disabled}
          onClick={() => run(() => editor.chain().focus().toggleStrike().run())}
        >
          <Strikethrough className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <span className="mx-1 h-5 w-px bg-border" aria-hidden />
        <ToolbarButton
          title="Bullet list"
          active={editor.isActive("bulletList")}
          disabled={disabled}
          onClick={() => run(() => editor.chain().focus().toggleBulletList().run())}
        >
          <List className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          title="Numbered list"
          active={editor.isActive("orderedList")}
          disabled={disabled}
          onClick={() => run(() => editor.chain().focus().toggleOrderedList().run())}
        >
          <ListOrdered className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          title="Quote"
          active={editor.isActive("blockquote")}
          disabled={disabled}
          onClick={() => run(() => editor.chain().focus().toggleBlockquote().run())}
        >
          <Quote className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <span className="mx-1 h-5 w-px bg-border" aria-hidden />
        <ToolbarButton
          title="Align left"
          active={editor.isActive({ textAlign: "left" })}
          disabled={disabled}
          onClick={() => run(() => editor.chain().focus().setTextAlign("left").run())}
        >
          <AlignLeft className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          title="Align center"
          active={editor.isActive({ textAlign: "center" })}
          disabled={disabled}
          onClick={() => run(() => editor.chain().focus().setTextAlign("center").run())}
        >
          <AlignCenter className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          title="Align right"
          active={editor.isActive({ textAlign: "right" })}
          disabled={disabled}
          onClick={() => run(() => editor.chain().focus().setTextAlign("right").run())}
        >
          <AlignRight className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <span className="mx-1 h-5 w-px bg-border" aria-hidden />
        <ToolbarButton
          title="Link"
          active={editor.isActive("link")}
          disabled={disabled}
          onClick={() => {
            const prev = editor.getAttributes("link").href as string | undefined;
            const url = window.prompt("Link URL", prev ?? "https://");
            if (url === null) return;
            if (url === "") {
              run(() => editor.chain().focus().extendMarkRange("link").unsetLink().run());
              return;
            }
            run(() => editor.chain().focus().extendMarkRange("link").setLink({ href: url }).run());
          }}
        >
          <Link2 className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <label
          className="inline-flex h-8 w-8 cursor-pointer items-center justify-center rounded-sm hover:bg-accent"
          title="Text color"
        >
          <Palette className="h-4 w-4" aria-hidden />
          <input
            type="color"
            className="sr-only"
            aria-label="Text color"
            disabled={disabled}
            onChange={(e) => run(() => editor.chain().focus().setColor(e.target.value).run())}
          />
        </label>
        <label
          className="inline-flex h-8 w-8 cursor-pointer items-center justify-center rounded-sm hover:bg-accent"
          title="Highlight"
        >
          <Highlighter className="h-4 w-4" aria-hidden />
          <input
            type="color"
            className="sr-only"
            aria-label="Highlight color"
            disabled={disabled}
            onChange={(e) =>
              run(() => editor.chain().focus().toggleHighlight({ color: e.target.value }).run())
            }
          />
        </label>
        <select
          className="h-8 max-w-[5.5rem] rounded-sm border border-border bg-background px-1 text-xs"
          aria-label="Font size"
          disabled={disabled}
          onChange={(e) => {
            const px = e.target.value;
            run(() => {
              if (!px) return editor.chain().focus().unsetFontSize().run();
              return editor.chain().focus().setFontSize(px).run();
            });
            e.target.selectedIndex = 0;
          }}
          defaultValue=""
        >
          <option value="">Size</option>
          <option value="12px">12px</option>
          <option value="14px">14px</option>
          <option value="16px">16px</option>
          <option value="18px">18px</option>
          <option value="24px">24px</option>
        </select>
        <select
          className="h-8 max-w-[6.5rem] rounded-sm border border-border bg-background px-1 text-xs"
          aria-label="Font family"
          disabled={disabled}
          onChange={(e) => {
            const fam = e.target.value;
            run(() => {
              if (!fam) return editor.chain().focus().unsetFontFamily().run();
              return editor.chain().focus().setFontFamily(fam).run();
            });
            e.target.selectedIndex = 0;
          }}
          defaultValue=""
        >
          <option value="">Font</option>
          <option value="ui-sans-serif, system-ui, sans-serif">Sans</option>
          <option value="ui-serif, Georgia, serif">Serif</option>
          <option value="ui-monospace, monospace">Mono</option>
        </select>
        <span className="mx-1 h-5 w-px bg-border" aria-hidden />
        <ToolbarButton
          title="Insert table"
          disabled={disabled}
          onClick={() =>
            run(() => editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run())
          }
        >
          <TableIcon className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          title="Horizontal rule"
          disabled={disabled}
          onClick={() => run(() => editor.chain().focus().setHorizontalRule().run())}
        >
          <Minus className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          title="Insert image from space upload"
          disabled={disabled}
          onClick={() => imageInputRef.current?.click()}
        >
          <ImagePlus className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <span className="mx-1 h-5 w-px bg-border" aria-hidden />
        <ToolbarButton
          title="Undo"
          disabled={disabled || !editor.can().undo()}
          onClick={() => run(() => editor.chain().focus().undo().run())}
        >
          <Undo2 className="h-4 w-4" aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          title="Redo"
          disabled={disabled || !editor.can().redo()}
          onClick={() => run(() => editor.chain().focus().redo().run())}
        >
          <Redo2 className="h-4 w-4" aria-hidden />
        </ToolbarButton>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto bg-card">
        <EditorContent editor={editor} className="h-full [&_.ProseMirror]:min-h-[12rem]" />
      </div>
    </div>
  );
}
