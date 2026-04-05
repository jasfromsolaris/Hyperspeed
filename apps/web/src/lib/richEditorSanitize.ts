import DOMPurify from "dompurify";

/** True if stored content is already HTML (or contains hyperspeed image refs). */
export function looksLikeRichHtml(content: string): boolean {
  const s = content.trim();
  if (!s) return false;
  if (s.startsWith("<")) return true;
  if (s.includes("data-hyperspeed-file=")) return true;
  if (/<\s*\/?\s*(p|div|br|ul|ol|li|h[1-6]|table|blockquote|span|strong)\b/i.test(s)) return true;
  return false;
}

export function escapeHtmlPlain(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/** API string → safe HTML for TipTap: legacy plain text becomes a paragraph; HTML is sanitized. */
export function prepareEditorHtmlFromStorage(raw: string): string {
  const trimmed = raw ?? "";
  if (!looksLikeRichHtml(trimmed)) {
    if (!trimmed) return "<p></p>";
    return `<p>${escapeHtmlPlain(trimmed).replace(/\n/g, "<br>")}</p>`;
  }
  return sanitizeRichHtml(trimmed);
}

export function sanitizeRichHtml(html: string): string {
  return DOMPurify.sanitize(html, {
    USE_PROFILES: { html: true },
    ADD_ATTR: ["data-hyperspeed-file", "target", "rel", "colspan", "rowspan"],
    ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto|tel):|[^a-z]|[a-z+.-]+(?:[^a-z+.\-:]|$))/i,
  });
}
