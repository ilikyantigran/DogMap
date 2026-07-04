---
name: feature-pr-flow
description: >-
  The required delivery flow for a DogMap feature: branch off main, implement, then
  open a pull request and get it reviewed by the pr-reviewer agent before it merges —
  and only the repo owner merges. MANUAL ONLY — use this strictly when explicitly
  asked (invoked by name, run as /feature-pr-flow, or told "open a PR and get it
  reviewed" / "follow the PR flow" / "start a new feature"). Do NOT auto-trigger it
  just because a task mentions a PR, a branch, or review; if the user or the
  orchestrator hasn't asked for it, don't apply it. Every feature agent (auth-service,
  profiles-service, map-service, frontend-service) uses this as its start-to-finish
  delivery process.
---

# Feature delivery: branch → PR → review → owner merges

How a feature reaches `main`: never by pushing to `main`, always on a dedicated
branch that becomes a reviewed pull request the **repo owner** merges.

## Branch first (before writing any code)

Every new feature starts on its own branch off the latest `main`:

```bash
git switch main && git pull --ff-only
git switch -c release/<short-name>      # e.g. release/find-by-login
```

- Branch name is **`release/<short-name>`** — lower-kebab, describes the feature.
- Do ALL work on that branch. Never commit feature work to `main`.

## Preconditions to open the PR

- Implemented against the `Docs/*.md` contract (source of truth).
- Tests written and **green** (`go test ./...` per service; `npm test` +
  `npm run build` for frontend).
- `PROGRESS.md` (in the service's own directory) up to date.
- Committed on the `release/<name>` branch.

## Steps

1. **Push the branch.** `git push -u origin release/<name>`.
2. **Open the PR.** `gh pr create --base main --head release/<name> --title "<what>"
   --body ...` — the body states what changed and why, the `Docs/02-Backend.md` (or
   `03-Frontend.md`) section it implements, cross-service contract points others must
   match, and the test evidence (what you ran, that it's green).
3. **Request review from `pr-reviewer`.** Hand the PR (number/URL) to the
   **pr-reviewer** agent. It **posts its findings onto the PR** as review comments
   (inline + a summary carrying the verdict) and returns the same to you. It is
   read-only on code and never merges. (On a solo repo it comments with
   `event=COMMENT`, not approve/request-changes, which GitHub blocks on own PRs.)
4. **Address findings.** Fix every `blocker`/`major`; decide per case on `minor`/`nit`.
   Re-run tests, push updates to the same branch, and re-request review until the
   verdict is **APPROVE**.
5. **Stop at APPROVE — do NOT merge.** Merging into `main` is the **repo owner's**
   decision alone (enforced by branch protection). Leave the PR ready and tell the
   owner it's approved and awaiting their merge.

## Rules

- **The owner merges. Automation and feature agents never merge to `main`.**
- No direct commits/pushes to `main` for feature work — always a `release/<name>` PR.
- The author never approves their own change; the **pr-reviewer** agent reviews, the
  owner decides.
- Never merge over a REQUEST CHANGES.

## Prerequisite: GitHub connected

`gh pr create` needs the GitHub **remote** (`origin`) and an authenticated **`gh` CLI**.
If `gh` isn't installed/authed yet, you can still push the `release/<name>` branch and
get a **local review** — hand `pr-reviewer` the branch and it reviews
`git diff main...release/<name>` — but the real flow is a PR the owner merges.
