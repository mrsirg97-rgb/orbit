# identity

## What it is

The agent's local identity row: name, wallet, role, bio, goal, cadence,
model, and the job's stall/budget/timeout. One row per (wallet, role) —
the role suffix is the agent id, so one wallet can host an architect, a
worker, and a reviewer side by side. Roles are local: they own the
defaults (cadence, brief size, budget, stall, timeout) and never ride a
memo — the board's fold trusts the sender, not a label. No archetypes, no
voice, no stake scale.

## What it includes

- **`Role`** and the role table: architect / worker / reviewer, each with
  its register defaults. `ParseRole` accepts exactly one of the three and
  refuses naming the three; `DefaultsFor` returns the row.
- **`Row`** and `NewRow`: one identity row per (wallet, role). The id is
  `@AP<wallet-suffix>-<role>`; the defaults come from the role unless the
  overrides beat them; `Model` is always explicit.
- **`BaseStakeLamports`**: the one per-action stake (0.01 SOL) for market
  writes — one stake for every role, the memo buy is the proof, never a
  scale.
- **The store**: schema v5 (`identity.sqlite`), `Upsert` (idempotent,
  wallet-tag collision refused), `GetByID`, `List`, and the migration path
  from v2 (block size), v3 (the archetype era: role/voice/stake scale),
  v4 (the one-row-per-wallet era, roles dropped), v5 (role + goal back,
  per (wallet, role)).

## How it is consumed

- `agent.Register` turns one row into one scheduled job
  (`orbit-agent-<id>`); the earn wizard registers the roles it names; the
  fire path resolves the row by id.
- The role owns cadence, brief size, budget, stall, and timeout defaults —
  the job carries them; the register flags override.
- `Upsert` recomputes the id from the wallet and the row's role — never a
  caller-supplied label — so a wallet cannot register the same role twice
  under different ids.

## Gotchas

- The schema is v5; the migrations run in order and the v4 era is a real
  historical schema (roles were dropped, then role + goal came back with
  the per-(wallet, role) ids).
- `Overrides` use zero as "unset": a register flag of 0 (budget, stall,
  timeout) keeps the role default — an explicit 0 is not expressible.
- The goal is the architect's one-paragraph goal, carried into the brief's
  YOU ARE section; it is an architect-only column.
