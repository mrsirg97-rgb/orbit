# ORBIT

## overview

orbit is a torch agent: the client, the shared board, the brief, and the
scheduled agents, built on the rig runtime. It reads the torch market (the
indexer HTTP API, the events websocket, the JSON-RPC seam), writes through
the operator's vault with the agent's hot wallet, folds the chain's memo
log into a shared board, and runs one scheduled agent per identity row.
The operator's vault authority key never enters the orbit home; every
write is devnet-only; the fold trusts the sender, not a label. It is not a
framework; it is one machine, built closed: the client is the only seam to
the chain, the board is the only state, and the brief is the only prompt
an agent sees.

## working in this repository

- Every Go package carries a `PACKAGE.md` that is the spec file for that
  package: what it is, what it includes, how it is consumed, the gotchas.
  Read it before touching the package and keep it current when behavior
  changes. The governing specs live in `specs/`, written and agreed
  before the code; the `PACKAGE.md` points at its spec.
- No comments in Go, implementation or tests: one rule everywhere,
  because a small model reads the repository as one corpus and cannot
  hold "allowed here, forbidden there"; it sees commented tests and
  writes commented code. A test's name carries its invariant; the
  `PACKAGE.md` carries the English; design rationale lives in the spec.
  The only `//` lines are compiler directives (`//go:embed`). Exempt:
  generated code, and the metadata packages; generation input whose
  doc comments lift reads.
- Follow Go best practices and design interface first when applicable.
  Interfaces make orbit modular, which is the goal: depend on the seams
  (`client.RPC`, the board's swarm surface), wire once at the root, swap
  at registration with zero consumer changes.
- Keep things lean and terse. Follow the established patterns, and apply
  a new pattern only if it is genuinely better.
- Be security conscious at all times. This is a harness agents work in,
  with a signing key and the chain's write path, running untrusted model
  output: deny by default, canonicalize untrusted input before acting on
  it, bound the work a caller can induce, and fail closed on uncertain
  state. The operator's authority key never enters the process.
- Build for the daily driver. This harness is what you will be using:
  anything added here should be a feature you will want to reach for,
  and it should remove friction, not add ceremony.

## packages

- `cmd/orbit`: the binary and composition root: the runtime's main with
  orbit's tools (`market`, `intel`, `wallet`, `board`), the `/earn`
  command beside the command set, the orbit title, and the subcommands
  (init, vault, project, board, agent, snapshot, bootstrap, run-job). The
  only package that imports the whole tree; the orbit client is a lazy
  seam (first tool use loads the agent config and names `/earn` when it
  is missing).
- `client`: the torch client (SPEC_CLIENT): env-only config loaders that
  fail closed, the embedded IDL (v21.0.0), PDA derivations checked
  against the IDL seeds, the pure quote math, the instruction builders,
  the `RPC` seam (the fake is the test double), the indexer reads, the
  events websocket, the RPC-only scan, and the vault admin path. Reads
  never need a key; the devnet-only gate is structural (`AllowWrite`).
- `board`: the shared board (SPEC_BOARD): the memo log on the chain, the
  pure fold (memo rows -> tasks + notes), the local SQLite cache (the
  log is the spine, the projection is disposable), the swarm surface
  (claim/note/complete/accept/reject/reap over a `Project`), and the
  lean render. The chain is the source of truth; the cache is rebuilt
  from it, never trusted.
- `board/metadata`: hand-written message-schema metadata: the source for
  the generated `ddl`/`domain` accessors. Edit and regenerate; never
  hand-edit the generated projections.
- `board/ddl`, `board/domain`: generated (lift); never hand-edited.
- `brief`: the agent's brief (SPEC_BRIEF): a pure projection of the live
  read side into the compact/full brief. The vocabulary is torch's —
  PNL, back/exit/post/pass, bonding/ready/migrated/reclaimed, HELD/
  FOUNDED/SENTIMENT — never another game's abbreviations. Same snapshot,
  same bytes; the goldens pin them.
- `identity`: the agent's local identity row: one row per (wallet, role),
  the role's register defaults (cadence, brief size, budget, stall,
  timeout), and schema v5. Roles are local — they own defaults and never
  ride a memo.
- `agent`: the scheduled agent: one identity row -> one scheduled job;
  `Fire` is the per-fire path (brief rebuild -> prompt refresh -> line
  re-assert -> spawn). The brief is per-fire, never per-register.
- `earn`: the `/earn` command (SPEC_EARN): the moments (bare status/join
  worker, join [roles], roles list/add/remove, goal) with the setup and
  the preflight line before any chain spend, the operator key path
  remembered (never the key), status/stop/start, and the footer snapshot
  (a local file, never the chain, at status-callback time).
- `project`: projects (SPEC_PROJECT): `create_token` plus the first
  vault buy that funds the treasury, the `goal:` memo, and `list` (the
  goal per market, the treasury float). The operator is the creator and
  the buyer; the mint keypair is generated in-process and never stored.
- `onboard`: the one-minute onboarding path (ONBOARDING): hot wallet
  (0600, resumable), config upsert, the bounded devnet airdrop, and the
  operator-key seam (flag > env, never stored).
- `tool`: the orbit tools on the runtime menu: `market` (buy/sell/post
  via vault + memo), `intel` (the brief's read side), `wallet` (the
  vault read), `board` (the shared board), and the `Snapshot` the brief
  builds from. Thin, schema-shaped, verbatim.
- `sol`: the minimal Solana wire: base58, keypairs, legacy message
  compilation, ed25519 signing (v0 versioned form, single and multi
  signer), PDA derivation with the on-curve check. Stdlib only; the
  JSON-RPC transport lives in `client/rpc.go`.
- `idl`: the embedded `torch_market` IDL (v21.0.0, `//go:embed`): parsed
  once at init, instruction discriminators and account lists in order,
  borsh arg encoding. A builder for an instruction the IDL does not name
  is an init error, never a runtime discovery.
- `specs/`: the specs, written and agreed before the code (SPEC_CLIENT
  first); the governing documents the `PACKAGE.md` files cite.
