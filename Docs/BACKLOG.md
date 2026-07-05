# DogMap — Backlog & Known Minor Issues

Collected from the PR reviews (#1–#9) and build notes for the feature batch delivered
2026-07-04/05. **None of these are blockers** — all planned MVP features shipped and
`main` is green. This is the running list of polish, tech-debt, deferred decisions, and
things to verify later. Grouped by area; check items off as they're addressed.

_Last updated: 2026-07-05._

---

## Quick wins / housekeeping
- [ ] **Dead code:** `LoginFor` in Profiles (`profiles/internal/domain/postgres/postgres.go`,
      the `profileStore` interface, and the fake) has **zero call sites** after PR #3's
      `LEFT JOIN` fix. Drop it.
- [ ] **Gitignore `.claude/settings.local.json`** — the local permission allowlist keeps
      appearing in PR diffs. `git rm --cached` + add to `.gitignore`.
- [ ] **`pr-reviewer` payload collision:** running review agents in parallel clobbered a
      **shared scratchpad payload file**, causing a duplicate review + a *misattributed*
      review (PR #3 content landed on PR #6). Bake "use a `mktemp` unique file, never a
      shared scratchpad path" into `.claude/agents/pr-reviewer.md`. (Stray COMMENT review
      `4631068481` on PR #6 can't be deleted — disregard it.)
- [ ] **Agent scope guardrails reference non-existent dirs:** `ios-dev` / `android-dev` /
      `frontend-service` say "don't touch `backend/` / `Backend/`", but the services are
      `auth/`, `map/`, `profiles/` at top level. Fix the wording; also "don't touch
      `frontend/`" should exclude the agent's own `frontend/ios|android` subdir.
- [ ] **Three near-duplicate frontend/mobile agents** — contract changes must be applied in
      three places. Consider a shared reference doc they all point at.

## Per-feature review nits
### Map — friends widget (PR #7)
- [ ] `frontend/src/stores/mapStore.ts` — `refreshObject` failure is swallowed and `select`
      still runs, so a fetch error **centers the map but opens no popup** (silent). Same on
      the friends-widget click path. Surface an error/toast or skip the select.
- [ ] `map/internal/app/map_server.go` (~FriendsPresence) — a `CurrentObject` error aborts
      the whole response, while a missing object row is skipped per-friend. Defensible; add
      a comment explaining the asymmetry.

### Friend on-walk status (PR #9)
- [ ] `frontend/src/components/profile/FriendsPanel.vue` — an empty `object_name` (docs allow
      unnamed OSM features) renders "🐾 on a walk at" + an **empty clickable link**. Fall back
      to `object_name || 'a spot'`.
- [ ] `frontend/src/stores/friendsStore.ts` — the best-effort presence `catch` keeps **stale**
      presence on a persistent outage (a friend who ended their walk keeps showing the old
      spot). Acceptable vs. blanking, but consider clearing/age-bounding after N failures.
- [ ] PR #9 body said "56 tests"; actual count is 54. (Cosmetic.)

### Two-step registration (PR #6)
- [ ] `frontend/src/pages/ProfileSetupPage.vue` — navigates before `toast.success(...)`, so the
      confirmation shows *after* leaving the page (works via the global toast store; ordering
      reads backwards).
- [ ] `frontend/src/router/index.ts` guard silently swallows a `loadSelf` failure (correct +
      test-covered) — add an optional debug log so a *repeated* failure isn't invisible.

### Auth email confirmation (PR #8) — ⚠️ NOT fully reviewed
- [ ] The #8 `pr-reviewer` pass was **stopped before completion**, so this security-sensitive
      code never got a full review. Outstanding to verify:
  - [ ] Email-format validation **at the edge** (is the protovalidate interceptor wired?) —
        header-injection surface on the email `to` / verify link. Register only checks
        non-empty.
  - [ ] Re-run a full `pr-reviewer` pass on the merged #8 changes when convenient.

### Rich map objects backlog (PR #4) — planning notes for when it's built
- [ ] The spec **reverses two _locked_ Idea-doc decisions** (`Docs/01-Idea.md`: no
      user-generated content; read-only / no moderation). State it supersedes those lines
      when the feature is scheduled.
- [ ] Put the rate-limit Valkey counter under a **Map-owned key prefix** (avoid ownership drift).
- [ ] Decide: `LoadMap` markers carry stats inline vs. lazy-load (drives the aggregate cache).
- [ ] Popup-feed index should be **partial** (`WHERE hidden_at IS NULL`) given soft-delete.
- Full spec: `map/PROGRESS.md` → "Future / Backlog: rich map objects".

## Known limitations / ops
- [ ] **Docker schema via `initdb` runs only on a fresh volume.** After editing a migration,
      `docker compose down -v` before `up`. Production should use a real migration runner
      (golang-migrate / goose) as a startup step.
- [ ] **Email is Mailpit** (dev capture at `localhost:8025`). Swap for a real provider
      (SendGrid / SES / Mailgun) via `smtp.*` config for production.
- [ ] **DB-backed integration tests skip without a DSN** — need testcontainers or a live
      Postgres+Valkey to run them (e.g. in CI).
- [ ] Presence can go **stale on a polling outage** (see PR #9) — a general polling limitation;
      a WS/SSE push would fix it (already the planned evolution).

## Future features (bigger — plan separately)
- [ ] **Rich map objects** (comments / ratings / photos) — `map/PROGRESS.md` backlog + PR #4 spec.
- [ ] **Email-change flow** (post-MVP; `login`/`email` currently immutable).
- [ ] **Replace polling with WebSocket/SSE** — the store poller is the swap seam.
- [ ] **Native iOS / Android apps** — `ios-dev` / `android-dev` agents exist; apps not scaffolded.
- [ ] **Branch-protection ruleset on `main`** if not already applied (owner-only merge; note the
      solo-repo self-approval caveat — don't require approvals).
