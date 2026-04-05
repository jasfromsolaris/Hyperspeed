# Deferred: GitHub OAuth / GitHub App for IDE Git

Interactive OAuth or a GitHub App installation flow is **not** part of the v1 IDE Git integration.

v1 uses **HTTPS remotes** and a **Personal Access Token** (encrypted at rest), configured by an org admin via the Source Control panel.

A future iteration can:

- Replace long-lived PATs with OAuth tokens and refresh flows.
- Offer a repository picker without pasting clone URLs manually.
- Align with enterprise GitHub App policies.

See [ide-git-github.md](./ide-git-github.md) for the current model.
