package board

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mrsirg97-rgb/rig/store"
	"github.com/mrsirg97-rgb/rig/store/sqlx"

	"github.com/mrsirg97-rgb/orbit/board/ddl"
	"github.com/mrsirg97-rgb/orbit/board/domain"
	"github.com/mrsirg97-rgb/orbit/board/metadata"
	"github.com/mrsirg97-rgb/orbit/client"
)

// SchemaVersion is the board cache's schema version.
const SchemaVersion = 1

// Project is one board's identity: the mint (the chain board's key) and
// the display label (the project name, or the FID when only the chain
// knows it).
type Project struct {
	Mint  string
	Label string
}

// Store is the board: the local SQLite cache (the chain's message log plus
// the fold projection) and the chain write path. The store file never is
// the record — the chain memo log is.
type Store struct {
	Client *client.TorchClient
	DB     store.DB
	Role   string
}

// Statements is the board schema: the generated DDL plus the signature
// idempotency index.
func Statements() []string {
	return append(ddl.Statements(), metadata.ExtraStatements()...)
}

// Open opens (or creates) the board cache.
func Open(path string) (store.DB, error) {
	db, quarantined, report, err := store.Open(path, Statements(), SchemaVersion)
	if err != nil {
		return store.DB{}, err
	}
	if quarantined != "" {
		return store.DB{}, fmt.Errorf("board: quarantined %s", quarantined)
	}
	if report != "" {
		return store.DB{}, fmt.Errorf("board: %s", report)
	}
	return db, nil
}

// StorePath is the board cache file: <home>/orbit/board.sqlite.
func StorePath(home string) string {
	return filepath.Join(home, "orbit", "board.sqlite")
}

// Sync folds the chain's message log into the cache. The indexer is the
// fast path; with ORBIT_INDEXER unset the RPC scan reads the chain
// directly. Rows insert idempotently by signature (the chain's slot and
// timestamp replace the local approximation), and the projection is
// rebuilt inside the same transaction, so the cache is always coherent.
// The cache is the board's window: each sync adds the newest messages and
// the fold covers what the cache holds.
func (s *Store) Sync(ctx context.Context, p Project, limit int) error {
	var rows []client.MessageRow
	if s.Client.Indexer != "" {
		msgs, err := s.Client.API.Messages(ctx, client.Q("mint", p.Mint, "limit", fmt.Sprint(limit)))
		if err != nil {
			return fmt.Errorf("board sync %s: indexer: %w", p.Mint, err)
		}
		rows = msgs
	} else {
		msgs, err := client.ScanMessages(ctx, s.Client.RPC, s.Client.ProgramID, p.Mint, limit)
		if err != nil {
			return fmt.Errorf("board sync %s: rpc scan: %w", p.Mint, err)
		}
		rows = msgs
	}
	bound, tx, err := s.DB.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	txr, err := sqlx.TxFrom(bound)
	if err != nil {
		return err
	}
	var maxSeq int64
	if err := txr.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM messages WHERE mint = ?`, p.Mint).Scan(&maxSeq); err != nil {
		return fmt.Errorf("board sync %s: %w", p.Mint, err)
	}
	next := maxSeq
	for _, r := range rows {
		seq := int64(r.MessageID)
		if s.Client.Indexer == "" {
			seq = next + 1
			next = seq
		}
		_, err := txr.ExecContext(ctx,
			`INSERT INTO messages (mint, seq, sender, memo_text, action_kind, slot, signature, inner_ix_idx, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(mint, signature) DO UPDATE SET slot = excluded.slot, created_at = excluded.created_at`,
			p.Mint, seq, r.Sender, r.MemoText, r.ActionKind, r.Slot, r.Signature,
			int64(r.InnerIxIdx), r.CreatedAt)
		if err != nil {
			return fmt.Errorf("board sync %s: cache: %w", p.Mint, err)
		}
	}
	if err := s.project(bound, p.Mint, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

// project rebuilds the fold projection (tasks + notes) from the cached
// message log inside the bound transaction.
func (s *Store) project(bound context.Context, mint string, now time.Time) error {
	rows, err := domain.NewMessageDomain().WindowMessageByMint(bound, mint, 0, math.MaxInt64, 1<<30).Rows()
	if err != nil {
		return fmt.Errorf("board fold %s: %w", mint, err)
	}
	memos := make([]Memo, 0, len(rows))
	for _, r := range rows {
		m, ok := ParseMemo(r.Memo)
		if !ok {
			continue
		}
		m.Sender = r.Sender
		m.At = r.CreatedAt
		memos = append(memos, m)
	}
	tasks := Fold(mint, memos, now)
	if err := rewrite(bound, mint, tasks); err != nil {
		return err
	}
	return nil
}

func rewrite(bound context.Context, mint string, tasks []Task) error {
	txr, err := sqlx.TxFrom(bound)
	if err != nil {
		return err
	}
	if _, err := txr.Exec("DELETE FROM notes WHERE project = ?", mint); err != nil {
		return fmt.Errorf("board fold %s: notes: %w", mint, err)
	}
	if _, err := txr.Exec("DELETE FROM tasks WHERE project = ?", mint); err != nil {
		return fmt.Errorf("board fold %s: tasks: %w", mint, err)
	}
	td := domain.NewTaskDomain()
	nd := domain.NewNoteDomain()
	for _, t := range tasks {
		if _, err := td.InsertTask(bound, domain.Task{
			Project: t.Project, Id: strconv.Itoa(t.ID), Title: t.Title, Brief: t.Brief,
			Status: t.Status, Owner: t.Owner, ClaimedAt: t.ClaimedAt,
			CompletedAt: t.CompletedAt, AcceptedBy: t.AcceptedBy,
			RejectedBy: t.RejectedBy, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		}); err != nil {
			return fmt.Errorf("board fold %s: task %d: %w", mint, t.ID, err)
		}
		for i, n := range t.Notes {
			if _, err := nd.InsertNote(bound, domain.Note{
				Project: mint, TaskId: strconv.Itoa(t.ID), Seq: int64(i + 1),
				Sender: n.Sender, Text: n.Text, CreatedAt: n.At,
			}); err != nil {
				return fmt.Errorf("board fold %s: note %d/%d: %w", mint, t.ID, i, err)
			}
		}
	}
	return nil
}

// Act writes one board verb: the memo + a vault-routed micro buy, the
// message cached (idempotent by signature), the projection rebuilt. The
// role comes from the shape — the tool passes the identity row's role,
// the swarm surface passes its own. The reply is the tx signature plus
// the memo, then the affected board.
func (s *Store) Act(ctx context.Context, p Project, shape Shape) (string, error) {
	memo, err := MemoFor(shape)
	if err != nil {
		return "", err
	}
	if err := s.Sync(ctx, p, 100); err != nil {
		return "", err
	}
	market, err := s.Market(ctx, p.Mint)
	if err != nil {
		return "", fmt.Errorf("board act %s: %w", shape.Verb, err)
	}
	res, err := s.Client.WriteAction(ctx, market, client.ActionMemo, memo, client.MemoBuyLamports)
	if err != nil {
		return "", fmt.Errorf("board act %s: %w", shape.Verb, err)
	}
	now := time.Now().UTC()
	bound, tx, err := s.DB.Tx(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	txr, err := sqlx.TxFrom(bound)
	if err != nil {
		return "", err
	}
	seq, err := nextSeq(bound, txr, p.Mint)
	if err != nil {
		return "", err
	}
	kind := "buy"
	created := now.Format(time.RFC3339)
	if _, err := domain.NewMessageDomain().InsertMessage(bound, domain.Message{
		Mint: p.Mint, Seq: seq, Sender: s.Client.AgentPublic(), Memo: memo,
		ActionKind: &kind, Signature: res.Signature, CreatedAt: created,
	}); err != nil {
		return "", fmt.Errorf("board act %s: cache: %w", shape.Verb, err)
	}
	if err := s.project(bound, p.Mint, now); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	board, err := s.BoardFromCache(ctx, p)
	if err != nil {
		return "", err
	}
	return res.Signature + " " + memo + "\n" + board, nil
}

func nextSeq(bound context.Context, tx *sql.Tx, mint string) (int64, error) {
	var maxSeq int64
	if err := tx.QueryRowContext(bound, `SELECT COALESCE(MAX(seq), 0) FROM messages WHERE mint = ?`, mint).Scan(&maxSeq); err != nil {
		return 0, err
	}
	return maxSeq + 1, nil
}

func (s *Store) Market(ctx context.Context, mint string) (client.MarketRow, error) {
	if s.Client.Indexer != "" {
		detail, err := s.Client.API.Market(ctx, mint)
		if err != nil {
			return client.MarketRow{}, err
		}
		return detail.Market, nil
	}
	return client.MarketFromRPC(ctx, s.Client.RPC, s.Client.ProgramID, mint)
}

// Board is the live read: sync the chain into the cache, fold, render.
func (s *Store) Board(ctx context.Context, p Project) (string, error) {
	if err := s.Sync(ctx, p, 100); err != nil {
		return "", err
	}
	return s.BoardFromCache(ctx, p)
}

// BoardFromCache renders the folded board without touching the chain (the
// act's reply path). The goal comes from the cached message log.
func (s *Store) BoardFromCache(ctx context.Context, p Project) (string, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	rows, err := domain.NewTaskDomain().WindowTaskByProject(bound, p.Mint, "", "\uffff", 1<<30).Rows()
	if err != nil {
		return "", err
	}
	goal, err := s.goalOf(bound, p.Mint)
	if err != nil {
		return "", err
	}
	return renderBoard(s.label(p), rows, goal, time.Now()), nil
}

func (s *Store) goalOf(bound context.Context, mint string) (string, error) {
	rows, err := domain.NewMessageDomain().WindowMessageByMint(bound, mint, 0, math.MaxInt64, 1<<30).Rows()
	if err != nil {
		return "", err
	}
	for _, r := range rows {
		if g, ok := GoalFrom(r.Memo); ok {
			return g, nil
		}
	}
	return "", nil
}

// Task returns one task's brief (title, brief, notes) — the swarm worker's
// prompt source, never a parsed render.
func (s *Store) Task(ctx context.Context, p Project, id string) (TaskInfo, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return TaskInfo{}, err
	}
	defer tx.Rollback()
	row, err := domain.NewTaskDomain().GetTask(bound, p.Mint, id).Row()
	if err != nil {
		return TaskInfo{}, err
	}
	if row == nil {
		return TaskInfo{}, fmt.Errorf("board: no task %s on %s", id, p.Mint)
	}
	notes, err := domain.NewNoteDomain().WindowNoteByProjectTaskId(bound, p.Mint, id, 0, math.MaxInt64, 1<<30).Rows()
	if err != nil {
		return TaskInfo{}, err
	}
	info := TaskInfo{ID: id, Title: row.Title, Brief: row.Brief, Status: row.Status, Owner: row.Owner}
	for _, n := range notes {
		info.Notes = append(info.Notes, Note{Sender: n.Sender, Text: n.Text, At: n.CreatedAt})
	}
	return info, nil
}

// TaskInfo is one task's brief.
type TaskInfo struct {
	ID     string
	Title  string
	Brief  string
	Status string
	Owner  string
	Notes  []Note
}

// nextPending returns the first pending task id on the live board. The
// claim's lease is the fold's expiry: an expired claim folds as pending,
// so a re-claim works with no separate door.
func (s *Store) nextPending(ctx context.Context, p Project) (int, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := domain.NewTaskDomain().WindowTaskByProject(bound, p.Mint, "", "\uffff", 1<<30).Rows()
	if err != nil {
		return 0, err
	}
	next := 0
	for _, r := range rows {
		if r.Status != StatusPending {
			continue
		}
		id, err := strconv.Atoi(r.Id)
		if err != nil {
			continue
		}
		if next == 0 || id < next {
			next = id
		}
	}
	return next, nil
}

// NextID is the fold's id mint for a project: the next task number.
func (s *Store) NextID(ctx context.Context, p Project) (int, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := domain.NewTaskDomain().WindowTaskByProject(bound, p.Mint, "", "\uffff", 1<<30).Rows()
	if err != nil {
		return 0, err
	}
	max := 0
	for _, r := range rows {
		id, err := strconv.Atoi(r.Id)
		if err != nil {
			continue
		}
		if id > max {
			max = id
		}
	}
	return max + 1, nil
}

func (s *Store) label(p Project) string {
	if p.Label != "" {
		return p.Label
	}
	return fid8(p.Mint)
}

func fid8(mint string) string {
	if len(mint) <= 8 {
		return mint
	}
	return mint[len(mint)-8:]
}

func shortAddr(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:4] + "…" + s[len(s)-4:]
}
