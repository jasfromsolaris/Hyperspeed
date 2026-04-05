import { describe, expect, it } from "vitest";
import {
  escapeHtmlPlain,
  looksLikeRichHtml,
  prepareEditorHtmlFromStorage,
  sanitizeRichHtml,
} from "./richEditorSanitize";

describe("richEditorSanitize", () => {
  it("detects stored HTML vs plain text", () => {
    expect(looksLikeRichHtml("hello")).toBe(false);
    expect(looksLikeRichHtml("<p>x</p>")).toBe(true);
    expect(looksLikeRichHtml('text <img data-hyperspeed-file="id" />')).toBe(true);
  });

  it("escapes plain text for paragraph wrapper", () => {
    expect(escapeHtmlPlain("<b>")).toBe("&lt;b&gt;");
  });

  it("wraps legacy plain in a paragraph with newlines as br", () => {
    expect(prepareEditorHtmlFromStorage("a\nb")).toBe("<p>a<br>b</p>");
  });

  it("sanitizes script tags from HTML", () => {
    const dirty = '<p>ok</p><script>alert(1)</script><img data-hyperspeed-file="n1" alt="x" />';
    const clean = sanitizeRichHtml(dirty);
    expect(clean).toContain("ok");
    expect(clean.toLowerCase()).not.toContain("script");
    expect(clean).toContain("data-hyperspeed-file");
  });
});
