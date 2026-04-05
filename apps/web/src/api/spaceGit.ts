import { apiFetch } from "./http";

export type SpaceGitLinkDTO = {
  space_id: string;
  remote_url: string;
  branch: string;
  root_folder_id: string | null;
  token_last4: string | null;
  last_commit_sha: string | null;
  last_error: string | null;
  last_sync_at: string | null;
  created_at: string;
  updated_at: string;
  workdir_ready?: boolean;
  local_head_sha?: string | null;
};

export async function getSpaceGitLink(orgId: string, spaceId: string): Promise<{
  git_link: SpaceGitLinkDTO | null;
  git_integration_available: boolean;
}> {
  const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${spaceId}/git`, { method: "GET" });
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || `git get ${res.status}`);
  }
  const j = (await res.json()) as {
    git_link: SpaceGitLinkDTO | null;
    git_integration_available?: boolean;
  };
  return {
    git_link: j.git_link,
    git_integration_available: j.git_integration_available !== false,
  };
}

export async function putSpaceGitLink(
  orgId: string,
  spaceId: string,
  body: {
    remote_url: string;
    branch: string;
    root_folder_id?: string | null;
    access_token?: string;
  },
): Promise<SpaceGitLinkDTO> {
  const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${spaceId}/git`, {
    method: "PUT",
    json: body,
  });
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || `git save ${res.status}`);
  }
  const j = (await res.json()) as { git_link: SpaceGitLinkDTO };
  return j.git_link;
}

export async function deleteSpaceGitLink(orgId: string, spaceId: string): Promise<void> {
  const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${spaceId}/git`, { method: "DELETE" });
  if (!res.ok && res.status !== 404) {
    const t = await res.text();
    throw new Error(t || `git delete ${res.status}`);
  }
}

export async function testSpaceGitRemote(orgId: string, spaceId: string): Promise<void> {
  const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${spaceId}/git/test`, {
    method: "POST",
  });
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || `git test ${res.status}`);
  }
}

export async function pullSpaceGit(orgId: string, spaceId: string): Promise<{ imported_files: number; head_sha: string }> {
  const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${spaceId}/git/pull`, {
    method: "POST",
  });
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || `git pull ${res.status}`);
  }
  return res.json() as Promise<{ imported_files: number; head_sha: string }>;
}

export async function pushSpaceGit(orgId: string, spaceId: string, message: string): Promise<{ head_sha: string }> {
  const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${spaceId}/git/push`, {
    method: "POST",
    json: { message },
  });
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || `git push ${res.status}`);
  }
  return res.json() as Promise<{ head_sha: string }>;
}
