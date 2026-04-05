import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { listServiceAccounts, patchServiceAccount } from "../api/serviceAccounts";
import {
  deleteSpaceGitLink,
  getSpaceGitLink,
  pullSpaceGit,
  pushSpaceGit,
  putSpaceGitLink,
  testSpaceGitRemote,
} from "../api/spaceGit";
import type { UUID } from "../api/types";

type Props = {
  orgId: string;
  spaceId: string;
};

export function IdeSourceControlPanel({ orgId, spaceId }: Props) {
  const qc = useQueryClient();
  const [remoteUrl, setRemoteUrl] = useState("");
  const [branch, setBranch] = useState("main");
  const [accessToken, setAccessToken] = useState("");
  const [rootFolderId, setRootFolderId] = useState("");
  const [pushMessage, setPushMessage] = useState("Sync from Hyperspeed");
  const [formError, setFormError] = useState<string | null>(null);
  const [cursorStaffId, setCursorStaffId] = useState<UUID | "">("");

  const q = useQuery({
    queryKey: ["space-git", orgId, spaceId],
    queryFn: () => getSpaceGitLink(orgId, spaceId),
    retry: false,
  });

  const saQ = useQuery({
    queryKey: ["service-accounts", orgId],
    queryFn: () => listServiceAccounts(orgId),
    enabled: !!orgId,
  });
  const cursorStaff = useMemo(
    () => (saQ.data ?? []).filter((s) => s.provider === "cursor"),
    [saQ.data],
  );

  const link = q.data?.git_link ?? null;
  const integrationAvailable = q.data?.git_integration_available !== false;
  const disabledRemote = !integrationAvailable;

  useEffect(() => {
    if (!link) {
      setRemoteUrl("");
      setBranch("main");
      setRootFolderId("");
      return;
    }
    setRemoteUrl(link.remote_url);
    setBranch(link.branch || "main");
    setRootFolderId(link.root_folder_id ?? "");
  }, [link?.space_id, link?.updated_at]);

  useEffect(() => {
    if (cursorStaff.length === 0) {
      setCursorStaffId("");
      return;
    }
    setCursorStaffId((prev) => {
      if (prev && cursorStaff.some((s) => s.id === prev)) return prev;
      return cursorStaff[0]!.id;
    });
  }, [cursorStaff]);

  const copyToCursorStaffM = useMutation({
    mutationFn: async () => {
      if (!link?.remote_url?.trim()) throw new Error("No remote URL to copy");
      const sid = cursorStaffId || cursorStaff[0]?.id;
      if (!sid) throw new Error("No Cursor service account");
      await patchServiceAccount(orgId, sid, {
        provider: "cursor",
        cursor_default_repo_url: link.remote_url.trim(),
        cursor_default_ref: (link.branch || "main").trim() || "main",
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["service-accounts", orgId] });
      setFormError(null);
    },
    onError: (e: Error) => setFormError(e.message),
  });

  const saveM = useMutation({
    mutationFn: async () => {
      setFormError(null);
      await putSpaceGitLink(orgId, spaceId, {
        remote_url: remoteUrl.trim(),
        branch: branch.trim() || "main",
        access_token: accessToken.trim() || undefined,
        root_folder_id: rootFolderId.trim() ? rootFolderId.trim() : null,
      });
    },
    onSuccess: () => {
      setAccessToken("");
      void qc.invalidateQueries({ queryKey: ["space-git", orgId, spaceId] });
    },
    onError: (e: Error) => setFormError(e.message),
  });

  const delM = useMutation({
    mutationFn: () => deleteSpaceGitLink(orgId, spaceId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["space-git", orgId, spaceId] }),
  });

  const testM = useMutation({
    mutationFn: () => testSpaceGitRemote(orgId, spaceId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["space-git", orgId, spaceId] }),
  });

  const pullM = useMutation({
    mutationFn: () => pullSpaceGit(orgId, spaceId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["space-git", orgId, spaceId] });
      void qc.invalidateQueries({ queryKey: ["file-nodes", spaceId] });
      void qc.invalidateQueries({ queryKey: ["file-tree", spaceId] });
    },
  });

  const pushM = useMutation({
    mutationFn: () => pushSpaceGit(orgId, spaceId, pushMessage.trim() || "Sync from Hyperspeed"),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["space-git", orgId, spaceId] }),
  });

  if (q.isLoading) {
    return (
      <div className="flex items-center gap-2 p-3 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading…
      </div>
    );
  }

  if (disabledRemote) {
    return (
      <div className="rounded-sm border border-border bg-card p-3 text-sm text-muted-foreground">
        <p className="font-medium text-foreground">Git disabled</p>
        <p className="mt-2">
          The API must set <code className="text-xs">HS_GIT_WORKDIR_BASE</code> and ship the{" "}
          <code className="text-xs">git</code> binary. See <code className="text-xs">docs/adr/ide-git-github.md</code>.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3 rounded-sm border border-border bg-card p-3 text-sm">
      <p className="font-medium text-foreground">Git / GitHub</p>
      <p className="text-xs text-muted-foreground">
        HTTPS remotes only. Use a GitHub personal access token with <code className="text-[11px]">repo</code> scope.
        Saving the link requires org admin. Pull/Push need file write permission.
      </p>

      {link ? (
        <div className="space-y-1 rounded-sm bg-muted/30 px-2 py-1.5 text-xs text-muted-foreground">
          <div>
            <span className="text-foreground">Remote:</span> {link.remote_url}
          </div>
          <div>
            <span className="text-foreground">Branch:</span> {link.branch}
          </div>
          {link.token_last4 ? (
            <div>
              <span className="text-foreground">Token:</span> …{link.token_last4}
            </div>
          ) : null}
          {link.last_commit_sha ? (
            <div className="truncate" title={link.last_commit_sha}>
              <span className="text-foreground">Last sync SHA:</span> {link.last_commit_sha.slice(0, 12)}…
            </div>
          ) : null}
          {link.local_head_sha ? (
            <div className="truncate">
              <span className="text-foreground">Local HEAD:</span> {link.local_head_sha.slice(0, 12)}…
            </div>
          ) : null}
          {link.last_error ? <div className="text-red-600">Error: {link.last_error}</div> : null}
        </div>
      ) : null}

      {link?.remote_url?.trim() && integrationAvailable ? (
        <div className="space-y-2 rounded-sm border border-border bg-muted/20 p-2">
          <p className="text-xs font-medium text-foreground">Cursor Cloud Agents</p>
          <p className="text-[11px] text-muted-foreground">
            Copy this space’s remote and branch onto a Cursor staff profile so org settings match IDE Git (optional;
            agent runs also resolve from the space link when the profile has no default repo).
          </p>
          {saQ.isLoading ? (
            <p className="text-xs text-muted-foreground">Loading staff accounts…</p>
          ) : cursorStaff.length === 0 ? (
            <p className="text-xs text-amber-700 dark:text-amber-500">
              No Cursor service accounts in this workspace. Create one under Organization → Service accounts.
            </p>
          ) : (
            <>
              <label className="block text-[11px] font-medium text-muted-foreground">Staff profile</label>
              <select
                className="w-full rounded-sm border border-input bg-background px-2 py-1.5 text-xs text-foreground"
                value={cursorStaffId}
                onChange={(e) => setCursorStaffId(e.target.value as UUID)}
              >
                {cursorStaff.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </select>
              <button
                type="button"
                className="rounded-sm border border-border bg-background px-2 py-1.5 text-xs font-medium hover:bg-accent disabled:opacity-50"
                disabled={copyToCursorStaffM.isPending || !cursorStaffId}
                onClick={() => copyToCursorStaffM.mutate()}
              >
                {copyToCursorStaffM.isPending ? (
                  <Loader2 className="inline h-3 w-3 animate-spin" />
                ) : null}{" "}
                Set as Cursor staff default (copy to profile)
              </button>
            </>
          )}
        </div>
      ) : null}

      <div className="space-y-2">
        <label className="block text-xs font-medium text-muted-foreground">Remote URL (https)</label>
        <input
          className="w-full rounded-sm border border-input bg-background px-2 py-1.5 text-xs text-foreground"
          value={remoteUrl}
          onChange={(e) => setRemoteUrl(e.target.value)}
          placeholder="https://github.com/org/repo.git"
          spellCheck={false}
        />
        <label className="block text-xs font-medium text-muted-foreground">Branch</label>
        <input
          className="w-full rounded-sm border border-input bg-background px-2 py-1.5 text-xs text-foreground"
          value={branch}
          onChange={(e) => setBranch(e.target.value)}
          placeholder="main"
        />
        <label className="block text-xs font-medium text-muted-foreground">Access token (new or rotate)</label>
        <input
          type="password"
          className="w-full rounded-sm border border-input bg-background px-2 py-1.5 text-xs text-foreground"
          value={accessToken}
          onChange={(e) => setAccessToken(e.target.value)}
          placeholder={link ? "(unchanged if empty)" : "required"}
          autoComplete="off"
        />
        <label className="block text-xs font-medium text-muted-foreground">Root folder ID (optional)</label>
        <input
          className="w-full rounded-sm border border-input bg-background px-2 py-1.5 text-xs text-foreground"
          value={rootFolderId}
          onChange={(e) => setRootFolderId(e.target.value)}
          placeholder="Sync under this folder only"
          spellCheck={false}
        />
      </div>

      {formError ? <p className="text-xs text-red-600">{formError}</p> : null}

      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          className="rounded-sm bg-primary px-2 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          disabled={saveM.isPending}
          onClick={() => saveM.mutate()}
        >
          {saveM.isPending ? <Loader2 className="inline h-3 w-3 animate-spin" /> : null} Save link
        </button>
        {link ? (
          <button
            type="button"
            className="rounded-sm border border-border px-2 py-1.5 text-xs hover:bg-accent"
            disabled={delM.isPending}
            onClick={() => delM.mutate()}
          >
            Remove
          </button>
        ) : null}
        {link ? (
          <button
            type="button"
            className="rounded-sm border border-border px-2 py-1.5 text-xs hover:bg-accent"
            disabled={testM.isPending}
            onClick={() => testM.mutate()}
          >
            {testM.isPending ? <Loader2 className="inline h-3 w-3 animate-spin" /> : null} Test
          </button>
        ) : null}
      </div>

      {link ? (
        <>
          <div className="border-t border-border pt-2">
            <label className="block text-xs font-medium text-muted-foreground">Commit message (push)</label>
            <input
              className="mt-1 w-full rounded-sm border border-input bg-background px-2 py-1.5 text-xs text-foreground"
              value={pushMessage}
              onChange={(e) => setPushMessage(e.target.value)}
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              className="rounded-sm border border-border px-2 py-1.5 text-xs hover:bg-accent disabled:opacity-50"
              disabled={pullM.isPending}
              onClick={() => pullM.mutate()}
            >
              {pullM.isPending ? <Loader2 className="inline h-3 w-3 animate-spin" /> : null} Pull from remote
            </button>
            <button
              type="button"
              className="rounded-sm border border-border px-2 py-1.5 text-xs hover:bg-accent disabled:opacity-50"
              disabled={pushM.isPending}
              onClick={() => pushM.mutate()}
            >
              {pushM.isPending ? <Loader2 className="inline h-3 w-3 animate-spin" /> : null} Push to remote
            </button>
          </div>
        </>
      ) : null}
    </div>
  );
}
