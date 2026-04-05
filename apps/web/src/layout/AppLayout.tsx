import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FormEvent, useEffect, useRef, useState } from "react";
import { Outlet } from "react-router-dom";
import { apiFetch } from "../api/http";
import { fetchOrganizationsList } from "../api/orgs";
import { UpdateNoticeBanner } from "../components/UpdateNoticeBanner";
import { useAuth } from "../auth/AuthContext";
import { useMultiOrgRealtime } from "../hooks/useMultiOrgRealtime";
import { usePresencePing } from "../hooks/usePresencePing";
import { AppSidebar } from "./AppSidebar";

export function AppLayout() {
  const { state, refreshMe } = useAuth();
  const qc = useQueryClient();
  const [open, setOpen] = useState(true);
  const [name, setName] = useState("");
  const [toast, setToast] = useState<string | null>(null);
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const myUserId = state.status === "authenticated" ? state.user.id : null;

  usePresencePing(state.status === "authenticated");

  const orgsQ = useQuery({
    queryKey: ["orgs"],
    enabled: state.status === "authenticated",
    queryFn: fetchOrganizationsList,
  });

  useMultiOrgRealtime(
    (orgsQ.data?.organizations ?? []).map((o) => o.id),
    state.status === "authenticated",
  );

  useEffect(() => {
    if (!myUserId) return;
    function onNotif(ev: Event) {
      const ce = ev as CustomEvent<{ orgId?: string; userId?: string }>;
      const userId = ce.detail?.userId;
      if (!userId || userId !== myUserId) return;
      setToast("You were mentioned");
    }
    window.addEventListener("hs:notification", onNotif);
    return () => window.removeEventListener("hs:notification", onNotif);
  }, [myUserId]);

  useEffect(() => {
    if (!toast) return;
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
    toastTimerRef.current = setTimeout(() => {
      toastTimerRef.current = null;
      setToast(null);
    }, 2500);
    return () => {
      if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
      toastTimerRef.current = null;
    };
  }, [toast]);

  const needsName = (() => {
    if (state.status !== "authenticated") {
      return false;
    }
    const email = state.user.email || "";
    const defaultName = email.includes("@") ? email.split("@")[0] : email;
    const dn = state.user.display_name?.trim() || "";
    // Prompt if missing OR still using the auto-filled default (email prefix).
    return !dn || (defaultName && dn.toLowerCase() === defaultName.toLowerCase());
  })();

  const saveName = useMutation({
    mutationFn: async (displayName: string) => {
      const res = await apiFetch("/api/v1/me", {
        method: "PATCH",
        json: { display_name: displayName.trim() },
      });
      if (!res.ok) {
        const e = await res.json().catch(() => ({}));
        throw new Error((e as { error?: string }).error || "Update failed");
      }
      return res.json() as Promise<unknown>;
    },
    onSuccess: async () => {
      await refreshMe();
      void qc.invalidateQueries();
      setOpen(false);
      setName("");
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!name.trim()) {
      return;
    }
    saveName.mutate(name.trim());
  }

  return (
    <div className="flex h-full min-h-0 max-h-full flex-row overflow-hidden bg-background">
      <AppSidebar />
      <main className="relative z-0 flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        {toast ? (
          <div className="pointer-events-none fixed left-1/2 top-3 z-[300] -translate-x-1/2">
            <div className="rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground shadow-md">
              {toast}
            </div>
          </div>
        ) : null}
        <UpdateNoticeBanner />
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <Outlet />
        </div>
      </main>

      {needsName && open && (
        <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/60 px-4">
          <div className="w-full max-w-sm rounded-sm border border-border bg-card p-4 shadow-lg">
            <h2 className="text-sm font-semibold text-card-foreground">
              Set your name
            </h2>
            <p className="mt-1 text-xs text-muted-foreground">
              This is what others will see in chat and members lists.
            </p>
            <form onSubmit={onSubmit} className="mt-4 space-y-3">
              <input
                autoFocus
                className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
                placeholder="Your name"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
              {saveName.isError ? (
                <p className="text-xs text-destructive">
                  {saveName.error instanceof Error ? saveName.error.message : "Error"}
                </p>
              ) : null}
              <div className="flex justify-end gap-2">
                <button
                  type="button"
                  className="rounded-sm border border-border px-3 py-1.5 text-sm hover:bg-accent"
                  onClick={() => setOpen(false)}
                >
                  Not now
                </button>
                <button
                  type="submit"
                  disabled={saveName.isPending || !name.trim()}
                  className="rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                >
                  Save
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
