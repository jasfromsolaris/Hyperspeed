# Version metadata and optional update notices

The API reports its build **version** and **git SHA** on `GET /health` and `GET /api/v1/public/instance`. Operators inject these at **image build time** (see [`apps/api/Dockerfile`](../../apps/api/Dockerfile) `VERSION` / `GIT_SHA` build args).

## Optional: in-app “new version available”

The dashboard can show a **non-intrusive banner** only when:

1. The server exposes **either** `UPDATE_MANIFEST_URL` **or** `UPSTREAM_GITHUB_REPO` on `GET /api/v1/public/instance`, and  
2. The user **opts in** on the Dashboard (“Check for updates…”). Until then, the browser does not fetch GitHub or your manifest.

Opt-in is stored in `localStorage` in this browser only.

### Environment variables (API)

| Variable | Role |
|----------|------|
| `UPSTREAM_GITHUB_REPO` | Optional `owner/name`. The SPA may call `GET https://api.github.com/repos/owner/name/releases/latest` (unauthenticated; [rate limit](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api) applies, typically 60 requests/hour per egress IP). |
| `UPDATE_MANIFEST_URL` | Optional HTTPS URL to a **static JSON** file. If set, it **takes precedence** over GitHub. |

### Manifest JSON schema

When `UPDATE_MANIFEST_URL` is set, the response must be JSON:

```json
{
  "version": "1.2.0",
  "release_notes_url": "https://github.com/org/repo/releases/tag/v1.2.0",
  "upgrade_guide_url": "https://example.com/docs/upgrade"
}
```

- `version` (required): semver-compatible string compared to the running API `version`.  
- `release_notes_url` (optional): shown as “Release notes”.  
- `upgrade_guide_url` (optional): shown as “Upgrade guide”.

Host the file on any HTTPS origin you control (object storage, GitHub raw URL, Pages, etc.). Ensure **CORS** allows `GET` from your app’s browser origin if the manifest is on a **different** host than the API (same-origin deployments often avoid CORS issues by proxying).

### Comparison rules

The UI uses **semver** coercion. If either side does not parse, no banner is shown. The running build string `dev` does not trigger “update available” notices.

### Upgrading the deployment

Updating is still **operator-driven** (pull new images, `docker compose up --build`, etc.). See [README_SELF_HOST.md](../../README_SELF_HOST.md).
