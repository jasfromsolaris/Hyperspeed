import { apiFetch } from "./http";

export type PreviewSessionDTO = {
  id: string;
  status: string;
  preview_url: string;
  expires_at: string;
  error_message?: string;
  command?: string;
  cwd?: string;
};

export async function createPreviewSession(
  orgId: string,
  spaceId: string,
  body?: { command?: string; cwd?: string },
): Promise<PreviewSessionDTO> {
  const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${spaceId}/preview/sessions`, {
    method: "POST",
    json: body ?? {},
  });
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || `preview session ${res.status}`);
  }
  const j = (await res.json()) as { session: PreviewSessionDTO };
  return j.session;
}

export async function getPreviewSession(
  orgId: string,
  spaceId: string,
  sessionId: string,
): Promise<PreviewSessionDTO> {
  const res = await apiFetch(
    `/api/v1/organizations/${orgId}/spaces/${spaceId}/preview/sessions/${sessionId}`,
    { method: "GET" },
  );
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || `get preview ${res.status}`);
  }
  const j = (await res.json()) as { session: PreviewSessionDTO };
  return j.session;
}

export async function deletePreviewSession(orgId: string, spaceId: string, sessionId: string): Promise<void> {
  const res = await apiFetch(
    `/api/v1/organizations/${orgId}/spaces/${spaceId}/preview/sessions/${sessionId}`,
    { method: "DELETE" },
  );
  if (!res.ok && res.status !== 404) {
    const t = await res.text();
    throw new Error(t || `delete preview ${res.status}`);
  }
}
