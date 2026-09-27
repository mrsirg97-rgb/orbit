# brief

## What it is

The agent's brief: the compact prompt shape over torch's read side, in
the gig economy's vocabulary (SPEC_WORK decisions 9 and 12). `Build` is a
pure function of a `ReadState` snapshot — same snapshot, same bytes, which
is what makes the goldens meaningful. The acts are invest/contract/work/
release/short/post/pass, the states are private/funded/public/closed
(torch's bonding/ready/migrated/reclaimed named once, in the LEGEND), and
the columns are STATE/SIZE/RATE/HELD/FOUNDED/VALUE/EARN/GOSSIP/COMMIT/
BACKING/LENDS — no glyphs, no foreign abbreviations, no torch trade words.
SPEC_BRIEF governs.

## What it includes

- `Build` — renders the brief: LEGEND / YOU ARE / GOSSIP / PROJECTS /
  ACTIONS / RULES / STRATEGIES, both sizes. The full size is "everything
  the agent can see"; the compact is the daily-fire brief. The LEGEND is
  the one place both vocabularies meet; the compact ACTIONS block points
  back at it instead of repeating the acts.
- `StateWord` — torch status to orbit's state word. `MarketView.Lending`
  and `TreasurySOL` feed the BACKING and LENDS columns; a public project
  with LENDS F is "public, not lending yet" (contract, not work), said in
  the PROJECTS legend line.
- `StubBrief` — the prompt stored at register/refresh: the identity
  header and the static rules, with a line telling the fire to rebuild
  the live brief. The real brief is built per fire by run-job.
- `HealthLine` — the PNL rule: total realized + unrealized, then the
  nudge (UP > 1, DOWN < -1, BREAKEVEN otherwise). The line is the
  wallet's overall profit and loss, not a job title.
- `SentimentFrom` — the deterministic sentiment score: +1 per bullish
  word, -1 per bearish word, normalized by message count, clamped to
  [-10, 10]. Same messages, same number — no LLM, no state. The lexicon
  is a package constant so tests pin it.
- `Tokens` — the documented 4-chars-per-token heuristic.
- The projection: `ReadState` (what the tools produce) -> `MarketView`
  rows; FID is the last 8 chars of the mint, the numbers are the wallet
  read's (PnL, holdings value, cost basis), and the vocabulary is
  torch's.

## How it is consumed

- `tool.Snapshot` assembles the `ReadState` from the live read side;
  `agent.Fire` passes the rebuilt brief through the fire path; `orbit
  snapshot` prints it; the earn footer's rows come from the same read.
- The brief is per-fire, never per-register: the prompt stored at
  register is only a stub naming the identity until the first fire
  replaces it.

## Gotchas

- The 4-chars-per-token count is a deterministic proxy, not a model
  tokenizer — the tests assert against it, and it must never be swapped
  for a random tokenizer.
- Same snapshot, same bytes: the goldens pin both sizes to the byte from
  the fixed fixture; a new number is a column in the projection and a
  line in the golden.
- PROJECTS names are truncated to 12 chars (rune-safe — a name is not
  split mid-rune); the FID is the last 8 chars of the mint.
- The brief never names a role, an archetype, or a stake scale: YOU ARE
  is the wallet, and the memos the tools write carry no role tag.
- Risk is said in the same plain register as the rest: work is borrowed
  budget with a clock on it, and getting washed out costs the stake. The
  immersion never hides the ledger.
