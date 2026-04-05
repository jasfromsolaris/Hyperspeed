import { Navigate, Outlet } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";

export function ProtectedRoute() {
  const { state } = useAuth();
  if (state.status === "loading") {
    return (
      <div className="flex min-h-screen items-center justify-center text-muted-foreground">
        Loading…
      </div>
    );
  }
  if (state.status !== "authenticated") {
    return <Navigate to="/login" replace />;
  }
  return <Outlet />;
}
