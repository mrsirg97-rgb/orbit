# board

## What it is

The shared board: the project's memo log on torch's chain folded into a
task board. SPEC_BOARD governs. The memo shapes, the fold, the local
cache, and the swarm surface (claim / note / complete / accept / reject /
reap) live here. The chain memo log is the source of truth; the local
SQLite is a cache rebuilt from it, never trusted.

## What it includes

- `MemoFor`, `ParseMemo`, `Verbs`, `MemoCap` — the memo grammar: one memo
  per verb (`task`, `brief`, `claim`, `note`, `complete`, `accept`,
  `reject`), capped at the curve memo bound. The verb says what happened —
  no role tag rides the memo. A memo outside the grammar parses false:
  the fold skips it, never throws.
- `Fold` — the pure board fold: a project's parsed memo log in log order
  applied to the task board. Ownership and stake decide, never a label:
  the task memo's sender is the task's funder (brief and accept count only
  from the funder), while claim, note, complete, and reject are honoured
  from anyone whose memo is in the log — gated only by task state.
  Malformed, foreign, or inapplicable memos are skipped; the lease is the
  one stateful-looking rule and it is pure.
- `Lease` — the claim's expiry (24h, the runtime's stale-claim window):
  it applies only while the claim is the task's live state — an active
  task whose claim is older than the lease folds as pending, while a
  claim superseded by complete/accept/reject is never dropped.
- `Store` — the local SQLite cache (the chain's message log plus the fold
  projection) and the chain write path. `Sync` folds the chain into the
  cache (indexer fast path, RPC scan fallback), idempotent by signature;
  `Act` writes one board verb (memo + vault-routed micro buy), refuses
  what the fold would refuse before spending, mints the task id after the
  sync, and re-syncs after the write (the memo's seq is the chain's,
  never a local guess); `Board` / `BoardFromCache` / `Task` / `NextID` /
  `Claims` / `LastMemo` are the reads.
- `Store.Claim` / `Note` / `Complete` / `Accept` / `Reject` / `Reap` —
  the swarm surface: the same vocabulary the runtime's swarm drains,
  chain-backed. `claim` is the memo, the lease is the fold's expiry, and
  `reap` returns expired claims to pending. The surface has no roles and
  no sessions — the chain has no sessions, only leases; any wallet may
  claim, note, complete, and reject; accept is honoured only from the
  task's funder (the fold decides).
- `renderBoard` — the lean read: project header (goal), one line per task
  (status, owner, age), the summary line. A project without a goal memo
  shows none.

## How it is consumed

- The board tool (`tool/board.go`) and the `orbit board` subcommand drive
  `Store`; the brief's INTEL section reads the same message rows through
  the client; the earn footer reads `Claims` and `LastMemo` from the
  cached board.
- `Sync` is the cache's window: each sync adds the newest messages (100
  per sync) and the fold covers what the cache holds. The chain's slot and
  timestamp replace the local approximation on the next sync.
- Reads are primary-key-seek only — one task is `GetTask(project, id)`, a
  board is `WindowTaskByProject`, a message is `GetMessage(mint, seq)`.
  The signature unique index is write-side only (the sync's idempotency
  door), never a read path.

## Gotchas

- The store file never is the record: the chain memo log is. The cache is
  rebuilt from the chain, never trusted.
- `Task.Notes` carry verdict reasons too — a reject's reason lands in the
  notes.
- The lease is pure: it materializes in the projection, and `Reap` only
  reports what the log shows as expired. The ended-session arm of the
  runtime's reap has no chain equivalent — the chain has no sessions.
- `StorePath` is the orbit home's `board.sqlite`; `Open` quarantines a
  corrupt file loudly.
- `board/metadata` is the generation input: edit it and regenerate
  `ddl`/`domain`; never hand-edit the generated projections.
