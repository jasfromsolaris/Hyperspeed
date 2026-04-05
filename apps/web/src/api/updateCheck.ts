import type { PublicInstance } from "./instance";
import semver from "semver";

export type LatestUpdateInfo = {
  latestVersion: string;
  releaseNotesUrl: string | null;
  upgradeGuideUrl: string | null;
};

type Manifest = {
  version: string;
  release_notes_url?: string;
  upgrade_guide_url?: string;
};

/** Fetch latest release info from static manifest (wins) or GitHub API. */
export async function fetchLatestUpdateInfo(
  inst: PublicInstance,
): Promise<LatestUpdateInfo | null> {
  const manifestUrl = inst.update_manifest_url?.trim();
  if (manifestUrl) {
    const res = await fetch(manifestUrl, {
      method: "GET",
      headers: { Accept: "application/json" },
    });
    if (!res.ok) {
      return null;
    }
    const j = (await res.json()) as Manifest;
    const v = typeof j.version === "string" ? j.version.trim() : "";
    if (!v) {
      return null;
    }
    return {
      latestVersion: v,
      releaseNotesUrl: j.release_notes_url?.trim() || null,
      upgradeGuideUrl: j.upgrade_guide_url?.trim() || null,
    };
  }

  const repo = inst.upstream_github_repo?.trim();
  if (!repo) {
    return null;
  }
  const parts = repo.split("/").filter(Boolean);
  if (parts.length !== 2) {
    return null;
  }
  const [owner, name] = parts;
  const url = `https://api.github.com/repos/${encodeURIComponent(owner)}/${encodeURIComponent(name)}/releases/latest`;
  const res = await fetch(url, {
    method: "GET",
    headers: {
      Accept: "application/vnd.github+json",
    },
  });
  if (!res.ok) {
    return null;
  }
  const j = (await res.json()) as {
    tag_name?: string;
    html_url?: string;
  };
  const tag = typeof j.tag_name === "string" ? j.tag_name.trim() : "";
  if (!tag) {
    return null;
  }
  const stripped = tag.replace(/^v/i, "");
  return {
    latestVersion: stripped,
    releaseNotesUrl: typeof j.html_url === "string" ? j.html_url : null,
    upgradeGuideUrl: null,
  };
}

/** True if remote is strictly newer than current (semver). */
export function isNewerThanCurrent(
  current: string | undefined,
  latest: string,
): boolean {
  if (!current || current === "dev") {
    return false;
  }
  const a = semver.coerce(current);
  const b = semver.coerce(latest);
  if (!a || !b) {
    return false;
  }
  return semver.gt(b, a);
}
