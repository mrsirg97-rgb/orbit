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
