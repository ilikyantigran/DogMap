---
name: progress-tracking
description: Maintain a durable PROGRESS.md file in the working directory so long-running work survives pauses, restarts, context summarization, and hand-offs to another agent. Use this whenever an agent is instructed to keep resumable state or track progress: read PROGRESS.md before starting any work, create it if it does not exist, and update it after every meaningful step so nothing is lost. Especially important for multi-step builds, migrations, and any task that may span more than one session.
---

# Progress tracking

You (or a future instance of you, or a different agent entirely) may be interrupted at any moment — a session ends, context gets summarized, the user pauses for a day. When that happens, everything in your head is gone. The only thing that survives is what you wrote to disk.

`PROGRESS.md` is that durable memory. It is a living state file that lets whoever picks up the work next continue from exactly where things stopped, without re-deriving decisions or redoing finished work. Treat it as the single source of truth for "where are we and what's next," written for a reader who has **zero memory of this task**.

## The two rules that matter

**1. Read before you act.** At the very start of any work — before writing code, before planning, before touching anything — locate `PROGRESS.md` in your working directory and read it in full. If it exists, it tells you what's done, what's in flight, and what to do next; resume from there rather than starting over. If it does not exist, create it (see the template) before doing the work.

**2. Write after every meaningful step.** Whenever you complete a coherent unit of work — a component built, a test passing, a decision made, a blocker hit — update `PROGRESS.md` to reflect the new reality. The goal is that if you vanished right now, the next reader would lose nothing. Update *before* you move on to the next thing, not in a batch at the end (the end may never come).

## What "working directory" means

Write `PROGRESS.md` in the part of the project you are responsible for — the subtree you're actually editing (e.g. `Frontend/WebApplication/`), not necessarily the repo root. This keeps the progress file next to the work it describes, so it travels with that component and doesn't collide with other agents tracking other parts of the project. If your scope genuinely spans the whole project, the repo root is fine. One agent, one PROGRESS.md for its scope.

## What "meaningful step" means

Update on units of work, not on every tool call — logging every file read would bury the signal and waste effort. Update when:

- you finish a discrete piece (a file, a feature, a store, a passing test suite),
- you make or reverse a decision that affects later work,
- you discover a constraint, blocker, or surprise,
- you're about to switch to a different area of the work,
- you sense an interruption coming (wrapping up, running low on context).

When in doubt, ask: "if I disappeared after this action, would the next reader be confused or redo something?" If yes, write it down.

## The file structure

Keep it lean and current — this is a *state* file, not a changelog of everything that ever happened. Prune stale detail; the "Done" list can stay terse. Always keep these sections:

```markdown
# PROGRESS — <component / task name>

_Last updated: <YYYY-MM-DD HH:MM> by <agent name>_

## Goal
One or two sentences: what this work is trying to achieve, and the source of truth
(e.g. "Implements the Map Page per Docs/03-Frontend.md").

## Current status
The single most important paragraph: where things stand *right now*, and what the
very next action is. A cold reader should be able to start from this line alone.

## Done
- [x] Concrete things completed, with file paths where useful.

## Next steps
- [ ] The immediate next action (top = do this first).
- [ ] Then this.

## Decisions & constraints
- Decision made, and *why* (so it isn't relitigated). Note anything the docs left
  open that you resolved, and how.

## Blockers / open questions
- Anything waiting on the user, an external dependency, or an unresolved question.
  Empty is fine — say "none".

## Notes
- Anything else the next reader needs: gotchas, commands to run, where things live.
```

## How to write it well

- **Write for a stranger.** No "as discussed" or "the usual" — spell out paths, commands, and reasoning. The reader has none of your context.
- **Lead with the next action.** The first thing someone needs is "what do I do now." Keep "Current status" and the top of "Next steps" sharp.
- **Record the *why* behind decisions**, not just the what — that's what stops the next agent from undoing your choices or re-asking a settled question.
- **Keep it honest.** If something is half-done or broken, say so plainly. A PROGRESS.md that overstates completion is worse than none, because it hides the real state.
- **Timestamp updates** so staleness is visible.

## Resuming from an existing PROGRESS.md

When you find one at startup: read it fully, sanity-check it against the actual state on disk (files may have changed, tests may now fail), reconcile any drift, then continue from "Next steps." If reality and the file disagree, trust the code and correct the file as your first update.
