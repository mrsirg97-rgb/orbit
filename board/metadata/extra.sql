-- What the DDL camera cannot emit (SPEC_BOARD, decisions): the write-side
-- idempotency door — a memo already cached under the same signature is
-- never applied twice. It is never a read path: the reads are the
-- primary-key accessors, no index no seek.
CREATE UNIQUE INDEX IF NOT EXISTS messages_signature_unique ON messages (mint, signature);
