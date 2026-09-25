# orbit: the torch client

A Go client for torch_market v21 (deep-pool integration, program
`FghCwWojts9MbU3Pmog5peacaKrEYM5n1T68KWHy7TAh`, IDL 21.0.0) and its indexer.
Read side: the indexer HTTP API, the `/events` websocket, and the `/rpc`
JSON-RPC passthrough. Write side: vault-routed buy/sell instructions built
from the IDL, signed by the agent hot wallet, with an SPL memo attached.
Devnet only. The operator's vault authority key never enters the process.

## definition

- **Read**: markets, messages, trades, positions, user P&L — typed rows that
  mirror `indexer-core/src/contracts.rs` exactly. Reads go through the
  indexer base URL (`ORBIT_INDEXER`); account/balance reads (vault SOL,
  holdings, treasury float) go through the JSON-RPC seam (`ORBIT_RPC`, which
  may be the indexer's `/rpc` proxy or a direct RPC).
- **Write**: `buy_via_vault` / `sell_via_vault` (bonding curve) and
  `vault_swap` (migrated, DeepPool) — chosen by market status — plus the
  SPL memo instruction in the same transaction. The signer is the agent hot
  wallet (`ORBIT_AGENT_KEY`), which must be linked to the operator's vault
  (`ORBIT_VAULT_CREATOR`). Every write returns the transaction signature and
  the memo.
- **Boundaries**: amounts in lamports / raw token units (6 decimals);
  strings are UTF-8; pubkeys are base58; the client never formats currency.

## decisions

### 1. One config struct, env-only, fail closed

`Config` is loaded from the environment at startup:

Three loaders share one resolver, one required set per mode:

| loader | indexer | rpc | vault creator | agent key | writes |
|---|---|---|---|---|---|
| `LoadConfig` (agent runtime) | yes | yes | yes | yes | devnet gate |
| `LoadOperatorConfig` (vault/project operator) | yes | yes | yes | no | devnet gate |
| `LoadReadConfig` (project list) | yes | yes | no | no | off |

| env | required | meaning |
|---|---|---|
| `ORBIT_INDEXER` | yes | indexer API base URL (e.g. `https://torch-api.example`) |
| `ORBIT_RPC` | yes | Solana JSON-RPC base URL (may be `<indexer>/rpc`) |
| `ORBIT_PROGRAM_ID` | no | torch program ID; default = the IDL address |
| `ORBIT_VAULT_CREATOR` | operator + agent runtime | the vault creator pubkey (operator's identity, public only) |
| `ORBIT_AGENT_KEY` | agent runtime only | base58 64-byte secret key of the agent hot wallet |

Reads never need a signing key — a stranger browsing projects before
deciding to join is the case. Missing required env, an unparseable key (a
present `ORBIT_AGENT_KEY` is validated in every mode), or a non-devnet
program ID is a startup error. "Devnet only" is enforced structurally:
`Config` carries an explicit `AllowWrite bool` that defaults false; the
operator and agent loaders flip it only when the program ID equals the
devnet IDL address. `ORBIT_AGENT_KEY` is never logged or echoed.

### 2. The IDL is embedded; instruction bytes are derived, not hand-coded

`idl/torch_market.json` is `//go:embed`-ed and parsed once at init. The
builder reads the instruction `discriminator` bytes (8) and the account
list directly from the IDL; PDA seeds and program-id are checked against a
small table of constants derived from `programs/torch_market/src/constants.rs`
(the IDL's pda seed entries are also parsed, and must agree). A builder for
an instruction the IDL does not name is a compile-time/init error, never a
runtime discovery.

Borsh args are encoded positionally per the IDL `types` definitions —
`BuyArgs{u64 sol_amount, u64 min_tokens_out}`, `SellArgs{u64 token_amount,
u64 min_sol_out}`, `CreateTokenArgs{string name, string symbol, string uri,
u64 sol_target, bool community_token}` — u64 little-endian, `bool` as one
byte, `string` as a u32-length-prefixed UTF-8 payload.

### 3. Curve vs DeepPool is decided by market status

`buy_via_vault`/`sell_via_vault` are valid while `status ∈ {BONDING,
COMPLETE}`; once `status = MIGRATED` (or `deep_pool_pubkey` is set) the
write path is `vault_swap(amount_in, minimum_amount_out, is_buy)`. The
decision is a pure function `Route(market) → Instruction`, so the tool layer
can quote before it routes. A market whose status is RECLAIMED refuses
writes.

### 4. Quote math is a pure mirror of the program

The slippage guard is computed client-side from the same formulas as
`programs/torch_market/src/{math,market}.rs`:

- buy: protocol fee = `sol * 50 / 10000`; treasury rate decays
  `1750 → 250` bps by `real_sol / bonding_target`; creator rate grows
  `20 → 100` bps (0 for community tokens); `sol_to_curve =
  sol_after_fees * (10000 - treasury_rate) / 10000`; `tokens_out =
  virtual_tokens * sol_to_curve / (virtual_sol + sol_to_curve)`;
  `min_tokens_out = applyBps(tokens_out, slippage)`.
- sell: `sol_out = virtual_sol * tokens / (virtual_tokens + tokens)`;
  `min_sol_out = applyBps(sol_out, slippage)`.
- DEX: quote from pool reserves `sol_reserve/token_reserve` via the
  constant-product formula, same single slippage knob.

All arithmetic is checked (overflow → error), never wrapping. Default
slippage `100` bps (1%), the SDK's single knob (prompt-008 F-3).

### 5. The RPC seam is an interface; the fake is the test double

`RPC` is the one seam to the chain:

```go
type RPC interface {
    GetLatestBlockhash(ctx) (string, error)
    SendTransaction(ctx, base58 string) (string, error)
    GetAccountInfo(ctx, pubkey string) (AccountInfo, error)
    GetTokenAccountsByOwner(ctx, owner, programID string) ([]TokenAccount, error)
    GetBalance(ctx, pubkey string) (uint64, error)
    GetSignatureStatus(ctx, sig string) (SignatureStatus, error)
}
```

`jsonrpc` implements it over `POST {base}/rpc` (the indexer's passthrough,
or a direct node). Tests use a fake that returns canned replies — no
network, no keypair needed to read. Writes in tests use a fake that records
the signed tx bytes and returns a canned signature, so the builder and the
signer are exercised end to end without devnet.

### 6. Transactions are signed client-side; the worker path sends one tx

The client builds a v0 legacy-style message (account keys, blockhash,
instructions), signs with the hot wallet's ed25519 key, and sends the base58
signed transaction. `SendTransaction` returns the signature, which is also
the reply to every write. The hot wallet's public key is derived from the
secret; only the linked-wallet constraint can reject a tx on-chain (the
vault link must exist — see §7).

### 7. Vault bootstrap never touches the operator key

`create_vault`, `link_wallet`, `deposit_vault`, `withdraw_vault`,
`transfer_authority` build **unsigned** instruction sets. `orbit bootstrap`
prints them as base64-encoded serialized transactions (one per line) for
the operator to sign and send with the vault authority key; the process
holds no operator secret. The write path assumes the vault exists, the hot
wallet is linked, and the vault has SOL (`vault_sol` lamports − rent ≥
`amount_in`); a missing link or an empty vault fails with a named error and
the client never retries blindly.

### 8. The websocket is a typed stream, resync is a sentinel

`Events` connects to `/events`, subscribes to `all` and/or `market:<mint>`
rooms (≤ 8, the server cap), and decodes frames by `kind` into the same
row types as the HTTP reads. A `resync` frame or a lagged room surfaces as
`ErrResync` — the consumer re-reads state rather than treating the gap as
data. Reconnect is the caller's loop; the client fails loudly on a bad
frame instead of skipping it.

## layout

- `client/config.go` — env config, devnet-only write gate
- `client/idl.go` — embedded IDL + parsed instruction/account metadata
- `client/pda.go` — PDA derivations (checked against the IDL seeds)
- `client/quote.go` — pure buy/sell/swap math
- `client/ix.go` — instruction builders (curve, vault_swap, memo, vault admin, create_token)
- `client/tx.go` — transaction assemble + sign + send
- `client/rpc.go` — `RPC` interface + `jsonrpc` implementation
- `client/api.go` — indexer HTTP reads + typed rows
- `client/events.go` — `/events` subscribe/decode
- `client/client.go` — `TorchClient`: read + write + route
- `client/*_test.go` — httptest fixtures, fake RPC, IDL-builders
- `testdata/` — recorded API fixtures (JSON), the IDL

## tests

- API reads against recorded fixtures (markets detail, messages, positions,
  user PnL) — field-by-field, not snapshot-only.
- Instruction builders against the IDL: byte-exact discriminator + args
  (golden hex), account order, PDA derivations matching the program seeds.
- Quote math against known program outputs (the pure functions' closed
  forms) and the recorded trade fixtures.
- Write path with a fake RPC: signed tx bytes contain the expected keys and
  the memo instruction; the reply carries the signature + memo.
- Config: missing env, bad key, non-devnet program ID → named failures.
- WS decode: every `kind` frame + resync, from a fixture; a malformed frame
  fails loudly.
