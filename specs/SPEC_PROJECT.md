# orbit: projects

A project is a torch market with a purpose. The operator creates one with
`orbit project create`: a new token (create_token), then a first buy from
the operator's own vault that funds the treasury. The project's goal is the
memo on that buy, `goal: ...`, so the board's fold
(intel, the world block's INTEL section) shows what the project is for.
`orbit project list` reads the indexer: markets, the goal memo per market,
and the treasury float. Devnet only, like every write.

## definition

- **Create**: `project create --name <n> --goal "<one paragraph>" --treasury <sol>`.
  The operator key comes from the same flag/env seam as `vault`
  (`--operator-key`, `--operator-key-path`, `ORBIT_OPERATOR_KEY_PATH`) and is
  never stored. The command runs two transactions:
  1. `create_token` — the operator is the creator, a fresh mint keypair signs
     (generated in-process, never stored), the IDL `CreateTokenArgs` are
     `name`, a derived `symbol`, empty `uri`, `sol_target = 200 SOL`, and
     `community_token = false`.
  2. `buy_via_vault` — the operator is the buyer (the creator's wallet link is
     initialized at `create_vault`), the vault pays, `sol_amount = --treasury`.
     The vault ATA is created first (idempotent), the curve quote sets
     `min_tokens_out` at the default 100 bps slippage, and the SPL memo is
     `goal: <goal>`.
  The create is confirmed (bounded `getSignatureStatus` poll) before the buy
  is sent; the market row comes from the indexer (bounded poll) so the quote
  uses the real curve state. A missing vault SOL balance, a missing market
  after the poll, or a missing global config fails loudly — nothing is
  retried blindly. Output: the mint, the symbol, the treasury amount, and
  both signatures.

- **List**: `project list` reads `/api/markets` (limit 50) and, per market,
  `/api/messages?mint=...` (limit 50) to find the first `goal:`
  memo. The treasury float is the `treasury_sol_vault` PDA balance minus the
  rent floor, read through the RPC seam. Rows: FID, name, symbol, status,
  treasury SOL, goal; a market without a goal memo shows `-`.

- **Symbol**: derived deterministically from the name — uppercase
  alphanumerics, first 6 runes, `PROJ` when the name has no letters.

- **Memo shape**: `goal: <goal>` — the verb says what happened; no role tag.
  The goal must be one paragraph and fit the curve memo cap (500 chars);
  the name is capped at 32 chars. The goal memo is the first message on a
  fresh market — the first buy is the market's first torch tx with a memo.

## decisions

### 1. The operator is the creator and the buyer

`create_token`'s creator is the operator's pubkey; `create_vault` already
initializes the creator's own `vault_wallet` link, so the operator can spend
from their own vault without a separate link step. The vault receives the
tokens (buy_via_vault writes to the vault's ATA), the treasury receives SOL,
and the buyer's `user_position` records the first buy. The agent hot wallet
is not involved in project create: the operator acts directly, the same key
never enters the process after the call.

### 2. create_token needs two signers

The mint is a fresh keypair. The create tx is signed by the operator (payer)
and the mint keypair; the buy tx by the operator alone. `sol.SignVersionedTx`
gains a multi-signer form that signs the v0 message once per required signer,
in the message's signed-key order, so the two-key create compiles and signs
without a hand-rolled path.

### 3. The goal lives on the buy memo, not in metadata

The IDL's `uri` stays empty; the goal is a memo because memos ride torch txs
and the indexer persists them as board messages. The world block's INTEL
section already surfaces message text, so no world change is needed — the
fold shows the goal as soon as the buy lands. `project list` reconstructs the
goal from the same indexer log, so the operator view and the agent view
agree.

### 4. The treasury is the first buy, read from the chain

`--treasury` is the SOL amount of the first buy, not a token allocation. The
treasury float shown by `list` is the `treasury_sol_vault` PDA lamports minus
the rent floor — the same number the world block reads for a held market.

### 5. Fail closed at each step

The create is one atomic unit of two transactions, but the chain has no
transactional join between them: if the buy fails after a confirmed create,
the project exists without a goal. The error names the step and the
signature, and never retries. A pre-flight vault balance check refuses a buy
that would bounce; the confirmation poll is bounded and refuses to proceed on
an unconfirmed create.

## layout

- `project/project.go` — `Row`, `List`, `Create`, `GoalMemo`, `GoalFrom`,
  `SymbolFor`
- `client/ix.go` — `BuildCreateToken` (discriminator, account order, borsh args)
- `client/pda.go` — `TreasuryLockPDA`
- `idl/idl.go` — borsh `string` encoding (length-prefixed UTF-8)
- `sol/sol.go` — `SignVersionedTxMulti`
- `cmd/orbit/project.go` — the `project` subcommand
- `scripts/seed-devnet.sh` — three example research projects with real goals
- `testdata/projects.json` — the list fixture (markets + goal memos)

## tests

- Instruction build against the IDL: `create_token` discriminator + borsh
  args golden hex, exact account order (16 accounts) and signer/writable
  flags, PDA derivations (bonding curve, treasury, treasury_lock, the three
  ATAs).
- List against a fixture: markets + goal memos served as the indexer routes,
  a canned treasury RPC — rows carry the goal text and the treasury float; a
  market without a goal memo shows `-`.
- Goal memo: `goal: <goal>` tag and parse round-trip; symbol
  derivation; the multi-signer tx verifies both signatures.
