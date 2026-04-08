import { useQuery } from "@tanstack/react-query";
import { FormEvent, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { fetchPublicInstance } from "../api/instance";

export default function RegisterPage() {
  const { register } = useAuth();
  const nav = useNavigate();
  const [step, setStep] = useState(0);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [organizationName, setOrganizationName] = useState("");
  const [intendedUrl, setIntendedUrl] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const instQ = useQuery({
    queryKey: ["public-instance"],
    queryFn: fetchPublicInstance,
  });
  const inst = instQ.data;

  const isBootstrap = inst?.needs_organization_setup !== false;

  const bootstrapSteps = 5;
  const staffSteps = 1;

  const maxStep = useMemo(
    () => (isBootstrap ? bootstrapSteps - 1 : staffSteps - 1),
    [isBootstrap],
  );

  function mapRegisterError(code: string): string {
    switch (code) {
      case "signups_disabled":
        return "Open registration is disabled on this instance. Ask a workspace admin for an invite.";
      case "signup_already_pending":
        return "You already have a pending access request. Sign in or use a different email.";
      case "organization_name required for the first account":
        return "Workspace name is required for the first account on this server.";
      case "single_org_exists":
        return "This instance already has a workspace. Ask an admin for an invite.";
      default:
        return code || "Register failed";
    }
  }

  async function submitStaff(e: FormEvent) {
    e.preventDefault();
    setErr(null);
    setLoading(true);
    try {
      const { signupPending } = await register({
        name,
        email,
        password,
      });
      nav(signupPending ? "/signup-pending" : "/");
    } catch (ex) {
      setErr(
        ex instanceof Error ? mapRegisterError(ex.message) : "Register failed",
      );
    } finally {
      setLoading(false);
    }
  }

  async function finishBootstrap(e: FormEvent) {
    e.preventDefault();
    setErr(null);
    const intendedTrim = intendedUrl.trim();
    if (
      intendedTrim &&
      !intendedTrim.toLowerCase().startsWith("http://") &&
      !intendedTrim.toLowerCase().startsWith("https://")
    ) {
      setErr("Use a full URL that starts with http:// or https://.");
      return;
    }
    setLoading(true);
    try {
      const { signupPending } = await register({
        name,
        email,
        password,
        organization_name: organizationName,
        intended_public_url: intendedTrim || undefined,
      });
      if (signupPending) {
        nav("/signup-pending");
        return;
      }
      nav("/");
    } catch (ex) {
      setErr(
        ex instanceof Error ? mapRegisterError(ex.message) : "Register failed",
      );
    } finally {
      setLoading(false);
    }
  }

  if (instQ.isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center text-muted-foreground">
        Loading…
      </div>
    );
  }

  if (instQ.isError) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center px-4">
        <p className="text-sm text-destructive">Could not load instance info.</p>
        <Link className="mt-4 text-sm text-link" to="/login">
          Back to sign in
        </Link>
      </div>
    );
  }

  // --- Staff: single form ---
  if (!isBootstrap) {
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
            Create account
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Request access to your team&apos;s workspace. An admin may need to
            approve your account.
          </p>
          <form className="mt-6 space-y-4" onSubmit={submitStaff}>
            <div>
              <label className="block text-xs font-medium text-muted-foreground">
                Name
              </label>
              <input
                className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
                autoComplete="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
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
                Password (min 8)
              </label>
              <input
                className="mt-1 w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
                type="password"
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                minLength={8}
                required
              />
            </div>
            {err && <p className="text-sm text-destructive">{err}</p>}
            <button
              type="submit"
              disabled={loading}
              className="w-full rounded-sm bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-50"
            >
              {loading ? "Creating…" : "Register"}
            </button>
          </form>
          <p className="mt-4 text-center text-sm text-muted-foreground">
            Have an account?{" "}
            <Link className="text-link hover:underline" to="/login">
              Sign in
            </Link>
          </p>
        </div>
      </div>
    );
  }

  // --- CEO bootstrap wizard ---
  const origin =
    typeof window !== "undefined" ? window.location.origin : "";
  const suggested =
    inst?.public_app_url?.trim() || "";

  return (
    <div className="flex min-h-screen flex-col items-center justify-center px-4 py-10">
      <div className="mb-8 text-center">
        <div className="mx-auto mb-3 h-1 w-12 rounded-full bg-brand" aria-hidden />
        <p className="text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
          Hyperspeed
        </p>
      </div>
      <div className="w-full max-w-md rounded-sm border border-border bg-card p-8">
        <h1 className="text-2xl font-semibold tracking-tight text-card-foreground">
          Set up your workspace
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Step {step + 1} of {bootstrapSteps}
        </p>

        {step === 0 && (
          <div className="mt-6 space-y-4">
            <label className="block text-xs font-medium text-muted-foreground">
              Your name
            </label>
            <input
              className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
              autoComplete="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>
        )}

        {step === 1 && (
          <div className="mt-6 space-y-4">
            <label className="block text-xs font-medium text-muted-foreground">
              Email
            </label>
            <input
              className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
              type="email"
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
            />
          </div>
        )}

        {step === 2 && (
          <div className="mt-6 space-y-4">
            <label className="block text-xs font-medium text-muted-foreground">
              Password (min 8)
            </label>
            <input
              className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              minLength={8}
              required
            />
          </div>
        )}

        {step === 3 && (
          <div className="mt-6 space-y-4">
            <label className="block text-xs font-medium text-muted-foreground">
              Workspace name
            </label>
            <input
              className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
              placeholder="e.g. Acme Engineering"
              value={organizationName}
              onChange={(e) => setOrganizationName(e.target.value)}
              required
            />
            <p className="text-xs text-muted-foreground">
              This becomes your organization in Hyperspeed. You can rename it
              later in settings.
            </p>
          </div>
        )}

        {step === 4 && (
          <form className="mt-6 space-y-4" onSubmit={finishBootstrap}>
            <div className="rounded-sm border border-border bg-muted/30 p-3 text-sm text-muted-foreground">
              <p className="font-medium text-foreground">Current browser URL</p>
              <p className="mt-1 font-mono text-xs break-all">{origin}</p>
              <p className="mt-3 text-xs">
                On first install this is often localhost or a LAN IP. That is
                fine — you can move to a public hostname later and set{" "}
                <code className="rounded bg-muted px-1">CORS_ORIGIN</code> /{" "}
                <code className="rounded bg-muted px-1">PUBLIC_API_BASE_URL</code>{" "}
                to match.
              </p>
            </div>

            {suggested ? (
              <div className="text-xs text-muted-foreground">
                <span className="font-medium text-foreground">Suggested public URL: </span>
                <span className="font-mono">{suggested}</span>
              </div>
            ) : null}

            <div>
              <label className="block text-xs font-medium text-muted-foreground">
                Intended team URL (optional)
              </label>
              <div className="mt-1">
                <input
                  className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground outline-none ring-ring ring-offset-2 ring-offset-background focus-visible:ring-2"
                  placeholder="https://app.example.com"
                  value={intendedUrl}
                  onChange={(e) => setIntendedUrl(e.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
              <p className="mt-1 text-xs text-muted-foreground">
                For your notes when DNS and TLS are ready. See{" "}
                <span className="font-mono text-xs">
                  docs/ops/custom-domains-and-subdomains.md
                </span>{" "}
                in the repository.
              </p>
            </div>

            {err && <p className="text-sm text-destructive">{err}</p>}

            <div className="flex flex-wrap gap-2 pt-2">
              <button
                type="button"
                className="rounded-sm border border-border px-4 py-2 text-sm"
                disabled={loading}
                onClick={() => setStep(3)}
              >
                Back
              </button>
              <button
                type="submit"
                disabled={loading}
                className="rounded-sm bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
              >
                {loading ? "Creating workspace…" : "Create workspace & continue"}
              </button>
            </div>
          </form>
        )}

        {step < 4 && (
          <div className="mt-8 flex flex-wrap justify-between gap-2">
            <button
              type="button"
              className="rounded-sm border border-border px-4 py-2 text-sm"
              disabled={step === 0 || loading}
              onClick={() => setStep((s) => Math.max(0, s - 1))}
            >
              Back
            </button>
            <button
              type="button"
              className="rounded-sm bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
              disabled={loading}
              onClick={() => {
                if (step === 0 && !name.trim()) return;
                if (step === 1 && !email.trim()) return;
                if (step === 2 && password.length < 8) return;
                if (step === 3 && !organizationName.trim()) return;
                setStep((s) => Math.min(maxStep, s + 1));
              }}
            >
              Next
            </button>
          </div>
        )}

        <p className="mt-6 text-center text-sm text-muted-foreground">
          Have an account?{" "}
          <Link className="text-link hover:underline" to="/login">
            Sign in
          </Link>
        </p>
      </div>
    </div>
  );
}
