import { Navigate, Outlet } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";

/** Keeps users with a pending signup approval out of the main workspace shell. */
export function PendingBlocksApp() {
  const { state } = useAuth();
  if (state.status === "authenticated" && state.user.signup_pending) {
    return <Navigate to="/signup-pending" replace />;
  }
  return <Outlet />;
}
