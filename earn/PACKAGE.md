# earn

## What it is

The one-minute join path as a slash command split into moments: bare
`/earn` (status when set up, else join as worker), `join [roles]`,
`roles list/add/remove`, `goal`, and the status rows the TUI's footer
renders. The operator key is named at the call — `--operator-key`,
`--operator-key-path`, or `ORBIT_OPERATOR_KEY(_PATH)`; the path is asked
once at the first signing step and remembered as
`ORBIT_OPERATOR_KEY_PATH` in the config (the path, never the key).
SPEC_EARN governs.

## What it includes

- **`Command`** (`command.go`): the `/earn` slash command — `Run`
  dispatches the moments: bare, `join`, `roles`, `goal`, `status`,
  `stop`, `start`.
- **`join [roles]`**: runs the setup (init, vault create, link, deposit)
  with the operator key named at the call, then registers the roles
  (worker by default). The key resolves lazily, at the first step that
  signs — a rerun on a set-up home registers roles with no key. Before
  any chain spend, one preflight line names what exists and what will
  happen. Every step is idempotent; one identity row per role is
  registered, the scheduled jobs start, and the roster prints.
- **`roles` / `roles add|remove <role>`**: the roster, and one role's
  registration or removal (identity row + job).
- **`goal "<text>"`**: the architect's goal — set or change it,
  registering the architect (and its job) when none exists.
- **`status` / `stop` / `start`**: the footer rows, and the pause/resume
  of every identity's scheduled job.
- **`Rows`** (`status.go`): the footer — projects held, open claims, last
  memo, PnL since start. `Status` reads it from the chain (wallet read +
  the board cache); `RowsFromBrief` computes the same rows from a fire's
  read state with no extra chain reads.
- **The snapshot**: `SnapshotPath` / `Snapshot` / `WriteSnapshot` — the
  local `status.json` (atomic temp + rename). The TUI status callback
  reads only the file, never the chain, so every command stays off the
  network. `/earn status` and each agent fire write it.

## How it is consumed

- The wizard asks with the answers at the call: `/earn architect worker
  --goal "..." --model <id>` — a missing role set or an architect without
  a goal refuses naming the exact follow-up.
- The rows are a local snapshot: command and fire time are the only chain
  reads. A mid-turn tool effect shows stale until the next command, and
  the brief's own numbers carry the same staleness.
- The role set must name one of the three; `--model` is always explicit
  (the active model is the fallback).

## Gotchas

- The board cache is the window: a project the cache has never synced
  contributes nothing to the footer, and a project whose claim/memo read
  fails is skipped rather than counted zero.
- The footer never carries a role tag: the wallet that paid the memo buy
  is the contributor, and the fold decides ownership and stake.
- `tokenize` honors double quotes (a quoted value may contain spaces) and
  has no escapes — the goal is one paragraph.
- The snapshot is best-effort at fire time: a failed snapshot write never
  kills the fire.
