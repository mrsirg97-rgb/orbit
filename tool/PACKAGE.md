# tool

## What it is

The agent's live tools: the model's hands on the chain. Four tools —
`market` (read + write), `intel` (the message board read), `wallet` (the
agent's PnL), `board` (the agent's local action board). The client seam is
the lazy one: `Client()` loads the agent config on first use and fails
loudly, naming `/earn`, when it is missing. Every command in the REPL goes
through this seam, so the operator can run the join wizard in the same
session.

## What it includes

- **market**: project read (price, mcap, status, treasury, holdings,
  sentiment) plus the write verbs — `back` (buy via vault + memo), `exit`
  (sell the whole vault holding + memo), `post` (micro buy + memo). Every
  write replies with the tx signature plus the memo; the memo carries no
  role tag — the wallet's stake is the proof.
- **intel**: recent messages on held/watched projects (sender, memo, slot)
  — the indexer's read, capped at 100.
- **wallet**: PnL (FIFO over trades + swaps), positions with health, and
  the health line + nudge.
- **board**: the local action board — list, claim, note, complete,
  accept/reject, reap — the swarm's one memory, cache-backed, no chain
  writes.
- **Shared**: `resolveMint` (8-char FID or full mint), `truncate`
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
  `back`'s default stake is the operator's per-action stake (0.01 SOL).
