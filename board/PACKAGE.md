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
  `reject`, `release`), capped at the curve memo bound. The verb says what happened —
  no role tag rides the memo. A memo outside the grammar parses false:
  the fold skips it, never throws.
- `Fold(project, memos, ledger, now)` — the pure board fold: a project's
  parsed memo log in log order applied to the task board, read against a
  `Ledger` (SPEC_WORK). Ownership and stake decide, never a label: the
  task memo's sender is the task's funder (brief and accept count only
  from the funder), while note, complete, and reject are honoured from
  anyone whose memo is in the log, gated by task state. The ledger adds
  three rules: the work verbs (task, claim, complete, accept, reject,
  release) fold only when `Ledger.Public` (torch status migrated); a
  claim is honoured only when its signature's `Carrier` carries capital
  (a buy above the memo stake is a contract, an open long is work, and
  the task records the backing, the stake, and the position key); a work
  claim whose position `Ledger.Ends` reports ended is released at that
  time. `release <id>` from the owner returns the task to pending. A
  reject whose carrier is a short records the collateral beside the
  verdict. Malformed, foreign, or inapplicable memos are skipped.
- `Ledger`, `Carrier` (client), `Backing` — the ledger beside the log:
  `Public`, the carriers by memo signature, and the ended long positions
  by (vault, index). `IsPublic` names the one torch status that accepts
  work. `backingOf` is the capital rule in one place.
- `Lease` — the contract claim's expiry (24h, the runtime's stale-claim
  window): it applies only while the claim is the task's live state. A
  work claim has no lease; the ledger releases it.
- `Store` — the local SQLite cache (the chain's message log plus the fold
  projection) and the chain write path. `Sync` folds the chain into the
  cache as a walk, idempotent by signature: the indexer pages newest-first
  with `before=<oldest created_at seen + 1s>` (the boundary second
  re-fetched, signatures dedupe) and the RPC scan pages with the signature
  cursor, until a scanned page holds a signature already cached or comes
  back short. The scan's short-page stop and cursor are the page facts
  (signatures scanned, oldest scanned signature), not the memo rows — a
  full page of trades with no memo is not genesis and the walk continues
  past it. A fresh cache walks to genesis; a warm cache reads one page.
  The walk is bounded at 50 pages: past it the cache is marked incomplete
  (`project_incomplete`) and the render says so — the goal is the first
  goal memo in the walked log. Reads default to the indexer and fall back
  to the RPC scan when the indexer is unreachable (connect error, 5xx,
  timeout — never a 4xx). The source is recorded per project in
  `project_sources` and a mint's source never changes while its rows
  exist: the fallback applies only to an unrecorded mint (one line
  naming the switch), an outage on a recorded-indexer mint inserts
  nothing — the board read serves the cache with `indexer unreachable:
  board may be stale` and acts are refused — and a mint first synced by
  scan stays on scan until the cache is rebuilt. The one source change
  by config (the indexer unset on a recorded-indexer mint) wipes the
  mint's rows and re-syncs under the new source in one transaction.
  The sync also carries the ledger: the project's torch status
  (`project_states`, kept when the read fails), the carrier of every new
  claim or reject row (`message_carriers`, read once from the row's
  transaction through the RPC seam; a failed read fails the sync rather
  than folding a claim as foreign), and the indexer's ended long
  positions (`project_positions`, kept when the indexer is unset or
  fails). `Act` writes one board verb (memo + vault-routed micro buy),
  refuses what the fold would refuse before spending (including the state
  gate, named), mints the task id after the sync, confirms the tx
  (bounded poll), then re-syncs until the memo is in the cache — the indexer lags, so an immediate re-sync would miss the
  memo and a retried task would double-spend. If the memo does not land,
  the reply is `pending: not yet indexed` (the write is confirmed, only
  the cache is behind); a renumbered task's reply names the assigned id.
  After every act the store calls `OnAct` (when set) with the final shape
  — the minted task id, or the assigned id when the fold renumbered — so
  the fire path can refresh the footer. `Board` / `BoardFromCache` /
  `Summary` (a cached project's goal and task counts, with the cached
  marker the discovery list uses) / `Task` / `NextID` / `Claims` /
  `LastMemo` are the reads.
- `Store.Contract` / `Work` / `Release` / `ShortReject` / `Held` — the
  acts (SPEC_WORK): a contract buys above the memo stake with `claim
  <id>` on the tx; work picks the next free long index, opens
  `open_long_via_vault` on the caller's token collateral with the claim
  memo; release closes the held claim's position (or sells the contract
  holding) with `release <id>`, and a partial close (under 10000 bps)
  sends no memo so the claim stands; a short reject opens
  `open_short_via_vault` with `reject <id>: <reason>`. Each goes through
  the one act path (sync, refuse-foreign with the candidate carrier,
  write, confirm, await). `Held` folds the cache and lists the caller's
  active or review tasks with their backing. `Accepts` and `Projects` are
  the reputation reads.
- `Store.Claim(lamports)` / `Note` / `Complete` / `Accept` / `Reject` /
  `Reap` — the swarm surface: the same vocabulary the runtime's swarm
  drains, chain-backed. `claim` is a contract at the given size, the
  lease is the fold's expiry, and `reap` returns expired claims to
  pending. The surface has no roles and
  no sessions — the chain has no sessions, only leases; any wallet may
  claim, note, complete, and reject; accept is honoured only from the
  task's funder (the fold decides).
- `renderBoard` — the lean read: project header (goal), one line per task
  (status, owner, age, and the backing: `contract 0.0050 SOL`, `work #2`,
  `short 0.0200 SOL` beside a reject), the summary line. A project
  without a goal memo shows none. The backing comes from `task_backing`,
  the projection's side table.

## How it is consumed

- The board tool (`tool/board.go`) and the `orbit board` subcommand drive
  `Store`; the brief's INTEL section reads the same message rows through
  the client; the earn footer reads `Claims` and `LastMemo` from the
  cached board.
- `Sync` is the cache's walk: a fresh cache walks to genesis, a warm
  cache reads one page, and the fold covers what the walk reached. The
  chain's slot and timestamp replace the local approximation on the next
  sync.
- Reads are primary-key-seek only — one task is `GetTask(project, id)`, a
  board is `WindowTaskByProject`, a message is `GetMessage(mint, seq)`.
  The signature unique index is write-side only (the sync's idempotency
  door), never a read path.

## Gotchas

- The store file never is the record: the chain memo log is. The cache is
  rebuilt from the chain, never trusted.
- `project_sources` is the one per-project record: the source that
  numbers the messages. It decides every sync and flips one way (indexer
  → scan); delete the cache to re-source a project.
- `project_incomplete` is the walk-bound flag: past 50 pages the cache
  is marked incomplete and the board render says so. It is cleared only
  by a walk to genesis — a cached-boundary stop proves nothing about the
  log below the old cache's edge.
- The ledger tables (`project_states`, `message_carriers`,
  `project_positions`, `task_backing`) live in `extra.sql` beside the
  generated projection. Carriers are chain facts keyed by signature and
  survive a source wipe; the status and the positions are the last
  successful reads. Without an indexer there are no position reads, so a
  work claim is released only by its `release` memo.
- Two clients agree on a board only when they read the same ledger: the
  same indexer's positions and the same chain's transactions. The
  determinism test extends to capital and positions.
- `Task.Notes` carry verdict reasons too — a reject's reason lands in the
  notes.
- The lease is pure: it materializes in the projection, and `Reap` only
  reports what the log shows as expired. The ended-session arm of the
  runtime's reap has no chain equivalent — the chain has no sessions.
- `StorePath` is the orbit home's `board.sqlite`; `Open` quarantines a
  corrupt file loudly.
- `board/metadata` is the generation input: edit it and regenerate
  `ddl`/`domain`; never hand-edit the generated projections.
