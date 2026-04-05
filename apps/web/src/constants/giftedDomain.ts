/** Canonical DNS apex for Hyperspeed-hosted gifted subdomains (always *.hyperspeedapp.com). */
export const GIFTED_SUBDOMAIN_APEX = "hyperspeedapp.com";

/** Hostname prefix for team URLs: `www.{team}.hyperspeedapp.com`. */
export const GIFTED_TEAM_WWW_PREFIX = "www.";

const dnsLabel = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;

/**
 * Extract team label from a stored intended URL:
 * `https://www.{label}.hyperspeedapp.com` (canonical) or legacy `https://{label}.hyperspeedapp.com`.
 */
export function parseTeamSubdomainFromIntendedUrl(
  url: string | null | undefined,
): string {
  if (!url?.trim()) return "";
  try {
    const u = new URL(url.trim());
    const host = u.hostname.toLowerCase();
    const suffix = `.${GIFTED_SUBDOMAIN_APEX}`;
    if (!host.endsWith(suffix)) return "";
    const withoutApex = host.slice(0, -suffix.length);
    if (!withoutApex) return "";
    if (withoutApex.startsWith(GIFTED_TEAM_WWW_PREFIX)) {
      const inner = withoutApex.slice(GIFTED_TEAM_WWW_PREFIX.length);
      if (!inner || inner.includes(".")) return "";
      return dnsLabel.test(inner) ? inner : "";
    }
    if (!withoutApex.includes(".")) {
      return dnsLabel.test(withoutApex) ? withoutApex : "";
    }
    return "";
  } catch {
    return "";
  }
}

/** Single-label DNS rules for team subdomain (lowercase). Returns null if empty or invalid. */
export function intendedUrlFromTeamSubdomain(
  subdomain: string,
): string | null {
  const raw = subdomain.trim().toLowerCase();
  if (!raw) return null;
  if (!dnsLabel.test(raw)) return null;
  return `https://${GIFTED_TEAM_WWW_PREFIX}${raw}.${GIFTED_SUBDOMAIN_APEX}`;
}

/** Sanitize input while typing (lowercase, allowed chars only, max label length). */
export function sanitizeTeamSubdomainInput(raw: string): string {
  return raw.toLowerCase().replace(/[^a-z0-9-]/g, "").slice(0, 63);
}
