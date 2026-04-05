/** Value for `<input type="datetime-local" />` from an API ISO timestamp. */
export function toDatetimeLocalValue(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** RFC3339 for API, or `null` when the field is empty (no due date). */
export function fromDatetimeLocalToISO(s: string): string | null {
  const t = s.trim();
  if (!t) return null;
  const d = new Date(t);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString();
}

export function isTaskOverdue(dueAt: string | null | undefined): boolean {
  if (!dueAt) return false;
  const t = new Date(dueAt).getTime();
  if (Number.isNaN(t)) return false;
  return t < Date.now();
}
