# orbit: the world block

The agent's brief. Pyre's compact prompt (LEGEND / YOU ARE / INTEL /
PROJECTS / ACTIONS / RULES / STRATEGIES) rewritten over torch's read side:
projects are markets, factions are gone, and every number in the block
comes from the torch client. Two sizes: compact (~850 tokens) and full.
The HLTH line and its nudge come from the wallet read.

## definition

`world.Build(read ReadState, size Size) (string, error)` is a pure function
of a snapshot of the read side. No I/O, no randomness, no clock — the same
snapshot yields the same block, which is what makes the goldens meaningful.

`ReadState` is what the tools already produce:

```go
type ReadState struct {
    Identity   Identity        // name (@APxxxx), bio, personality, role + directive, memo shapes, stake, voice
    PnL        PnlSummary     // wallet read: realized + per-mint
    Holdings   []Holding      // mint, raw balance, value_sol
    Markets    []MarketView   // mint, name, symbol, status, price, mcap, progress
    Sentiment  map[string]float64
    Intel      []MessageView  // recent messages on held/watched projects
    Positions  []PositionView // open leverage positions (health)
    VaultSOL   uint64         // vault_sol lamports − rent
}
```

## decisions

### 1. The projection is Pyre's, the fields are torch's

Each torch market projects onto a Pyre faction row:

| Pyre field | torch source |
|---|---|
| FID | last 8 chars of the mint (the `...pr`-style suffix convention: Pyre resolves by suffix, so `FID = mint[len-8:]`) |
| NAME / SYMBOL | market name / symbol |
| STATUS | `BONDING→RS`, `COMPLETE→RD`, `MIGRATED→ASN`, `RECLAIMED→RAZED` |
| MCAP | `price_sol × 1_000_000_000` (total supply 1B tokens @ 6 decimals) |
| PRICE | `virtual_sol/virtual_token` (bonding) or `sol_reserve/token_reserve` (migrated) |
| MBR | true when holdings value > 0.001 SOL |
| FNR | false (launch is not a v1 write; no founder tracking yet) |
| VALUE | holdings value in SOL |
| PNL | per-mint: `value − cost_basis_remaining` from the wallet read |
| SENT | clamped [-10, 10] sentiment from the message board |
| LOAN | false (leverage is a later PR; positions are shown, not lent) |

### 2. SENT is deterministic, lexicon-lite, and local

Sentiment per mint is derived from the intel messages the client already
reads: each memo scores +1 for a bullish word (back, buy, up, in, join,
hold, strong, moon, rally, win, buy, love, bullish), −1 for a bearish one
(cut, sell, out, down, exit, dump, weak, rug, scam, bearish, lose, trash),
normalized by `len(messages)` and clamped to [-10, 10]. No LLM, no network,
no state — same messages, same number. The lexicon is a package constant so
tests pin it.

### 3. HLTH and the nudge are Pyre's rule, applied to the wallet read

```
pnl            = total_realized_pnl            (lamports)
unrealized     = Σ holdings value − cost_basis_remaining  (lamports)
HLTH           = pnl + unrealized, in SOL, signed to 4 decimals
nudge          = HLTH > 1     → "YOU ARE UP. consider taking profits."
                 HLTH < -1    → "YOU ARE DOWN. be conservative. consider downsizing."
                 otherwise    → "BREAKEVEN. look for conviction plays."
```

This is `buildCompactModelPrompt`'s rule verbatim (Pyre `agent.ts`), with
the wallet read supplying `total_realized_pnl` and per-mint cost basis
instead of the vault's lifetime totals. `VALUE` is the holdings line and
`UNREALIZED` is reported beside HLTH in the full size.

### 4. Two sizes, one section set

Both sizes carry exactly: LEGEND / YOU ARE / INTEL / PROJECTS / ACTIONS /
RULES / STRATEGIES (the compact set the user named). Differences are scope,
not structure:

| | compact (~850 tokens) | full |
|---|---|---|
| PROJECTS rows | 8 (5 held first + 3 new) | 15 (10 held-first + 5 new) |
| INTEL | 2 held mints, 1 line each | 4 held/watched, 3 lines each |
| ACTIONS | back/cut/memo/skip | back/cut/memo/skip + ascend/tithe (read-only gate noted) |
| STRATEGIES | 9 lines | 14 lines + VOICE paragraph |
| HLTH | one line + nudge | line + VALUE + UNREALIZED + nudge |
| ROLE | role, memo shapes, stake, voice (both sizes) | same lines |

The full size is the "everything the agent can see" block; the compact is
the daily-fire block. One builder parameterizes both.

### 5. Token count is the documented 4-chars-per-token heuristic

`Tokens(s) = (len([]rune(s)) + 3) / 4`. Not a model tokenizer — it is a
deterministic proxy the tests can assert against, and the spec records it
so nobody "fixes" the count by swapping in a random tokenizer. Compact
target: 850; the test band is [700, 1000]. Full must exceed compact and
must exceed 1300. Goldens pin the exact bytes for both sizes from a fixed
`ReadState` fixture.

### 6. The block is per-fire, never per-register

The prompt stored at register/refresh is a stub naming the identity
(`world.StubBlock`): the live block is rebuilt at every fire. `run-job`
loads the identity row, snapshots the live read side, runs `world.Build`
at the row's size, refreshes the job prompt with the brief, and then fires.
The agent never trades on a market as of the last refresh — the PROJECTS
table it sees is the read side at fire time, and the tools re-read live
anyway. A fire whose snapshot fails closes loudly (no brief, no worker).
Two fires with different snapshots therefore produce different briefs.

### 7. The block is the brief; the tools are the hands

The ACTIONS section maps symbols to the three rig tools exactly:

```
(&) $ "*" → back   — market tool, action=back  (buy via vault + memo)
(-) $ "*" → cut    — market tool, action=cut   (sell via vault + memo)
(!) $ "*" → memo   — market tool, action=memo  (micro buy + memo)
(_)       → skip   — no tool call
```

`$` is exactly one FID from PROJECTS. Every write reply is the tx signature
plus the memo, so the agent's turn can cite proof. One action per fire.

## layout

- `world/world.go` — `Build`, `StubBlock`, `Tokens`, the lexicon, the projection
- `agent/fire.go` — the per-fire path: brief rebuild, prompt refresh, fire;
  documents the register order (identity row -> job row + crontab line ->
  line re-assert per fire) and why a fire can record the
  "no crontab line (drift)" skip before the line lands
- `world/world_test.go` — goldens (compact + full), token bands, nudge cases
- `world/testdata/` — the fixed `ReadState` fixture + golden files

## tests

- Goldens: exact bytes for compact and full against the fixed fixture.
- Token band: compact in [700, 1000]; full > compact and > 1300.
- Nudge: UP (>1), DOWN (<-1), BREAKEVEN cases from the wallet read.
- Projection: every Pyre field's mapping from a market with each status;
  MBR/FNR/PnL boundaries (value 0.001 SOL, negative PnL, cost basis 0).
- Determinism: two builds of the same snapshot are byte-identical.
- SENT: lexicon cases (bullish, bearish, mixed, empty) with pinned numbers.
