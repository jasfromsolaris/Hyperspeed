import { apiFetch } from "./http";
import type { Organization, OrganizationsListResponse } from "./types";

export type OrgsQueryData = {
  organizations: Organization[];
  canCreateOrganization: boolean;
};

export async function fetchOrganizationsList(): Promise<OrgsQueryData> {
  const res = await apiFetch("/api/v1/organizations");
  if (!res.ok) {
    throw new Error("orgs");
  }
  const j = (await res.json()) as OrganizationsListResponse;
  return {
    organizations: j.organizations ?? [],
    canCreateOrganization: j.can_create_organization ?? true,
  };
}
