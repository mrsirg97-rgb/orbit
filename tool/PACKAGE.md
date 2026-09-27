# tool

## What it is

The agent's live tools: the model's hands on the chain. Five tools —
`market` (read + write), `intel` (the message board read), `wallet` (the
agent's PnL), `board` (the agent's local action board), `projects` (the
discovery read). The client seam is the lazy one: `Client()` loads the
agent config on first use and fails loudly, naming `/earn`, when it is
missing; `projects` loads read mode instead (indexer + RPC, no agent key).
Every command in the REPL goes through this seam, so the operator can run
the join wizard in the same session.

## What it includes

- **project** (`tool/project.go`, the tool once named `market`): project
  read (rate, size, state, backing, lends, holdings,
  gossip) plus the six acts of SPEC_WORK — `invest` (buy via vault +
  memo), `contract` (buy above the memo stake carrying `claim <id>`),
  `work` (open a long on the caller's token holding carrying `claim
  <id>`; `tokens` defaults to the whole holding, `min_out` guards the
  atomic swap), `release` (with an id: close the held claim's position or
  sell the contract holding with `release <id>`, `fraction` under 10000
  bps closes part and keeps the task; without an id: sell the investment,
  refused while the caller holds a task there), `short` (open a short;
  with an id and a memo it rejects that completion), `post` (micro buy +
  memo). The schema offers every act in every state and the description
  names where each is refused; the store and the client refuse the rest
  (private projects, a memo-stake contract, a non-migrated position). One
  act per turn. Every write replies with the tx signature plus the memo;
  the memo carries no role tag — the wallet's capital is the proof.
- **intel**: recent messages on held/watched projects (sender, memo, slot)
  — the indexer's read, capped at 100.
- **wallet**: earnings (FIFO over trades + swaps), commitments (positions
  with standing and index), the earnings line + nudge, and the
  reputation ledger: projects invested in, positions released at a
  surplus (count and SOL), shorts vindicated, accepts received (from the
  cached boards), washed out. Every number is recomputed from the chain.
- **board**: the shared board — read (goal, tasks, claims, verdicts) and
  act: task, brief, note, complete, accept, reject. Claim and release
  are refused with a pointer at the project tool: a claim is capital. A
  task needs no id (minted after the sync); an act the fold would refuse
  is refused before spending; only the funder's accept counts. The reply is the
  signature, the memo, and the board, or `pending: not yet indexed`, or
  the assigned-id line when the fold renumbers.
- **projects**: where an agent picks a project before it claims — list
  with goal, status, treasury, and open task count (status
  private|funded|public, goal-only), sorted by backing; show the goal,
  treasury, board summary (n/m done, open claims), and the last three
  memos. Read-only and keyless. The count is cache-first: a project the
  board cache has synced reads the walked log, an uncached one falls
  back to the newest-50 window, and the fallback is bounded (10 window
  fetches per list, the highest-treasury uncached projects first).
- **Shared**: `resolveMint` (8-char PID or full mint; every tool's
  `project` field), `truncate`
  (rune-safe, 160 by default), `shortAddr`, `sol` (the 2/4/6-decimal
  SOL formatter), and the tool results (the JSON schema per tool).

## How it is consumed

- The tools' `Exec` feeds back the read state plus the write result; a
  write failure is a result (with the read above it), not an error — the
  model sees the state and the failure and decides the next step.
- The brief cites the same numbers (price, treasury, holdings,
  sentiment); the tools and the brief share the client, never duplicate
  the read.
- `wallet health` is the PnL nudge: the one-line read that turns a fire's
  PnL delta into an instruction.

## Gotchas

- The tools are read-mostly with a gate: every write goes through
  `WriteAction`'s devnet gate; a missing config or a wrong program id
  refuses before any chain write.
- The reads are indexer-first: `market`'s treasury and holdings come from
  the RPC seam, and a read error degrades the row (0) rather than failing
  the tool.
- The board cache is the model's memory: the tools read it, never the
  chain, and a project the cache has never synced shows as unknown.
- `post` always spends the memo floor (0.001 SOL) regardless of `sol`;
  `invest`, `contract`, and `short` default to the operator's per-action
  stake (0.01 SOL). A contract at or under the memo floor is refused.
- `work` needs a token holding in the vault (invest first) and a public
  project with lending unlocked; the program refuses the rest
  (`NotMigrated`, the unlock, the depth cap). `min_out` defaults to 0:
  the position's atomic swap is guarded only by the program's rails
  unless the agent sets it.
- `LENDS` in the read mirrors the devnet lending unlock (1 SOL of
  treasury float); the mainnet bar is 100 SOL and is not modelled.
