# Changelog
## [0.1.3] — the board lease expires only the live claim

The fold's claim case skipped any claim older than the lease
unconditionally, so a task claimed, completed, and accepted in one hour
folded as done today and pending tomorrow. The lease now applies only
while the claim is the task's live state: after the fold, an active task
whose claim is older than the lease returns to pending, and a claim
superseded by complete/accept/reject is never dropped.

## [0.1.2] — the TUI title follows rig's letterforms

The `orbit` art rows didn't match rig's letterforms: `o` had no counter,
`r` and `b` were identical, `i` was two bars, and `t` was a block. The
rows now use rig's letterforms in the same 3-row shape, and the ASCII
fallback name stays `orbit`.

## [0.1.1] — vault create on a clean home

The first real `/earn` run on a clean home found three create-path bugs,
fixed and pinned by tests.

- **wizard vault create** — the create step loaded read mode, so the write
  gate was never on and `SendVaultIx` refused. It now loads the operator
  create config (writes on, no prior creator) and derives the creator from
  the operator key named at the call.
- **`orbit vault create`** — `LoadOperatorConfig` demanded
  `ORBIT_VAULT_CREATOR`, but create is the step that sets it. Create now
  derives the creator from the operator key and needs no prior creator;
  link, deposit, and withdraw keep the requirement.
- **the roles hint** — the "which roles?" error now names `--goal`, which
  an architect requires.

## [0.1.0] — initial release

The torch agent on the rig runtime: the client, the shared board, the
brief, and the scheduled agents.

- **client** — the torch client (SPEC_CLIENT): env-only config loaders
  that fail closed, the embedded IDL (v21.0.0), PDA derivations checked
  against the IDL seeds, the pure quote math, the instruction builders,
  the `RPC` seam (the fake is the test double), the indexer reads, the
  events websocket, the RPC-only scan, and the vault admin path. Reads
  never need a key; the devnet-only gate is structural (`AllowWrite`),
  and the operator's authority key never enters the process.
- **board** — the shared board (SPEC_BOARD): the memo log on the chain
  is the source of truth, the pure fold (memo rows -> tasks + notes) is
  a deterministic projection, the local SQLite is a cache rebuilt from
  the log and never trusted, and the swarm surface
  (claim/note/complete/accept/reject/reap over a `Project`) drains with
  no change to the runtime.
- **brief** — the agent's brief (SPEC_BRIEF): a pure projection of the
  live read side into the compact/full brief, torch's vocabulary (PNL,
  back/exit/post/pass, bonding/ready/migrated/reclaimed,
  HELD/FOUNDED/SENTIMENT), both sizes pinned to the byte by goldens.
- **identity + agent** — one row per (wallet, role) with the role's
  register defaults (cadence, brief size, budget, stall, timeout), and
  one scheduled job per row; each fire rebuilds the brief, refreshes the
  prompt, and runs one-shot.
- **earn** — the `/earn` command (SPEC_EARN): the register wizard (init
  and the vault steps when missing, the operator key named at the call),
  status/stop/start, and the footer snapshot (a local file, never the
  chain).
- **project** — projects (SPEC_PROJECT): `create_token` plus the first
  vault buy that funds the treasury, the `goal:` memo, and `list` (the
  goal per market, the treasury float).
- **onboard** — the one-minute path (ONBOARDING): hot wallet (0600,
  resumable), config upsert, the bounded devnet airdrop, and the
  operator-key seam (flag > env, never stored).
- **tool** — the orbit four on the runtime menu: `market` (buy/sell/post
  via vault + memo), `intel` (the brief's read side), `wallet` (the
  vault read), `board` (the shared board), and the `Snapshot` the brief
  builds from.
- **sol + idl** — the minimal Solana wire (base58, keypairs, legacy
  message compilation, ed25519 signing v0, PDA derivation with the
  on-curve check) and the embedded `torch_market` IDL (v21.0.0, parsed
  once at init, borsh arg encoding). Stdlib only.
- **build + release** — CI (go vet, go test -race -p 2, make fmt-check,
  shellcheck install.sh) and the tagged release workflow: the tag is
  asserted against the `Version` const before any asset is built, the
  four binaries are cross-built with `CGO_ENABLED=0` and checksummed,
  each asset is signed with the pinned minisign key, provenance is
  attested, and the matching CHANGELOG section is the release body. The
  installer verifies the checksum before anything moves.
