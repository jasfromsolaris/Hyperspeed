import { apiFetch } from "./http";
import type { ServiceAccount, UUID } from "./types";

export async function listServiceAccounts(orgId: string): Promise<ServiceAccount[]> {
  const res = await apiFetch(`/api/v1/organizations/${orgId}/service-accounts`);
  if (!res.ok) throw new Error("list service accounts");
  const j = (await res.json()) as { service_accounts: ServiceAccount[] };
  return j.service_accounts ?? [];
}

export async function patchServiceAccount(
  orgId: string,
  serviceAccountId: UUID,
  body: {
    provider?: "openrouter" | "cursor";
    openrouter_model?: string;
    cursor_default_repo_url?: string;
    cursor_default_ref?: string;
  },
): Promise<ServiceAccount> {
  const res = await apiFetch(`/api/v1/organizations/${orgId}/service-accounts/${serviceAccountId}`, {
    method: "PATCH",
    json: body,
  });
  if (!res.ok) {
    const j = await res.json().catch(() => ({}));
    throw new Error((j as { error?: string }).error ?? "patch service account");
  }
  const j = (await res.json()) as { service_account: ServiceAccount };
  return j.service_account;
}
