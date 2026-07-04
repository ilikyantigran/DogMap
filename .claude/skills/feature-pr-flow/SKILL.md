---
name: feature-pr-flow
description: >-
  The required delivery flow for a DogMap feature: after the work is implemented and
  green, open a pull request and get it reviewed by the pr-reviewer agent BEFORE it
  merges. MANUAL ONLY — use this strictly when explicitly asked (invoked by name, run
  as /feature-pr-flow, or told "open a PR and get it reviewed" / "follow the PR flow").
  Do NOT auto-trigger it just because a task mentions a PR, a branch, or review; if the
  user or the orchestrator hasn't asked for it, don't apply it. Every feature agent
  (auth-service, profiles-service, map-service, frontend-service) uses this as its final
  delivery step.
---

# Feature delivery: PR + review

This is how a completed feature reaches `main`: never by pushing straight to `main`,
always via a reviewed pull request. The reviewer is the **pr-reviewer** agent.

## Preconditions (don't open a PR until all true)

- The feature is implemented against the `Docs/*.md` contract (source of truth).
- Tests are written and **green** (`go test ./...` per service; `npm test` +
  `npm run build` for frontend).
- `PROGRESS.md` (in the service's own directory) is up to date.
- Work is on a **feature branch**, not `main`, and committed.

## Steps

1. **Push the branch.** `git push -u origin <branch>` (requires a GitHub remote —
   see Prerequisite below).
2. **Open the PR.** `gh pr create --base main --head <branch> --title "<what>" --body`
   with a body that states: what changed and why, the `Docs/02-Backend.md` (or
   `03-Frontend.md`) section it implements, cross-service contract points other
   services must match, and the test evidence (what you ran, that it's green).
3. **Request review from `pr-reviewer`.** Hand the PR to the **pr-reviewer** agent
   (delegate to it with the PR number/URL). It returns ranked findings + an
   APPROVE / REQUEST CHANGES verdict. It is read-only — it does not fix anything.
4. **Address findings.** Fix every `blocker` and `major`; decide per case on
   `minor`/`nit`. Re-run tests, push updates to the same branch, and re-request
   review until the verdict is **APPROVE**.
5. **Merge** only after APPROVE — `gh pr merge <n> --squash` (or leave it for a human
   to merge if repo policy requires human sign-off). Delete the branch.

## Rules

- The author never approves their own change; the **pr-reviewer** agent (or a human)
  does.
- No direct commits to `main` for feature work.
- If the reviewer requests changes, iterate — don't merge over a REQUEST CHANGES.

## Prerequisite: GitHub must be connected

`gh pr create` needs a GitHub **remote** and an authenticated **`gh` CLI**. If the
repo has no remote yet (`git remote -v` is empty) or `gh` isn't installed/authed, set
that up first. Until then you can still get a **local review**: push the branch (or
just have it locally) and hand `pr-reviewer` the branch name — it reviews
`git diff main...<branch>` — but the real flow is a PR.
