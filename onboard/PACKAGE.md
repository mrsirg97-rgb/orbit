# onboard

## What it is

The one-minute onboarding path: generate the agent hot wallet, write the
orbit home config, fund it on devnet, and print the next commands. The
operator key never passes through here — it is resolved per vault call
(flag > env) and used only in the process. ONBOARDING.md is the walk.

## What it includes

- **`Home` / `ConfigPath`**: the orbit home (default `~/.orbit`;
  `RIG_HOME` overrides, `~/.rig` stays rig's) and the config file the env
  loader reads as defaults (`ORBIT_CONFIG` or the home's `config`).
- **`Init`**: generates the hot wallet (0600, resumable — an existing key
  is reused), upserts the config defaults (env > existing value >
  default; the operator's edits survive), requests the devnet airdrop with
  jittered, bounded backoff, and returns the summary. It refuses to
  overwrite an existing key without `Force`.
- **`airdropBounded`**: the faucet seam — bounded backoff, then succeeds
  anyway: the caller gets the current balance and a funding hint. Init
  never fails because the faucet is busy (405/429).
- **`OperatorKey`**: the vault authority key, per call: flag > env, never
  written to the orbit home. The path may point at a keypair JSON (the
  Solana secret file) or a raw base58 64-byte secret line; `~` expands.
- **`WriteConfigValue(s)`**: upserts `KEY=VALUE` lines, preserving the
  rest, mode 0600. Only public values are ever written (a pubkey, never a
  key).
- **`Load`**: the config file as a map (missing file = empty), read-only —
  the `/earn` wizard checks the hot key and the vault creator before it
  runs any step.

## How it is consumed

- `orbit init` calls `Init` with the direct devnet faucet (the indexer
  proxy rate-limits `requestAirdrop`); reads and writes still use the
  config RPC.
- `/earn` calls `Init` when the hot key is missing, then the vault steps
  with the operator key named at the call.
- `cmd/orbit` resolves the operator key from the same flag/env seam for
  vault and project commands.

## Gotchas

- The key file is the only copy (0600); the config file holds the key
  *path*, never the secret.
- The config is `KEY=VALUE` lines; env always overrides the file at load.
- The airdrop's lamport budget is bounded by `AirdropBudget` (default
  60s); `AirdropLamports` defaults to 1 SOL.
- `Init` writes `ORBIT_RPC` derived as `{indexer}/rpc` when unset, and a
  host-only value gets `/rpc` appended — the site base answers 301/405.
