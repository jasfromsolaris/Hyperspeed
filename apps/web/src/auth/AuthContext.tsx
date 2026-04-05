import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import type { User } from "../api/types";
import { apiFetch, clearTokens, getAccessToken, setTokens } from "../api/http";
import type { TokenResponse } from "../api/types";

export type RegisterPayload = {
  name: string;
  email: string;
  password: string;
  /** Required when this install has no organization yet (CEO bootstrap). */
  organization_name?: string;
  intended_public_url?: string;
};

type AuthState =
  | { status: "loading" }
  | { status: "anonymous" }
  | { status: "authenticated"; user: User };

const AuthCtx = createContext<{
  state: AuthState;
  login: (email: string, password: string) => Promise<void>;
  register: (payload: RegisterPayload) => Promise<{ signupPending: boolean }>;
  logout: () => Promise<void>;
  refreshMe: () => Promise<void>;
} | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: "loading" });

  const refreshMe = useCallback(async () => {
    const token = getAccessToken();
    if (!token) {
      setState({ status: "anonymous" });
      return;
    }
    const res = await apiFetch("/api/v1/me");
    if (!res.ok) {
      clearTokens();
      setState({ status: "anonymous" });
      return;
    }
    const user = (await res.json()) as User;
    setState({ status: "authenticated", user });
  }, []);

  useEffect(() => {
    void refreshMe();
  }, [refreshMe]);

  const login = useCallback(
    async (email: string, password: string) => {
      const res = await apiFetch("/api/v1/auth/login", {
        method: "POST",
        json: { email, password },
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error((err as { error?: string }).error || "Login failed");
      }
      const tok = (await res.json()) as TokenResponse;
      setTokens(tok);
      await refreshMe();
    },
    [refreshMe],
  );

  const register = useCallback(
    async (payload: RegisterPayload) => {
      const json: Record<string, unknown> = {
        name: payload.name,
        email: payload.email,
        password: payload.password,
      };
      if (payload.organization_name?.trim()) {
        json.organization_name = payload.organization_name.trim();
      }
      if (payload.intended_public_url?.trim()) {
        json.intended_public_url = payload.intended_public_url.trim();
      }
      const res = await apiFetch("/api/v1/auth/register", {
        method: "POST",
        json,
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error((err as { error?: string }).error || "Register failed");
      }
      const tok = (await res.json()) as TokenResponse;
      setTokens(tok);
      await refreshMe();
      return { signupPending: !!tok.signup_pending };
    },
    [refreshMe],
  );

  const logout = useCallback(async () => {
    await apiFetch("/api/v1/auth/logout", {
      method: "POST",
      json: { refresh_token: localStorage.getItem("hyperspeed_refresh") },
    }).catch(() => {});
    clearTokens();
    setState({ status: "anonymous" });
  }, []);

  const value = useMemo(
    () => ({ state, login, register, logout, refreshMe }),
    [state, login, register, logout, refreshMe],
  );

  return <AuthCtx.Provider value={value}>{children}</AuthCtx.Provider>;
}

export function useAuth() {
  const v = useContext(AuthCtx);
  if (!v) {
    throw new Error("useAuth outside AuthProvider");
  }
  return v;
}
