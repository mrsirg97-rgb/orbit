# Changelog
## [0.2.1] — the wizard checks before it signs

`/earn` resolved the operator key at the top of the vault create step,
before it checked what needs doing. The wizard now checks the vault
against the recorded creator first and resolves the operator key only at
the step that actually signs (create, link, deposit); a rerun where the
vault, the link and the deposit already exist registers roles with no
key.

The operator configs also stop reading the agent key file: `create` and
the read paths never needed the hot key, so a missing `ORBIT_AGENT_KEY_FILE`
no longer blocks them.

Tests: a wizard run on an existing setup with no `--operator-key-path`
succeeds and sends nothing.

## [0.2.0] — the hygiene pass: nine fixes, each with a test

- **Task rows order numerically** — the board window and render were
  sorted by the task id as text (t1, t10, t2); the window now orders by
  the numeric id.
- **Memo text renders raw — control characters rejected at parse** —
  newlines, tabs, escapes, and the other C0/DEL bytes never become a
  board memo (parse and the write side refuse).
- **resolveMint accepts only real mints** — a 44-character input must
  decode with `sol.Decode` to 32 bytes before it is treated as a mint
  (board and market); an invalid base58 string resolves by FID instead.
- **The legacy env file has the lowest precedence** — it is read into the
  map (live env > env file > config file), never into the process
  (`os.Setenv` is gone).
- **A user-set ORBIT_RPC is used verbatim** — only the indexer-derived
  RPC gets `/rpc` appended; a bare-host ORBIT_RPC no longer gains one.
- **The cron command is shell-quoted** — `RunnerCommand` quotes the
  executable path in the crontab line (spaces and metacharacters are
  safe); all run-job registration sites use it.
- **The job cwd is pinned to the orbit home** — agent jobs no longer
  follow the TUI's cwd.
- **Cost basis is one unit** — the brief and the tools both treat
  `cost_basis_remaining` as lamports (the tools convert to SOL with
  `/1e9`, not `/1e6`).
- **`orbit -update`** — ported from rig's updater: fetches the latest
  `mrsirg97-rgb/orbit` release, verifies `checksums.txt` against the
  pinned minisign key (`ORBIT_UPDATE_KEY` or `settings.json updateKey`;
  unpinned refuses — the rig embedded key is not orbit's), replaces the
  running binary in place, and prints old and new versions.

## [0.1.7] — the fire's brief carries the goal; the sandbox comes from settings

`run-job` built the fire's brief from the agent row's `Name` and `Bio`
only — the architect's `Goal` never appeared in a live brief. The fire
now builds the brief from `Name`, `Bio`, and `Goal`, so the `GOAL:` line
is in every fire's brief.

The sandbox and the worker swap URL came from `ORBIT_SANDBOX` /
`RIG_SWAP_URL` env only, with the swap URL hardcoded to
`http://127.0.0.1:8090` in the run-job path. The fire now reads
`settings.json` the way main does (`config.Load`) and the env overrides:
`ORBIT_SANDBOX` > `settings.json sandbox` > off, `RIG_SWAP_URL` >
`settings.json swapUrl`. No hardcoded swap URL.

Tests: a fire from a row with a goal prints the `GOAL:` line, and a fire
with `settings.json` sandbox on passes the sandbox to the runner (env
overrides both settings keys).

## [0.1.6] — the act waits for its memo to be indexed

`WriteAction` returns at send time and the indexer lags, so the act's
immediate re-sync missed the memo: the reply board omitted the act and
the next act's fold pre-check refused it — a retried task double-spent.

- **Confirm, then wait for the cache** — after the write the act polls
  `GetSignatureStatus` until confirmed (bounded, now the shared
  `client.WaitConfirmed`), then re-syncs until its signature is in the
  cache (bounded, 15s default). If the memo does not land, the reply is
  `<sig> <memo>` plus `pending: not yet indexed` — the write is
  confirmed, only the cache is behind.
- **The reply names a renumbered id** — when the fold renumbers a stale
  task, the reply says which id was assigned, so a follow-up claim does
  not target someone else's task.

Tests: task then claim with the fake's indexer lag 2 succeeds with one
task on chain (no retry double-spend), and the contested-verdict test
now asserts the pending reply for a memo the cache has not seen. The
shared `WaitConfirmed` replaces the project and earn copies.

## [0.1.5] — a board act never guesses the memo's seq

An `Act` inserted its own memo row with `seq = max + 1` and `Sync` never
renumbered it, so a writer's memo ordered before others' and a
`(mint, seq)` collision wedged the project.

- **No local row** — the act writes the memo, then re-syncs: the memo's
  seq is the chain's order, never a local guess.
- **The task id is minted after the sync**, not before, and the fold
  renumbers a task memo whose id is already taken (a stale cache minted
  the same id) to a fresh id in log order.
- **Refuse before spending** — before the write, the act folds the cache
  plus the candidate memo and refuses what the fold would refuse (a
  foreign state, an accept from a non-funder, a stale id) without a
  chain spend.

Tests: two clients folding the same contested verdict agree (both
verdicts land, no `(mint, seq)` wedge), a task from a stale cache gets a
fresh id, and an accept from a non-funder is refused before spending.

## [0.1.4] — the wizard is safe to rerun; the RPC decoders match a real node

The first `/earn` runs after the vault fix found two rerun and wire bugs.

- **earn wizard rerun safety** — deposit is gated on the recorded
  `ORBIT_VAULT_DEPOSITED` marker or the vault record's `total_deposited`
  (never the running balance), link and deposit confirm the signature
  before returning, vault create checks `TorchVaultPDA(creator)` on chain
  and records the creator without sending when the vault exists, a role
  whose job exists is refreshed instead of created, and the model check
  runs before any chain spend.
- **RPC decoders** — `requestAirdrop` returns a bare string signature,
  not `{"value": ...}`; `getSignatureStatuses` uses `confirmationStatus`
  (confirmed/finalized), with `confirmations` null once finalized.
- **The memo cap** — `CurveMemoCap` was 500 and overflowed the 1232-byte
  legacy limit on a curve buy with an ATA. It is now pinned from a
  `sol.Compile` measurement of the worst case (295 bytes, not runes; a
  test re-measures and fails if the builder drifts, so no startup cost),
  and the board and project goal caps count bytes.

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
