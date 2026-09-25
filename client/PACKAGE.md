# client

## What it is

The torch client: the one seam to the chain. Read side: the indexer HTTP
API, the `/events` websocket, and the JSON-RPC passthrough. Write side:
vault-routed buy/sell/swap instructions built from the IDL, signed by the
agent hot wallet, with an SPL memo attached. Devnet only — the gate is
structural (`AllowWrite` flips only for the devnet IDL address). The
operator's vault authority key never enters the process. SPEC_CLIENT
governs.

## What it includes

- **Config loaders** (`config.go`): env-only, fail closed; `Home` resolves
  the orbit home (`RIG_HOME`, default `~/.orbit`), and six loaders share
  one resolver with one required set per mode (agent runtime, operator,
  vault create, read, board write, board read). Reads never need a signing
  key; a present-but-bad key still fails closed.
- **The embedded IDL** (`idl/`): parsed once at init — discriminators,
  account order with signer/writable flags, borsh args. A builder for an
  instruction the IDL does not name is an init error, never a runtime
  discovery.
- **PDA derivations** (`pda.go`): the program's seed table, checked
  against the IDL's pda seeds by the client tests.
- **Quote math** (`quote.go`): the pure mirror of the program — curve
  buy/sell splits (protocol fee, dev share, treasury decay, creator
  growth), DeepPool swap fees, the single slippage knob, all checked and
  never wrapping.
- **Instruction builders** (`ix.go`, `vault.go`): curve buy/sell via
  vault, `vault_swap` (DeepPool), `create_token`, the SPL memo, the vault
  ATA create, and the operator's admin set (create/link/unlink/deposit/
  withdraw — built unsigned, sent by `SendVaultIx` with the operator key).
- **The RPC seam** (`rpc.go`): the `RPC` interface plus `JSONRPC` over
  POST `{base}/rpc`; the fake is the test double. It also carries the
  airdrop, the signature status poll, and the board's chain scan calls.
- **Indexer reads** (`api.go`): typed rows mirroring `indexer-core`
  `contracts.rs` — markets, trades, messages, positions, liquidations,
  migrations, swaps, user PnL.
- **The websocket** (`events.go`): one room per connection (`all` or
  `market:<mint>`; a second room refuses by name rather than being
  dropped), frames decoded by kind, a lagged room surfaces as
  `ErrResync`.
- **The RPC-only scan** (`scan.go`): the board's chain fallback —
  `getSignaturesForAddress` on the curve, `getTransaction` per signature,
  memo decoded, sender = the tx's first account key, action kind from the
  co-resident torch instruction, dedupe by signature, rows in chain order.
- **`TorchClient`** (`client.go`): read + write + route. `WriteAction`
  routes by market status: curve while BONDING/COMPLETE, `vault_swap`
  when MIGRATED, refuse when RECLAIMED.

## How it is consumed

- The tools (`market`, `intel`, `wallet`, `board`), the board store, the
  brief's snapshot, the subcommands, and the earn wizard all drive
  `TorchClient`.
- The board reads the chain directly when the indexer is unset:
  `MarketFromRPC` (curve decode + treasury community flag + global config)
  and `ScanMessages`.
- The wallet read (`WalletRead`) is PnL (with vault attribution) +
  holdings (agent ATA + vault ATA) + vault SOL (lamports − rent) + the
  agent balance.

## Gotchas

- The gate: `AllowWrite` flips only when `ProgramID == DevnetProgramID`;
  every write path (`WriteAction`, `SendVaultIx`) re-checks it, and a
  non-devnet program refuses at load.
- The agent hot key is the only secret the process holds. The operator
  key is resolved per call (flag > env) in `onboard`, never stored.
- Memo caps: curve 500 chars, swap 280 — the transaction stays under the
  legacy 1232-byte limit.
- Amounts are lamports / raw token units (6 decimals); the client never
  formats currency except `FormatSOL`, the read-only display form.
- The indexer is optional: reads fall back to the chain through the RPC
  seam, and the board works with `ORBIT_INDEXER` unset.
- The websocket is one room per connection; reconnect is the caller's
  loop, and a bad frame fails loudly instead of being skipped.
