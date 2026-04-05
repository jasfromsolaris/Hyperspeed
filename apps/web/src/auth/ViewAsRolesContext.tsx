import { createContext, ReactNode, useContext, useEffect, useMemo, useState } from "react";

import type { UUID } from "../api/types";

type ViewAsState = {
  enabled: boolean;
  roleIds: UUID[];
};

type ViewAsByOrg = Record<string, ViewAsState | undefined>;

type Ctx = {
  get: (orgId: string) => ViewAsState;
  set: (orgId: string, next: ViewAsState) => void;
  clear: (orgId: string) => void;
};

const DEFAULT_STATE: ViewAsState = { enabled: false, roleIds: [] };

export function viewAsStorageKey(orgId: string) {
  return `hs:viewAsRoles:${orgId}`;
}

export function parseViewAsState(json: string | null): ViewAsState | null {
  if (!json) return null;
  try {
    const v = JSON.parse(json) as Partial<ViewAsState>;
    const enabled = v.enabled === true;
    const roleIds = Array.isArray(v.roleIds) ? (v.roleIds.filter(Boolean) as UUID[]) : [];
    return { enabled, roleIds };
  } catch {
    return null;
  }
}

export function readPersistedViewAsState(orgId: string): ViewAsState | null {
  return parseViewAsState(localStorage.getItem(viewAsStorageKey(orgId)));
}

const ViewAsRolesContext = createContext<Ctx | null>(null);

export function ViewAsRolesProvider({ children }: { children: ReactNode }) {
  const [byOrg, setByOrg] = useState<ViewAsByOrg>({});

  const ctx = useMemo<Ctx>(() => {
    return {
      get: (orgId: string) => byOrg[orgId] ?? DEFAULT_STATE,
      set: (orgId: string, next: ViewAsState) => {
        setByOrg((prev) => ({ ...prev, [orgId]: next }));
        try {
          localStorage.setItem(viewAsStorageKey(orgId), JSON.stringify(next));
        } catch {
          // ignore
        }
      },
      clear: (orgId: string) => {
        setByOrg((prev) => ({ ...prev, [orgId]: DEFAULT_STATE }));
        try {
          localStorage.removeItem(viewAsStorageKey(orgId));
        } catch {
          // ignore
        }
      },
    };
  }, [byOrg]);

  // Lazy-load state per org on first read via explicit helper hook; this effect is a no-op.
  // Kept intentionally empty to avoid eagerly scanning localStorage.
  useEffect(() => {}, []);

  return <ViewAsRolesContext.Provider value={ctx}>{children}</ViewAsRolesContext.Provider>;
}

export function useViewAsRoles(orgId: string | null | undefined) {
  const c = useContext(ViewAsRolesContext);
  if (!c) throw new Error("useViewAsRoles must be used within ViewAsRolesProvider");

  const cur = useMemo(() => {
    if (!orgId) return DEFAULT_STATE;
    return c.get(orgId);
  }, [c, orgId]);

  const persisted = useMemo(() => {
    if (!orgId) return null;
    return readPersistedViewAsState(orgId);
  }, [orgId]);

  // Never update state during render; hydrate after paint.
  useEffect(() => {
    if (!orgId) return;
    if (cur !== DEFAULT_STATE) return;
    if (!persisted) return;
    c.set(orgId, persisted);
  }, [c, cur, orgId, persisted]);

  const state = cur !== DEFAULT_STATE ? cur : persisted ?? DEFAULT_STATE;

  return {
    state,
    set: (next: ViewAsState) => {
      if (!orgId) return;
      c.set(orgId, next);
    },
    clear: () => {
      if (!orgId) return;
      c.clear(orgId);
    },
  };
}

export function useViewAsRolesController() {
  const c = useContext(ViewAsRolesContext);
  if (!c) throw new Error("useViewAsRolesController must be used within ViewAsRolesProvider");

  return {
    getState: (orgId: string): ViewAsState => {
      const cur = c.get(orgId);
      if (cur === DEFAULT_STATE) {
        const persisted = readPersistedViewAsState(orgId);
        if (persisted) return persisted;
      }
      return cur;
    },
  };
}

