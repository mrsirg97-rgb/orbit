# board/metadata

## What it is

Hand-written metadata for the board store: the containers SPEC_BOARD
fixes — meta (the version door), messages (the chain log cache, the
indexer's message schema verbatim), tasks (the fold projection, keyed by
project and task), notes (a task's note/reason rows). Source of truth;
domain and ddl are generated from it, never typed by hand. Nullable
columns are pointers.

## How it is consumed

- Consumed by the gen tooling to project the generated `ddl`/`domain`
  accessors; not part of the runtime path.

## Gotchas

- Source for generated code: edit and regenerate; the generated
  projections are derived, not hand-edited.
- The signature unique index lives in `extra.sql` — what the DDL camera
  cannot emit: the write-side idempotency door, a memo already cached
  under the same signature is never applied twice. It is never a read
  path: the reads are the primary-key accessors, no index, no seek.
- `extra.sql` also carries the ledger beside the log (SPEC_WORK):
  `project_states` (the torch status the fold gates on),
  `message_carriers` (the capital a memo's transaction carried),
  `project_positions` (the long positions the indexer reports ended), and
  `task_backing` (the projection of what stands behind a claim or a
  verdict). Hand tables, keyed by project; the store reads them by key.
