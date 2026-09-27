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
-- SPEC_WORK: the ledger beside the log. project_states caches the torch
-- status the fold gates on, message_carriers the capital a memo's
-- transaction carried (a claim is capital or a position), project_positions
-- the long positions the indexer reports ended (the fold's release), and
-- task_backing the projection of what stands behind a claim or a verdict.
CREATE TABLE IF NOT EXISTS project_states (
  project TEXT NOT NULL,
  status TEXT NOT NULL,
  PRIMARY KEY (project)
);
CREATE TABLE IF NOT EXISTS message_carriers (
  project TEXT NOT NULL,
  signature TEXT NOT NULL,
  lamports INTEGER NOT NULL,
  opened TEXT NOT NULL,
  vault TEXT NOT NULL,
  position_index INTEGER NOT NULL,
  collateral INTEGER NOT NULL,
  PRIMARY KEY (project, signature)
);
CREATE TABLE IF NOT EXISTS project_positions (
  project TEXT NOT NULL,
  vault TEXT NOT NULL,
  position_index INTEGER NOT NULL,
  ended_at TEXT NOT NULL,
  PRIMARY KEY (project, vault, position_index)
);
CREATE TABLE IF NOT EXISTS task_backing (
  project TEXT NOT NULL,
  id TEXT NOT NULL,
  backing TEXT NOT NULL,
  stake INTEGER NOT NULL,
  vault TEXT NOT NULL,
  position_index INTEGER NOT NULL,
  reject_short INTEGER NOT NULL,
  PRIMARY KEY (project, id)
);
