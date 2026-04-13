/**
 * Normalizes /organizations/:id/me/permissions query data. The same React Query key is
 * used in the sidebar and settings pages; cache entries must tolerate both legacy shapes
 * (raw string[] vs { permissions: string[] }) so .includes never runs on a non-array.
 */
export function coerceOrgPermissionList(data: unknown): string[] {
  if (Array.isArray(data)) {
    return data.filter((x): x is string => typeof x === "string");
  }
  if (data && typeof data === "object" && "permissions" in data) {
    const p = (data as { permissions: unknown }).permissions;
    if (Array.isArray(p)) {
      return p.filter((x): x is string => typeof x === "string");
    }
  }
  return [];
}
