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

  Task ids are positive integers, minted per project by the fold (next =
  max + 1); the writer reads the board before it writes, so the id it
  quotes is the fold's. Every shape fits the curve memo cap (measured
  bytes, not runes, from `sol.Compile` against the 1232-byte legacy
  limit); `goal` is the project create's first buy and is not a board act.

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
| task | id is new | pending, title, funder = sender |
| brief | task exists, brief empty, sender = funder | set the brief |
| claim | task pending | active, owner = sender, claimed_at = memo time |
| note | task exists | append the note (sender, text, time) |
| complete | task active | review, completed_at = memo time |
| accept | task review, sender = funder | done, accepted_by = sender |
| reject | task review | pending, reason appended as a note, rejected_by = sender |

  A foreign row is one whose state or ownership does not match: a second
  claim on an active task, a complete on a pending task, a verdict on a
  task not in review, a task memo that reuses an id, an accept or brief
  from a wallet that did not fund the task — the row does not move the
  state. The fold never reads outside the project's log: the board is
  keyed by project and task, nothing else.

- **Dissent**: `reject` is the dissent verb, honoured from anyone who paid
  for the memo. A short arrives in a later PR as `dissent`: reject plus a
  small vault-routed short on the project.

- **Lease**: a claim carries an expiry — `now - claimed_at > Lease`
  (24h, the todo store's stale-claim window) — and it applies only while
  the claim is the task's live state: after the fold, an active task whose
  claim is older than the lease folds as pending (owner and claimed_at
  cleared), while a claim superseded by complete/accept/reject is never
  dropped. The fold's expiry is the board's lease; `Reap` is the door that
  materializes it (the projection is rebuilt with `now`, so an expired
  active row returns to pending).

- **The store**: the local SQLite log caches the chain's message rows (the
  indexer's message schema verbatim, generated with lift from
  `board/metadata`): `messages` keyed `(mint, seq)`, `tasks` keyed
  `(project, id)`, `notes` keyed `(project, task, seq)`, `meta` for the
  schema version. The reads are schema-shaped and primary-key-seek only —
  `GetTask(project, id)`, `WindowTaskByProject`, `GetMessage(mint, seq)`,
  `WindowMessageByMint` — no index, no scan, no seek beyond the key. The
  one index is `messages (mint, signature)` unique: the write-side
  idempotency door (a memo already cached is never applied twice); it is
  never a read path. `tasks`/`notes` are a disposable projection rebuilt
  from the message log inside every transaction, exactly like the todo
  store's. The cache is the board's window: each sync adds the newest
  messages (100 per sync) and the fold covers what the cache holds; the
  chain's slot and timestamp replace the local approximation on the next
  sync.

- **The seam**: the board store implements rig's board seam — the swarm
  surface (claim / note / complete / accept / reject / reap over a
  `Project`) with the same vocabulary the todo store's swarm drains, so a
  chain board can be drained with no change to rig. `claim = the memo`:
  the claim is written on-chain (memo + micro buy), the lease is the
  fold's expiry, and `reap` returns expired claims to pending. The drain
  sequence is unchanged: claim → work → complete (submits for review) →
  accept/reject; a dead worker's claim is released by the reap door. The
  surface has no roles: any wallet may claim, note, complete, and reject;
  accept lands only from the task's funder (the fold decides).

- **The board tool**: `board` reads a project's board (sync → fold →
  render) and acts: task, brief, claim, note, complete, accept, reject —
  each act is one memo + one vault-routed micro buy (0.001 SOL, the memo
  buy). Any wallet may act; ownership and stake are the fold's rules.
  Every write replies with the tx signature plus the memo.

- **RPC scan**: with the indexer unset, the board reads the chain
  directly: `getSignaturesForAddress` on the project's bonding curve (and
  deep pool, migrated), then `getTransaction` per signature; the memo
  instruction is decoded (memo program, data = UTF-8), the sender is the
  tx's first account key, the timestamp is the block time, and the action
  kind is the torch instruction discriminator co-resident in the same tx
  (buy/sell/swap). Messages dedupe by signature. The RPC-only market row
  for a write comes from the same seam: the curve account decode
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
the funder's accept counts (briefs too). Claim, note, complete, and
reject are honoured from anyone who paid for the memo; a second claim, a
complete on a pending task, a verdict on a task not in review, and an
accept from a non-funder are foreign rows and are skipped.

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
- **Board read without a key**: `orbit board <mint> read` loads config in
  read mode — ORBIT_RPC only, no indexer, no vault creator, no agent key
  (the config test pins the loader; the RPC scan fixture pins the read).
