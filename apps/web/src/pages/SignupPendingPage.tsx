import { Link, Navigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";

export default function SignupPendingPage() {
  const { state, logout, refreshMe } = useAuth();
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
  if (!state.user.signup_pending) {
    return <Navigate to="/" replace />;
  }
  return (
    <div className="flex min-h-screen flex-col items-center justify-center px-4">
      <div className="w-full max-w-md rounded-sm border border-border bg-card p-8 text-center">
        <h1 className="text-xl font-semibold text-foreground">Approval pending</h1>
        <p className="mt-3 text-sm text-muted-foreground">
          Your account is waiting for a workspace administrator to approve access. You’ll be able
          to use Hyperspeed once they approve your request.
        </p>
        <p className="mt-4 text-xs text-muted-foreground">
          Signed in as <span className="font-mono text-foreground">{state.user.email}</span>
        </p>
        <div className="mt-6 flex flex-wrap justify-center gap-3">
          <button
            type="button"
            className="rounded-sm border border-border bg-background px-4 py-2 text-sm text-foreground hover:bg-accent"
            onClick={() => void refreshMe()}
          >
            Refresh status
          </button>
          <button
            type="button"
            className="rounded-sm border border-border bg-background px-4 py-2 text-sm text-foreground hover:bg-accent"
            onClick={() => void logout()}
          >
            Sign out
          </button>
          <Link
            to="/login"
            className="rounded-sm border border-border px-4 py-2 text-sm text-link hover:underline"
          >
            Use a different account
          </Link>
        </div>
      </div>
    </div>
  );
}
