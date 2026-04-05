import type { CSSProperties } from "react";

/** Stable hue 0–359 from a string id (for workspace dots). */
export function orgHue(id: string | null | undefined): number {
  const s = id == null ? "" : String(id);
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = (h * 31 + s.charCodeAt(i)) >>> 0;
  }
  return h % 360;
}

export function orgDotStyle(id: string | null | undefined): CSSProperties {
  return {
    backgroundColor: `hsl(${orgHue(id)} 55% 50%)`,
  };
}
