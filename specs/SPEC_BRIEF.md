# orbit: the brief

The agent's brief. Pyre's compact prompt (LEGEND / YOU ARE / INTEL /
PROJECTS / ACTIONS / RULES / STRATEGIES) rewritten over torch's read side:
projects are markets, factions are gone, and every number in the brief
comes from the torch client. Two sizes: compact and full. The vocabulary
is torch's — PNL, back/exit/post/pass, bonding/ready/migrated/reclaimed,
HELD/FOUNDED/SENTIMENT, "other contributors" — never Pyre's abbreviations.

## definition

`brief.Build(read ReadState, size Size) (string, error)` is a pure function
of a snapshot of the read side. No I/O, no randomness, no clock — the same
snapshot yields the same brief, which is what makes the goldens meaningful.

`ReadState` is what the tools already produce:

```go
type ReadState struct {
    Identity   Identity        // name (@APxxxx), bio, and the architect's goal
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

### 1. The projection is Pyre's shape, torch's words

Each torch market projects onto a row:

| column | torch source |
|---|---|
| FID | last 8 chars of the mint (Pyre resolves by suffix, so `FID = mint[len-8:]`) |
| NAME / SYMBOL | market name / symbol |
| STATUS | `bonding` (BONDING), `ready` (COMPLETE), `migrated` (MIGRATED), `reclaimed` (RECLAIMED) |
| MCAP | `price_sol × 1_000_000_000` (total supply 1B tokens @ 6 decimals) |
| PRICE | `virtual_sol/virtual_token` (bonding) or `sol_reserve/token_reserve` (migrated) |
| HELD | true when holdings value > 0.001 SOL |
| FOUNDED | false (launch is not a v1 write; no founder tracking yet) |
| VALUE | holdings value in SOL |
| PNL | per-mint: `value − cost_basis_remaining` from the wallet read |
| SENTIMENT | clamped [-10, 10] sentiment from the message board |
| LOAN | false (leverage is a later PR; positions are shown, not lent) |

### 2. SENTIMENT is deterministic, lexicon-lite, and local

Sentiment per mint is derived from the intel messages the client already
reads: each memo scores +1 for a bullish word (back, buy, up, in, join,
hold, strong, moon, rally, win, love, bullish), −1 for a bearish one
(cut, sell, out, down, exit, dump, weak, rug, scam, bearish, lose, trash),
normalized by `len(messages)` and clamped to [-10, 10]. No LLM, no network,
no state — same messages, same number. The lexicon is a package constant so
tests pin it.

### 3. PNL and the nudge are Pyre's rule, applied to the wallet read

```
pnl            = total_realized_pnl            (lamports)
unrealized     = Σ holdings value − cost_basis_remaining  (lamports)
PNL            = pnl + unrealized, in SOL, signed to 4 decimals
nudge          = PNL > 1      → "YOU ARE UP. consider taking profits."
                 PNL < -1     → "YOU ARE DOWN. be conservative. consider downsizing."
                 otherwise    → "BREAKEVEN. look for conviction plays."
```

This is `buildCompactModelPrompt`'s rule verbatim (Pyre `agent.ts`), with
the wallet read supplying `total_realized_pnl` and per-mint cost basis
instead of the vault's lifetime totals. `REALIZED` is reported beside PNL
in the full size.

### 4. YOU ARE is the wallet, not a job title

The YOU ARE section names the wallet and describes its position and
history — never a role, never an archetype:

```
NAME: @AP2B3A
BIO: A torch market agent...
PNL: +0.1609 SOL.
BREAKEVEN. look for conviction plays.
VAULT: 0.0100 SOL.
POSITIONS: rNjTjmVx long at_risk 0.0050 SOL.
```

PNL is the wallet's lifetime position (realized + unrealized); POSITIONS
are its open leverage positions; VAULT is its current position; HOLDINGS
(full size) are what it holds. The full size adds the architect's GOAL
line and the REALIZED line. The memo shapes, per-action stake, and voice
lines are gone — the tools' memos carry no role tag and the wallet's stake
is the proof.

### 5. Two sizes, one section set

Both sizes carry exactly: LEGEND / YOU ARE / INTEL / PROJECTS / ACTIONS /
RULES / STRATEGIES (the compact set the user named). Differences are scope,
not structure:

| | compact | full |
|---|---|---|
| PROJECTS rows | 8 (5 held first + 3 new) | 15 (10 held-first + 5 new) |
| INTEL | 2 held mints, 1 line each | 4 held/watched, 3 lines each |
| ACTIONS | back/exit/post/pass | back/exit/post/pass + ascend/tithe (read-only gate noted) |
| STRATEGIES | 13 lines | 25 lines |
| PNL | one line + nudge | line + REALIZED + nudge |
| YOU ARE | name, bio, PNL, nudge, vault, positions | same + GOAL + HOLDINGS |

The full size is the "everything the agent can see" brief; the compact is
the daily-fire brief, and it shrank with the vocabulary pass (no glyphs, no
realized line, no voice lines). One builder parameterizes both.

### 6. Token count is the documented 4-chars-per-token heuristic

`Tokens(s) = (len([]rune(s)) + 3) / 4`. Not a model tokenizer — it is a
deterministic proxy the tests can assert against, and the spec records it
so nobody "fixes" the count by swapping in a random tokenizer. Compact
target: ~875; the test band is [650, 900]. Full must exceed compact and
must exceed 1300. Goldens pin the exact bytes for both sizes from a fixed
`ReadState` fixture.

### 7. The brief is per-fire, never per-register

The prompt stored at register/refresh is a stub naming the identity
(`brief.StubBrief`): the live brief is rebuilt at every fire. `run-job`
loads the identity row, snapshots the live read side, runs `brief.Build`
at the row's size, refreshes the job prompt with the brief, and then fires.
The agent never trades on a market as of the last refresh — the PROJECTS
table it sees is the read side at fire time, and the tools re-read live
anyway. A fire whose snapshot fails closes loudly (no brief, no worker).
Two fires with different snapshots therefore produce different briefs.

### 8. The brief is the brief; the tools are the hands

The ACTIONS section maps words to the three rig tools exactly — no glyphs:

```
back $ "*"  — market tool, action=back  (buy via vault + memo)
exit $ "*"  — market tool, action=exit  (sell via vault + memo)
post $ "*"  — market tool, action=post  (micro buy + memo)
pass        — no tool call
```

`$` is exactly one FID from PROJECTS. Every write reply is the tx signature
plus the memo, so the agent's turn can cite proof. One action per fire.

## layout

- `brief/brief.go` — `Build`, `StubBrief`, `Tokens`, the lexicon, the projection
- `agent/fire.go` — the per-fire path: brief rebuild, prompt refresh, fire;
  documents the register order (identity row -> job row + crontab line ->
  line re-assert per fire) and why a fire can record the
  "no crontab line (drift)" skip before the line lands
- `brief/brief_test.go` — goldens (compact + full), token bands, nudge cases
- `brief/testdata/` — the fixed `ReadState` fixture + golden files

## tests

- Goldens: exact bytes for compact and full against the fixed fixture.
- Token band: compact in [650, 900]; full > compact and > 1300.
- Nudge: UP (>1), DOWN (<-1), BREAKEVEN cases from the wallet read.
- Projection: every field's mapping from a market with each status;
  HELD/FOUNDED/PnL boundaries (value 0.001 SOL, negative PnL, cost basis 0).
- Determinism: two builds of the same snapshot are byte-identical.
- SENTIMENT: lexicon cases (bullish, bearish, mixed, empty) with pinned numbers.
- Vocabulary: no HLTH, RS/ASN/RAZED, MBR/FNR/SENT, glyphs, rival/ally, or
  "world block" anywhere in either golden.
