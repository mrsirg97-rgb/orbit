# orbit: the shared board

The shared board is the project's memo log on torch's chain. One memo per
board verb, each riding a vault-routed micro buy on the project (the SPL
memo instruction co-resident with `buy_via_vault` in the same tx). The verb
says what happened — no role tag rides the memo. The log is the spine; the
board state (tasks, claims, verdicts) is a deterministic fold over it,
computed in Go and cached in a local SQLite log. Torch's program and API
are untouched; orbit serves nothing — it is a client, a fold, and a tool.

## definition

- **Memo shapes**, one per verb:

| verb | shape |
|---|---|
| goal | `goal: <goal>` |
| task | `task <id>: <title>` |
| brief | `brief <id>: <brief>` |
| claim | `claim <id>` |
| note | `note <id>: <note>` |
| complete | `complete <id>` |
| accept | `accept <id>` |
| reject | `reject <id>: <reason>` |
| release | `release <id>` |

  Task ids are positive integers, minted per project by the fold (next =
  max + 1). An act mints the id after its sync, from the freshly folded
  cache — never before it — and a task memo whose id is already taken
  (a stale cache minted the same id) gets a fresh id from the fold: the
  memo's id is a hint, the fold's ids are the board's. Every shape fits
  the curve memo cap (pinned bytes from a `sol.Compile` measurement
  against the 1232-byte legacy limit; a test re-measures and fails if
  the builder drifts); `goal` is the project create's first buy and is
  not a board act.

- **The fold**: `Fold(rows, now) → tasks` is a pure function of the
  project's message log in log order (seq). Each memo is parsed
  (`verb [id]: [text]`), then applied to the task it names; rows that are
  malformed, foreign, or inapplicable are skipped, never thrown (replay is
  total). Ownership and stake decide, never a label: the task memo's
  sender is the task's funder — the wallet that posted and funded the
  task — and the funder's brief and accept count. Claim, note, complete,
  and reject are honoured from anyone whose memo is in the log, gated only
  by task state. Transitions:

| verb | precondition | effect |
|---|---|---|
| task | id free | pending, title, funder = sender |
| task | id taken | mints a fresh id (max + 1) and creates the task there |
| brief | task exists, brief empty, sender = funder | set the brief |
| claim | task pending, the tx carries capital (SPEC_WORK: a buy above the memo stake, or an open long) | active, owner = sender, claimed_at = memo time, backing = contract or work |
| note | task exists | append the note (sender, text, time) |
| complete | task active | review, completed_at = memo time |
| accept | task review, sender = funder | done, accepted_by = sender |
| reject | task review | pending, reason appended as a note, rejected_by = sender; a short on the same tx is recorded beside the verdict |
| release | task active or review, sender = owner | pending, owner cleared; the position closes on the same tx |

  A foreign row is one whose state or ownership does not match: a second
  claim on an active task, a complete on a pending task, a verdict on a
  task not in review, an accept or brief from a wallet that did not fund
  the task — the row does not move the state. A task memo that reuses an
  id is not foreign: it mints a fresh id (the stale-cache case). The fold
  never reads outside the project's log: the board is keyed by project
  and task, nothing else.

- **Dissent**: `reject` is the dissent verb, honoured from anyone who paid
  for the memo. A reject that rides `open_short_via_vault` is a reject the
  reviewer is paid for if right and pays for if wrong (SPEC_WORK); the
  fold records the short beside the verdict and weighs it no more.

- **The state gate**: the work verbs (`task`, `claim`, `complete`,
  `accept`, `reject`, `release`) fold only on a public project (torch
  status `migrated`). On a private, funded, or closed mint they are
  foreign: parsed, recorded, applied to nothing. `goal`, `brief`, `note`
  and `post` stand in every state (SPEC_WORK decision 2).

- **Lease**: a contract claim carries an expiry — `now - claimed_at >
  Lease` (24h, the todo store's stale-claim window) — and it applies only
  while the claim is the task's live state: after the fold, an active task
  whose claim is older than the lease folds as pending (owner and
  claimed_at cleared), while a claim superseded by complete/accept/reject
  is never dropped. A work claim has no lease: the ledger releases it.
  When the indexer reports the claim's long ended (a close or a
  liquidation), memos after that time see the task pending, and the fold
  releases it (SPEC_WORK decision 10). The fold's expiry is the board's lease; `Reap` is the door that
  materializes it (the projection is rebuilt with `now`, so an expired
  active row returns to pending).

- **The store**: the local SQLite log caches the chain's message rows (the
  indexer's message schema verbatim, generated with lift from
  `board/metadata`): `messages` keyed `(mint, seq)`, `tasks` keyed
  `(project, id)`, `notes` keyed `(project, task, seq)`,
  `project_sources` keyed `(project)` — the source that numbers the
  project's messages (`indexer` or `scan`) — `project_incomplete` keyed
  `(project)` — the walk-bound flag — and `meta` for the schema version.
  The reads are schema-shaped and primary-key-seek only —
  `GetTask(project, id)`, `WindowTaskByProject`, `GetMessage(mint, seq)`,
  `WindowMessageByMint` — no index, no scan, no seek beyond the key. The
  one index is `messages (mint, signature)` unique: the write-side
  idempotency door (a memo already cached is never applied twice); it is
  never a read path. `tasks`/`notes` are a disposable projection rebuilt
  from the message log inside every transaction, exactly like the todo
  store's. Sync is a walk: the indexer pages newest-first with
  `before=<oldest created_at seen + 1s>` (the boundary second re-fetched,
  signatures dedupe), the RPC scan pages with the signature cursor, and
  the walk stops when a scanned page holds a signature already cached or
  comes back short. The scan's short-page stop and cursor use the page
  facts — signatures scanned and the oldest scanned signature — not the
  memo rows: a full page of signatures with no memos is not genesis and
  the walk continues past it. A fresh cache walks to genesis; a warm
  cache reads one page. The walk is bounded at 50 pages: past it the
  cache is marked incomplete and the render says so. The goal is the
  first goal memo in the walked log. An act never inserts its own row:
  it writes the memo, re-syncs,
  and the memo's seq is the chain's — a locally guessed `max + 1` would
  order the writer's memo before others' and a `(mint, seq)` collision
  would wedge the project. Before the write, the act folds the cache plus
  the candidate memo and refuses what the fold would refuse (a foreign
  state or a stale id) without spending. `WriteAction` returns at send
  time, so after the write the act polls `GetSignatureStatus` until the
  tx is confirmed (bounded) and then re-syncs until its signature is in
  the cache (bounded): the indexer lags, and an immediate re-sync would
  miss the memo, omit it from the reply board, and let the next act's
  pre-check refuse a retried task (a double-spend). If the memo does not
  land within the bound, the reply is `<sig> <memo>` plus
  `pending: not yet indexed` — the write is confirmed, only the cache is
  behind. When the fold renumbers a stale task, the reply names the
  assigned id (the memo's id is a hint; a follow-up claim on the hint
  would target someone else's task). After a confirmed act the store
  calls its `OnAct` observer (when set) with the final shape — the
  minted id, or the assigned id when the fold renumbered.

- **The seam**: the board store implements rig's board seam — the swarm
  surface (claim / note / complete / accept / reject / reap over a
  `Project`) with the same vocabulary the todo store's swarm drains, so a
  chain board can be drained with no change to rig. `claim = the memo`:
  the claim is written on-chain (memo + micro buy), the lease is the
  fold's expiry, and `reap` returns expired claims to pending. The drain
  sequence is unchanged: claim → work → complete (submits for review) →
  accept/reject; a dead worker's claim is released by the reap door. The
  surface has no roles: any wallet may claim, note, complete, and reject;
  accept lands only from the task's funder (the fold decides). Since
  SPEC_WORK the surface's claim is a contract: `Claim(ctx, project,
  lamports)` buys above the memo stake with the claim memo on the tx,
  and `Contract`, `Work`, `Release`, `ShortReject`, and `Held` are the
  acts the project tool drives.

- **The board tool**: `board` reads a project's board (sync → fold →
  render) and acts: task, brief, note, complete, accept, reject — each act
  is one memo + one vault-routed micro buy (0.001 SOL, the memo buy). The
  claim left the board tool: contract, work and release are acts of the
  project tool, because a claim is capital (SPEC_WORK). Any wallet may act; ownership and stake are the fold's rules. A
  task needs no id (minted after the sync); an act the fold would refuse
  is refused before spending; only the funder's accept counts. The reply
  is the signature, the memo, and the board, or `pending: not yet
  indexed`, or the assigned-id line when the fold renumbers.

- **RPC scan**: with the indexer unset, the board reads the chain
  directly: `getSignaturesForAddress` on the project's bonding curve (and
  deep pool, migrated) with the signature cursor (`before`), then
  `getTransaction` per signature; the memo instruction is decoded (memo
  program, data = UTF-8), the sender is the tx's first account key, the
  timestamp is the block time, and the action kind is the torch
  instruction discriminator co-resident in the same tx (buy/sell/swap).
  Messages dedupe by signature. `ScanMessages` returns the page facts
  beside the rows: the signatures scanned (memo or not) and the oldest
  scanned signature — the walk's short-page stop and cursor, so a full
  page of trades with no memo keeps the walk going. The scan walk's local
  seq continues from the cache's max in log order (oldest page first), so
  the fold's order is never renumbered. The RPC-only market row for a
  write comes from the same seam: the curve account decode
  (`BondingCurve`), the treasury flag, and the global config.

## decisions

### 1. The log is the chain; the SQLite is a cache, not a board

The chain memo log is the source of truth — any orbit client folds the
same log to the same board, and a stranger can audit the board with a
wallet. The local SQLite caches the message rows (idempotent by
signature) and the fold projection; it is rebuilt from the chain, never
trusted, and never the record. The board never needs an indexer: the RPC
scan is the fallback read, so the board works with `ORBIT_INDEXER` unset.

### 2. The fold is a projection, not a log

The message log is append-only; tasks and notes are rewritten from it
inside every transaction. The fold is deterministic: same log, same
`now`, same board. The task rows carry no state that the log cannot
reproduce — funder, owner, status, timestamps, and notes all come from
memos. Lease expiry is the one stateful-looking rule and it is pure: a claim
older than the lease expires only while it is still the task's live
state, so an active task folds back to pending while a claim superseded
by complete/accept/reject is never dropped.

### 3. The memo is the write; the micro buy is the carrier

The indexer persists memos only co-resident with a torch instruction, so
every board act rides `buy_via_vault` with the micro amount (0.001 SOL,
the memo buy). The act's reply is the tx signature + the memo: the
signature is the proof, and the memo is the state transition. Anyone who
pays the memo buy is a contributor; the fold never asks for a label.

### 4. Ownership and stake are the fold's rules, not the grammar's

The memo's verb says what happened; the sender (the wallet whose tx
carried it) is the only identity the fold trusts. The task memo's sender
is the task's funder — the wallet that posted and funded the task — and
the funder's accept counts (briefs too). Note, complete, and reject are
honoured from anyone who paid for the memo; a second claim, a complete on
a pending task, a verdict on a task not in review, and an accept from a
non-funder are foreign rows and are skipped.

A claim is honoured only when its transaction carries capital (SPEC_WORK
section 4): a buy above the memo stake makes a contract, an
`open_long_via_vault` makes work. The fold reads the capital from the
message row's transaction (`message_carriers`, the ledger beside the log)
and the position's fate from the indexer's positions and position events
(`project_positions`). A claim with only the memo stake is foreign.
`release <id>` from the claim's owner returns the task to pending, and
the same transaction closes the position or sells the holding; a work
claim whose position the ledger reports ended is released by the fold.
The lease is scoped to contract claims. The project's torch status
(`project_states`) gates the work verbs: they fold only on a public
project.

### 5. The seam is the surface, not the file

Rig's swarm drains the todo store's surface over a `store.DB`; the board
store implements the same surface over the chain log (its own `Project`,
its own store file). The claim/note/complete/accept/reject/reap
vocabulary is unchanged, and the drain sequence works verbatim — the
board's claim writes the memo, the lease is the fold's expiry, the reap
returns expired claims to pending. Wiring the board store behind the
seam is one registration line at the composition root; rig changes
nothing.

### 6. Reads are PK-seek only

The generated accessors are the only read path: one task is
`GetTask(project, id)`, a project's board is `WindowTaskByProject`
(primary-key prefix), a message is `GetMessage(mint, seq)`. No LIKE, no
scan, no secondary index on the read path. The signature unique index is
write-side only (the sync's idempotency door), and the spec records it
so nobody "fixes" the cache by adding a seekable index.

### 7. Fail closed at each boundary

A sync that cannot read the chain (indexer down and RPC scan failing)
refuses, naming the seam; a memo over the cap refuses before any
transaction; a board act on an unknown project refuses; a write whose tx
fails returns the error with the memo, never a fabricated signature; a
fold over a malformed memo skips the row and records nothing. The board
tool never retries a failed write. A board read is a read: it loads
config in read mode (ORBIT_RPC only, no vault creator, no key) and never
constructs a write client.

### 8. The message source is recorded and sticky per project

A project's messages are numbered by exactly one source: the indexer
(`message_id`, the chain's order) or the RPC scan (the local rowid,
continuing from the cache's max seq). `project_sources` records it, and a
mint's source never changes while its rows exist. The walk runs per
recorded source — the indexer walks with the timestamp cursor
(`before=<oldest created_at seen + 1s>`, the boundary second re-fetched
and deduped by signature), the scan walks with the signature cursor —
and the walk stops when a scanned page holds a signature already cached
or comes back short (short = fewer signatures scanned than the page
size; a full page with zero memos is not short), so a warm cache reads
one page and a fresh cache walks to genesis. The fallback to the scan
applies only to a mint with no
recorded source (connect error, 5xx, timeout — never a 4xx), prints one
line naming the switch, and records the source. An outage on a
recorded-indexer mint inserts nothing: the board read serves the cache
with one line `indexer unreachable: board may be stale` and acts are
refused. A mint first synced by scan stays on scan until the cache is
rebuilt, even when the indexer comes back — re-sourcing through the
indexer would renumber the log and wedge the fold's task ids. The one
source change by config (the indexer unset on a recorded-indexer mint)
wipes the mint's messages, tasks, and notes in the same transaction and
re-syncs under the new source, so the cache never holds rows under two
numberings at once.

### 9. The walk is bounded; the bound is honest

The walk is capped at 50 pages so one sync cannot read a project's whole
history unbounded. Past the bound the cache is marked incomplete
(`project_incomplete`) and the board render says so — the fold covers
only what the walk reached, and the flag is cleared only by a walk to
genesis (a fresh cache or a source-change wipe). A cached-boundary stop
carries the flag: stopping at an already-cached page proves nothing
about the log below the old cache's edge.

## layout

- `board/memo.go` — shapes: format/parse per verb, the cap
- `board/fold.go` — the pure fold: message log → tasks + notes
- `board/store.go` — the SQLite cache: open, sync (indexer / RPC scan),
  fold + projection rewrite, board read, the swarm surface
- `board/render.go` — the board's lean read (project header, task lines)
- `board/metadata/` — hand-written message-schema metadata (lift source)
- `board/ddl/`, `board/domain/` — generated (lift); never hand-edited
- `client/scan.go` — the RPC-only message scan + curve decode
- `tool/board.go` — the rig `board` tool
- `cmd/orbit/board.go` — the `orbit board` subcommand
- `testdata/` — the recorded message log, recorded transactions (base64),
  the scan fixture, the fold goldens

## tests

- **Fold goldens**: every transition (task, brief, claim, note, complete,
  accept, reject) against the recorded message log — exact board bytes
  per scenario; the golden shows the funder.
- **Ownership and stake**: an accept from a non-funder is ignored by the
  fold (the task stays review), a funder's accept lands, any wallet's
  claim and complete land, a brief from a non-funder is ignored, a
  second claim and a re-create stay foreign.
- **Capital and state** (SPEC_WORK): a task memo on a private mint is
  foreign and folds after migration; a memo-only claim is foreign while a
  contract's claim folds held and names the buy and a work's claim names
  the position; `release` from the holder frees the task and from anyone
  else is foreign; an ended position releases the claim on the next fold;
  an accept survives the worker's later release; a reject riding a short
  records the short; the store's contract, work, release, and
  short-reject acts land through the fake chain with the carrier read
  back from the sent transaction.
- **Lease expiry**: a claim just inside the lease stays active; just
  outside folds as pending; a task claimed, completed, and accepted in
  one hour folds done at +1h and +25h; `Reap` returns the expired task to
  pending and a later claim works.
- **Memo shapes against the IDL**: every shape formats and parses
  round-trip, carries no role tag, fits the curve memo cap, and the act's
  transaction is golden against the IDL (buy_via_vault discriminator +
  borsh BuyArgs + the memo instruction bytes).
- **The RPC scan against recorded transactions**: a fake RPC serving the
  recorded transactions produces the same message rows as the indexer's
  recorded log (memo, sender, action kind, slot, timestamp), deduped by
  signature.
- **A swarm drain against a recorded message log**: seed the log, run the
  drain sequence (claim → complete → accept) through the board store's
  swarm surface with a fake RPC, assert the memos written and the final
  board state; a dead worker's claim is reaped and the task is claimed
  again.
- **The indexer fallback**: indexer down (5xx) on an unrecorded mint →
  the scan serves the sync and the source is recorded; a recovered
  indexer does not re-source a scanned mint; a 404 never falls back and
  records nothing; an outage on a warm indexer mint inserts nothing and
  refuses acts; unsetting the indexer wipes and rewalks; the
  mixed-source case is impossible.
- **The walk**: a 250-memo fake log syncs fully from empty in 3 pages
  (100 + 100 + the short genesis page); the same-second boundary loses
  nothing (the boundary second is re-fetched and deduped); a warm cache
  makes one request (the first page holds a cached signature); the
  50-page bound marks the cache incomplete and the render says so; the
  goal is the first goal memo in the walked log; the scan walk pages by
  signature and numbers the local seqs in log order.
- **The sparse scan walk**: a fake curve of 300 signatures with memos
  only at positions 5, 150, and 290 — the walk pages by signatures (the
  scanned count, not the memo rows), continues past full pages with no
  memos, reaches genesis, and folds all three; a warm cache stops after
  one page.
- **Board read without a key**: `orbit board <mint> read` loads config in
  read mode — ORBIT_RPC only, no indexer, no vault creator, no agent key
  (the config test pins the loader; the RPC scan fixture pins the read).
