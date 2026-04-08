import { apiFetch } from "./http";

export type PublicInstance = {
  /** True when the database has no organization yet (CEO bootstrap). */
  needs_organization_setup?: boolean;
  /** Operator-configured public browser origin for onboarding hints. */
  public_app_url?: string;
  /** API build version (semver or tag). */
  version?: string;
  git_sha?: string;
  /** Optional "owner/name" for GitHub release lookup. */
  upstream_github_repo?: string;
  /** Optional HTTPS URL to a static JSON manifest (see docs). */
  update_manifest_url?: string;
};

export async function fetchPublicInstance(): Promise<PublicInstance> {
  const res = await apiFetch("/api/v1/public/instance");
  if (!res.ok) {
    throw new Error("instance");
  }
  return res.json() as Promise<PublicInstance>;
}
