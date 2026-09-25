# project

## What it is

The operator's project surface: create a market with a purpose
(`create_token` plus a first vault buy whose memo carries the goal) and
list the projects the indexer knows, goal and treasury included.
SPEC_PROJECT governs.

## What it includes

- **`GoalMemo` / `GoalFrom`**: the goal memo shape — `goal: <goal>`, one
  paragraph, capped at the curve memo bound (500 chars); `GoalFrom`
  parses it back out of a board memo.
- **`SymbolFor`**: the ticker derived from the name — uppercase
  alphanumerics, the first 6 runes, `PROJ` when the name has no letters.
- **`List`**: the indexer read — markets, the first `goal:` memo per
  market, and the treasury float from the RPC seam (the
  `treasury_sol_vault` lamports minus the rent floor). A market without a
  goal memo keeps `Goal` empty; a market without a treasury account shows
  0.
- **`Create`**: the two-transaction create — `create_token` (operator +
  fresh mint keypair, both sign) confirmed before the buy is sent; then
  the first `buy_via_vault` from the operator's own vault with the goal
  memo. The operator's key must be the config's vault creator; nothing is
  retried blindly and every failure names the step.

## How it is consumed

- `cmd/orbit`'s `project create` / `project list` drive it; the operator
  key comes from the same flag/env seam as `vault` and is never stored.
- The create is one atomic unit of two transactions, but the chain has no
  transactional join between them — the error names the step and the
  signature, and never retries.

## Gotchas

- The goal lives on the buy memo, not in metadata: the IDL's `uri` stays
  empty, and the brief's INTEL section surfaces the goal as soon as the
  buy lands.
- The treasury is the first buy, not a token allocation: `--treasury` is
  the SOL amount of the first buy, and the float shown by `list` is read
  from the chain.
- The mint keypair is generated in-process and never stored; the create
  tx is signed by the operator (payer) and the mint keypair.
- A pre-flight vault balance check refuses a buy that would bounce; the
  confirmation poll is bounded and refuses to proceed on an unconfirmed
  create.
