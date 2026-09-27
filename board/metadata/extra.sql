-- What the DDL camera cannot emit (SPEC_BOARD, decisions): the write-side
-- idempotency door — a memo already cached under the same signature is
-- never applied twice. It is never a read path: the reads are the
-- primary-key accessors, no index no seek.
CREATE UNIQUE INDEX IF NOT EXISTS messages_signature_unique ON messages (mint, signature);
-- The walk-bound flag: a project whose log the 50-page walk did not reach
-- is marked here, and the board render says the cache is incomplete.
CREATE TABLE IF NOT EXISTS project_incomplete (
  project TEXT NOT NULL,
  PRIMARY KEY (project)
);
