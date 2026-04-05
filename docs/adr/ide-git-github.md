# ADR: IDE Git / GitHub integration

## Status

Accepted — initial implementation (v1).

## Context

Hyperspeed stores space files in Postgres metadata plus object storage, not as a native Git working tree. The IDE exposes a Source Control surface that must bridge Git remotes (typically GitHub via HTTPS + PAT) and Hyperspeed’s file model.

## Decision

### Canonical source of truth (v1)

- **Working copy on disk** (under `HS_GIT_WORKDIR_BASE`) is an implementation detail for `git` CLI operations.
- **Hyperspeed file nodes + object storage** are the **canonical editor state** for day-to-day work.
- **Pull** overwrites matching paths under the configured **root folder** from the remote checkout into Hyperspeed (import).
- **Push** exports the current tree under that folder into the workdir, then `git commit` + `git push` to the configured branch.

This is closest to option **B** (Hyperspeed-first) with explicit **pull** to reconcile from remote when needed. Full two-way merge/conflict UI is **out of scope** for v1.

### Branch model

- One **tracked branch** per space link (default `main`). Configurable in the link record.

### Authentication (v1)

- **HTTPS only**; remote URL must be an `https://` Git URL.
- **Personal Access Token (PAT)** stored **encrypted** (AES-GCM via `HS_SSH_ENCRYPTION_KEY`, same pipeline as other org secrets). Last 4 characters shown for confirmation.
- **GitHub OAuth / GitHub App** is deferred (see “Follow-ups”).

### Conflict / safety

- Push runs `git pull --rebase` is **not** required in v1; we **fetch + reset hard** to `origin/<branch>` before applying exported files only when preparing a fresh mirror for commit—actually v1 push flow: clone or open existing repo, **wipe non-`.git` paths**, write HS tree, `git add -A`, commit, `git push`. If push fails (non-fast-forward), return error and ask user to **Pull** first.
- Pull **imports** remote files; existing nodes with same relative path get **content replaced** (same node ID where path matches).

### Operational

- Server-side `git` binary required at runtime (Alpine runtime image includes `git`).
- Workdirs are per-space: `{GIT_WORKDIR_BASE}/{spaceID}/repo`.

### Cursor Cloud Agents vs IDE Git (bridge)

- **Per-space** `space_git_links` (IDE Source Control) and **per–service-account** `cursor_default_repo_url` (Cursor-backed AI staff) are separate configuration surfaces.
- **Launch-time resolution:** When a Cursor staff mention triggers Cloud Agents, the server uses **`cursor_default_repo_url` on the service account if set**; otherwise it uses this space’s **IDE Git remote URL** from `space_git_links` when present. **Git ref** uses `cursor_default_ref` if set, else the space link’s **branch**, else `main`.
- If neither an explicit default repo nor a space Git remote exists, the run fails with a clear error (configure one or the other).
- **Optional UI:** Org admins can **copy** the current space remote + branch onto a Cursor service account (Organization → Service accounts, or the shortcut in IDE Source Control) so org settings stay in sync; this is not auto-synced on every link edit.

## Consequences

- Operators must provision disk for `HS_GIT_WORKDIR_BASE` (Compose volume in production).
- Tokens are high sensitivity; audit and rate-limit endpoints in future hardening passes.

## Follow-ups

- GitHub OAuth App or GitHub App installation for org-scoped tokens.
- Shallow clone tuning, garbage collection of abandoned workdirs.
