# agent

## What it is

The scheduled agent: one identity row -> one runtime scheduled job. The
fire is the one-shot worker with the brief as its prompt; cadence comes
from the identity row; stall/budget/timeout ride along as configured.
The per-fire brief is the subject of SPEC_BRIEF — this package owns the
job and the fire.

## What it includes

- `JobPrefix`, `JobName`, `JobAgentID` — the stable job-name mapping: a
  job's name is `orbit-agent-<id>`, and a job whose name lacks the prefix
  is not an orbit agent job (the scheduler enforces one job per name).
- `Register` — creates the scheduled job from the identity row. The
  prompt is the brief; the cadence, model, stall, budget, and timeout
  come from the row.
- `Refresh` — updates the job's prompt (the fresh brief) without touching
  the cadence or the budget.
- `Fire` — the per-fire path: the caller builds the fresh brief (a live
  read snapshot projected through `brief.Build`), Fire refreshes the
  job's prompt with it, then runs the fire. The brief is therefore
  per-fire — the prompt stored at register is only a stub naming the
  identity until the first fire replaces it.

## How it is consumed

The register order, and why a fire can skip with "no crontab line
(drift)":

1. identity upsert (one row, idempotent);
2. scheduler `Create` — it installs the crontab line FIRST, then commits
   the job row. A concurrent fire that checks the line before the commit
   (or after the line was pruned/replaced by another crontab edit) sees
   the line missing and records the drift skip;
3. the fire's `run-job` re-asserts the line through `Refresh` (the
   scheduler's update runs upsert-line + install), then loads the job and
   spawns the worker.

So: register -> job row + line; every fire -> brief rebuild -> prompt
update -> line re-assert -> spawn. The drift skip is the guard firing
before the line lands, not a lost job.

## Gotchas

- `Register`, `Refresh`, and `Fire` refuse an empty brief with a named
  error — no fire without a brief.
- `JobAgentID` returns "" for a job that is not an orbit agent job; the
  run-job path treats that as a refusal, not a row.
- The job row carries the identity's cadence/stall/budget/timeout; the
  defaults come from the identity role table, and the register flags
  override them.
