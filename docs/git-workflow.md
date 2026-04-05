# Git-oriented workflow (without embedding VS Code)

Hyperspeed spaces store files in object storage with metadata in Postgres. For teams that want Git-style portability without a managed bare repository in v1:

## Export / import zip

- In **Space → Files**, use **Export zip** to download the full file tree (plus `manifest.json` listing paths and sizes).
- Use **Import zip** to upload a `.zip` and recreate folders/files under the current folder (or space root if you are at the root). Paths inside the archive must not contain `..` (zip-slip safe).

Size and entry limits are enforced server-side to keep imports predictable.

## External Git hosting (manual)

1. Export a zip (or clone files via your own automation against the API).
2. Initialize a repository locally, commit, and push to GitHub/GitLab/etc.
3. To bring changes back, zip the tree (preserving relative paths) and use **Import zip**, or script uploads via the existing file APIs.

A future iteration can add stored `git_remote_url` plus a worker/CI pull-push flow; that is intentionally out of scope for the first milestone.
