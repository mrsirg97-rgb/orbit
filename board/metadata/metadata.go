// Hand-written metadata for the board store: the containers SPEC_BOARD
// fixes — meta (the version door), messages (the chain log cache, the
// indexer's message schema verbatim), tasks (the fold projection, keyed by
// project and task), notes (a task's note/reason rows). Source of truth;
// domain and ddl are generated from it, never typed by hand. Nullable
// columns are pointers.
package metadata

// table:"meta"
type Meta struct {
	Key   string `primary:"true" alias:"name=key,nullable=false"`
	Value string `alias:"name=value,nullable=false"`
}

// table:"messages"
//
// The chain's message log cache, keyed by (mint, seq): seq is the indexer's
// message_id (the indexer path) or the local rowid (the RPC scan path). The
// log is the spine; tasks and notes are a disposable projection rebuilt
// from it inside every transaction. The signature unique index (extra.sql)
// is the write-side idempotency door — a memo already cached is never
// applied twice; it is never a read path.
type Message struct {
	Mint       string  `primary:"true" alias:"name=mint,nullable=false"`
	Seq        int64   `primary:"true" alias:"name=seq,nullable=false"`
	Sender     string  `alias:"name=sender,nullable=false"`
	Memo       string  `alias:"name=memo_text,nullable=false"`
	ActionKind *string `alias:"name=action_kind,nullable=true"`
	Slot       int64   `alias:"name=slot,nullable=false"`
	Signature  string  `alias:"name=signature,nullable=false"`
	InnerIxIdx int64   `alias:"name=inner_ix_idx,nullable=false"`
	CreatedAt  string  `alias:"name=created_at,nullable=false"`
}

// table:"tasks"
//
// The fold projection, keyed by (project, id): id is the task number the
// memo names. Status is pending | active | review | done; owner is the
// claim's sender; the timestamps are the memo times. The reads are
// primary-key-seek only — one task is GetTask(project, id), a project's
// board is WindowTaskByProject — no index, no seek beyond the key.
type Task struct {
	Project     string `primary:"true" alias:"name=project,nullable=false"`
	ID          string `primary:"true" alias:"name=id,nullable=false"`
	Title       string `alias:"name=title,nullable=false"`
	Brief       string `alias:"name=brief,nullable=false"`
	Status      string `alias:"name=status,nullable=false"`
	Owner       string `alias:"name=owner,nullable=false"`
	ClaimedAt   string `alias:"name=claimed_at,nullable=false"`
	CompletedAt string `alias:"name=completed_at,nullable=false"`
	AcceptedBy  string `alias:"name=accepted_by,nullable=false"`
	RejectedBy  string `alias:"name=rejected_by,nullable=false"`
	CreatedAt   string `alias:"name=created_at,nullable=false"`
	UpdatedAt   string `alias:"name=updated_at,nullable=false"`
}

// table:"notes"
//
// A task's notes and verdict reasons, keyed by (project, task, seq) in
// append order.
type Note struct {
	Project   string `primary:"true" alias:"name=project,nullable=false"`
	TaskID    string `primary:"true" alias:"name=task_id,nullable=false"`
	Seq       int64  `primary:"true" alias:"name=seq,nullable=false"`
	Sender    string `alias:"name=sender,nullable=false"`
	Text      string `alias:"name=text,nullable=false"`
	CreatedAt string `alias:"name=created_at,nullable=false"`
}
