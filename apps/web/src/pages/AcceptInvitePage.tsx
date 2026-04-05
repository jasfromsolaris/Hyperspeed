import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { apiFetch } from "../api/http";

export default function AcceptInvitePage() {
  const nav = useNavigate();
  const [token, setToken] = useState("");

  const accept = useMutation({
    mutationFn: async () => {
      const t = token.trim();
      const res = await apiFetch(`/api/v1/invites/${encodeURIComponent(t)}/accept`, {
        method: "POST",
        json: {},
      });
      if (!res.ok) throw new Error("accept");
      return res.json() as Promise<{ organization_id: string }>;
    },
    onSuccess: (j) => {
      void nav(`/o/${j.organization_id}`);
    },
  });

  return (
    <div className="min-h-0 flex-1 bg-background px-4 py-10">
      <div className="mx-auto max-w-md rounded-sm border border-border bg-card p-6">
        <Link to="/" className="text-xs text-link hover:underline">
          ← Home
        </Link>
        <p className="mt-3 text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
          Invite
        </p>
        <h1 className="mt-1 text-xl font-semibold tracking-tight text-foreground">
          Accept workspace invite
        </h1>
        <form
          className="mt-4 space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (!token.trim()) return;
            accept.mutate();
          }}
        >
          <input
            className="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm text-foreground"
            placeholder="Paste invite token"
            value={token}
            onChange={(e) => setToken(e.target.value)}
          />
          <button
            type="submit"
            disabled={accept.isPending}
            className="w-full rounded-sm bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
          >
            Accept invite
          </button>
          {accept.isError && (
            <p className="text-sm text-destructive">
              Failed to accept invite. Make sure you’re signed in and the token is valid.
            </p>
          )}
        </form>
      </div>
    </div>
  );
}

