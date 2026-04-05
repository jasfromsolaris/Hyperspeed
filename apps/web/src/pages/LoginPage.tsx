import { FormEvent, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";

export default function LoginPage() {
  const { login } = useAuth();
  const nav = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setErr(null);
    setLoading(true);
    try {
      await login(email, password);
      nav("/");
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Error");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-screen flex-col items-center justify-center px-4">
      <div className="mb-8 text-center">
        <div className="mx-auto mb-3 h-1 w-12 rounded-full bg-brand" aria-hidden />
        <p className="text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
          Hyperspeed
        </p>
      </div>
      <div className="w-full max-w-sm rounded-sm border border-border bg-card p-8">
        <h1 className="text-2xl font-semibold tracking-tight text-card-foreground">
          Sign in
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Continue to your workspaces
        </p>
        <form className="mt-6 space-y-4" onSubmit={onSubmit}>
          <div>
            <label className="block text-xs font-medium text-muted-foreground">
              Email
            </label>
            <input
              className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
              type="email"
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-muted-foreground">
              Password
            </label>
            <input
              className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          {err && <p className="text-sm text-destructive">{err}</p>}
          <button
            type="submit"
            disabled={loading}
            className="w-full rounded-sm bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-50"
          >
            {loading ? "Signing in…" : "Sign in"}
          </button>
        </form>
        <p className="mt-4 text-center text-sm text-muted-foreground">
          No account?{" "}
          <Link className="text-link hover:underline" to="/register">
            Register
          </Link>
        </p>
      </div>
    </div>
  );
}
