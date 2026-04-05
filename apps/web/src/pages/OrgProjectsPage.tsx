import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { apiFetch } from "../api/http";
import type { Organization, Project } from "../api/types";
import { useOrgRealtime } from "../hooks/useOrgRealtime";

export default function OrgProjectsPage() {
  const { orgId } = useParams<{ orgId: string }>();

  useOrgRealtime(orgId, !!orgId);

  const orgQ = useQuery({
    queryKey: ["org", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}`);
      if (!res.ok) {
        throw new Error("org");
      }
      return res.json() as Promise<Organization>;
    },
  });

  const spacesQ = useQuery({
    queryKey: ["projects", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces`);
      if (res.status === 403) {
        return { spaces: [] as Project[], noAccess: true as const };
      }
      if (!res.ok) {
        throw new Error("projects");
      }
      const j = (await res.json()) as { spaces: Project[] };
      return { spaces: j.spaces, noAccess: false as const };
    },
  });

  if (!orgId) {
    return null;
  }

  const spaces = spacesQ.data?.spaces;
  const noAccess = spacesQ.data?.noAccess === true;

  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-background">
      <div className="mx-auto max-w-3xl px-4 py-8">
        <header className="border-b border-border pb-6">
          <p className="text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
            Spaces
          </p>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight text-foreground">
            {orgQ.data?.name ?? "…"}
          </h1>
          <div className="mt-4 flex flex-wrap gap-2">
            <Link
              to={`/o/${orgId}/roles`}
              className="rounded-sm border border-border bg-transparent px-3 py-1.5 text-sm text-foreground hover:bg-accent"
            >
              Roles & permissions
            </Link>
          </div>
          <p className="mt-2 text-sm text-muted-foreground">
            Each space is an empty folder — open it to add task boards, chat rooms,
            and files. Right-click the workspace in the sidebar to create a space.
          </p>
        </header>

        {noAccess ? (
          <div className="mt-10 rounded-sm border border-border bg-card p-4 text-sm text-muted-foreground">
            You don&apos;t have any roles in this workspace yet, so you can&apos;t view
            spaces. Ask an owner to assign you a role under{" "}
            <Link className="text-link underline" to={`/o/${orgId}/roles`}>
              Roles & permissions
            </Link>
            .
          </div>
        ) : (
          <>
            <h2 className="mt-10 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
              Spaces
            </h2>
            <ul className="mt-3 space-y-2">
              {spaces?.map((r) => (
                <li key={r.id}>
                  <Link
                    to={`/o/${orgId}/p/${r.id}`}
                    className="block rounded-sm border border-border bg-card px-4 py-3 transition-colors hover:border-muted-foreground/40"
                  >
                    <div className="font-medium text-card-foreground">
                      <span className="text-muted-foreground">#</span> {r.name}
                    </div>
                    {r.description && (
                      <div className="mt-1 text-sm text-muted-foreground">
                        {r.description}
                      </div>
                    )}
                  </Link>
                </li>
              ))}
            </ul>
            {spaces?.length === 0 && (
              <p className="mt-4 text-sm text-muted-foreground">
                No spaces yet — right-click this workspace in the sidebar and choose
                &quot;Create space&quot;.
              </p>
            )}
          </>
        )}
      </div>
    </div>
  );
}
