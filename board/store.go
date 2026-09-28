package board

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mrsirg97-rgb/rig/v2/store"
	"github.com/mrsirg97-rgb/rig/v2/store/sqlx"

	"github.com/mrsirg97-rgb/orbit/board/ddl"
	"github.com/mrsirg97-rgb/orbit/board/domain"
	"github.com/mrsirg97-rgb/orbit/board/metadata"
	"github.com/mrsirg97-rgb/orbit/client"
)

const SchemaVersion = 5

const WalkBound = 50

type Project struct {
	Mint  string
	Label string
}

type Store struct {
	Client     func() (*client.TorchClient, error)
	DB         store.DB
	WaitBudget time.Duration
	OnAct      func(context.Context, Shape)
	mu         sync.Mutex
}

func (s *Store) client() (*client.TorchClient, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("board: no orbit config (run /earn)")
	}
	return s.Client()
}

func Statements() []string {
	return append(ddl.Statements(), metadata.ExtraStatements()...)
}

func migration(tx *sql.Tx, from, to int) (string, error) {
	if from < 2 {
		if _, err := tx.Exec(`ALTER TABLE tasks ADD COLUMN funder TEXT NOT NULL DEFAULT ''`); err != nil {
			return "", err
		}
	}
	return "", nil
}

func Open(path string) (store.DB, error) {
	db, quarantined, report, err := store.Open(path, Statements(), SchemaVersion, migration)
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

func StorePath(home string) string {
	return filepath.Join(home, "board.sqlite")
}

var ErrIndexerUnreachable = errors.New("indexer unreachable: board may be stale")

func (s *Store) Sync(ctx context.Context, p Project, limit int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tc, err := s.client()
	if err != nil {
		return err
	}
	source, recorded, err := s.sourceOf(ctx, p.Mint, tc)
	if err != nil {
		return err
	}
	if source == client.SourceIndexer && tc.Indexer == "" {
		res, err := s.walk(ctx, p, tc, client.SourceScan, limit, true)
		if err != nil {
			return fmt.Errorf("board sync %s: rpc scan: %w", p.Mint, err)
		}
		up, err := s.ledgerUpdate(ctx, p, tc, res.rows)
		if err != nil {
			return fmt.Errorf("board sync %s: ledger: %w", p.Mint, err)
		}
		return s.cacheRows(ctx, p, res.rows, client.SourceScan, true, res.bound, up)
	}
	res, err := s.walk(ctx, p, tc, source, limit, false)
	if err != nil {
		if source == client.SourceIndexer && client.IndexerUnreachable(err) {
			if recorded {
				return fmt.Errorf("board sync %s: %w", p.Mint, ErrIndexerUnreachable)
			}
			fmt.Printf("board sync %s: source switched from indexer to RPC scan (indexer unreachable)\n", p.Mint)
			res, err = s.walk(ctx, p, tc, client.SourceScan, limit, false)
			if err != nil {
				return fmt.Errorf("board sync %s: rpc scan: %w", p.Mint, err)
			}
			incomplete := res.bound
			if !res.genesis && !res.bound {
				prev, err := s.projectIncomplete(ctx, p.Mint)
				if err != nil {
					return err
				}
				incomplete = prev
			}
			up, err := s.ledgerUpdate(ctx, p, tc, res.rows)
			if err != nil {
				return fmt.Errorf("board sync %s: ledger: %w", p.Mint, err)
			}
			return s.cacheRows(ctx, p, res.rows, client.SourceScan, false, incomplete, up)
		}
		if source == client.SourceIndexer {
			return fmt.Errorf("board sync %s: indexer: %w", p.Mint, err)
		}
		return fmt.Errorf("board sync %s: rpc scan: %w", p.Mint, err)
	}
	incomplete := res.bound
	if !res.genesis && !res.bound {
		prev, err := s.projectIncomplete(ctx, p.Mint)
		if err != nil {
			return err
		}
		incomplete = prev
	}
	up, err := s.ledgerUpdate(ctx, p, tc, res.rows)
	if err != nil {
		return fmt.Errorf("board sync %s: ledger: %w", p.Mint, err)
	}
	return s.cacheRows(ctx, p, res.rows, source, false, incomplete, up)
}

type walkResult struct {
	rows    []client.MessageRow
	bound   bool
	genesis bool
}

func (s *Store) walk(ctx context.Context, p Project, tc *client.TorchClient, source client.Source, limit int, fresh bool) (walkResult, error) {
	if limit <= 0 {
		limit = 100
	}
	var pages [][]client.MessageRow
	cursor := ""
	for page := 0; page < WalkBound; page++ {
		pg, err := tc.MessagesPage(ctx, p.Mint, limit, source, cursor)
		if err != nil {
			return walkResult{}, err
		}
		if len(pg.Signatures) == 0 {
			return walkResult{rows: s.flatten(pages), genesis: true}, nil
		}
		pages = append(pages, pg.Rows)
		if len(pg.Signatures) < limit {
			return walkResult{rows: s.flatten(pages), genesis: true}, nil
		}
		if !fresh {
			cached, err := s.anyCached(ctx, p.Mint, pg.Signatures)
			if err != nil {
				return walkResult{}, err
			}
			if cached {
				return walkResult{rows: s.flatten(pages)}, nil
			}
		}
		cursor, err = s.nextCursor(source, pg)
		if err != nil {
			return walkResult{}, err
		}
		if cursor == "" {
			return walkResult{rows: s.flatten(pages), genesis: true}, nil
		}
	}
	return walkResult{rows: s.flatten(pages), bound: true}, nil
}

func (s *Store) flatten(pages [][]client.MessageRow) []client.MessageRow {
	total := 0
	for _, p := range pages {
		total += len(p)
	}
	out := make([]client.MessageRow, 0, total)
	for i := len(pages) - 1; i >= 0; i-- {
		out = append(out, pages[i]...)
	}
	return out
}

func (s *Store) nextCursor(source client.Source, pg client.MessagePage) (string, error) {
	switch source {
	case client.SourceIndexer:
		oldest := pg.Rows[len(pg.Rows)-1].CreatedAt
		t, err := time.Parse(time.RFC3339, oldest)
		if err != nil {
			return "", fmt.Errorf("board sync: created_at %q: %w", oldest, err)
		}
		return t.Add(time.Second).UTC().Format(time.RFC3339), nil
	case client.SourceScan:
		return pg.OldestSignature, nil
	default:
		return "", fmt.Errorf("board sync: unknown source %q", source)
	}
}

func (s *Store) anyCached(ctx context.Context, mint string, signatures []string) (bool, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	args := make([]any, 0, len(signatures)+1)
	args = append(args, mint)
	placeholders := make([]string, 0, len(signatures))
	for _, sig := range signatures {
		placeholders = append(placeholders, "?")
		args = append(args, sig)
	}
	q := `SELECT 1 FROM messages WHERE mint = ? AND signature IN (` + strings.Join(placeholders, ", ") + `) LIMIT 1`
	var one int
	err = tx.QueryRowContext(bound, q, args...).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) sourceOf(ctx context.Context, mint string, tc *client.TorchClient) (client.Source, bool, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	var recorded string
	err = tx.QueryRowContext(bound, `SELECT source FROM project_sources WHERE project = ?`, mint).Scan(&recorded)
	switch {
	case err == sql.ErrNoRows:
		if tc.Indexer == "" {
			return client.SourceScan, false, nil
		}
		return client.SourceIndexer, false, nil
	case err != nil:
		return "", false, err
	case recorded == string(client.SourceIndexer):
		return client.SourceIndexer, true, nil
	case recorded == string(client.SourceScan):
		return client.SourceScan, true, nil
	default:
		return "", false, fmt.Errorf("board sync %s: unknown message source %q", mint, recorded)
	}
}

func (s *Store) projectIncomplete(ctx context.Context, mint string) (bool, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	return incompleteIn(bound, mint)
}

func incompleteIn(bound context.Context, mint string) (bool, error) {
	tx, err := sqlx.TxFrom(bound)
	if err != nil {
		return false, err
	}
	var one int
	err = tx.QueryRowContext(bound, `SELECT 1 FROM project_incomplete WHERE project = ?`, mint).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) cacheRows(ctx context.Context, p Project, rows []client.MessageRow, source client.Source, wipe, incomplete bool, up ledgerUpdate) error {
	bound, tx, err := s.DB.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	txr, err := sqlx.TxFrom(bound)
	if err != nil {
		return err
	}
	if wipe {
		if _, err := txr.ExecContext(ctx, `DELETE FROM messages WHERE mint = ?`, p.Mint); err != nil {
			return fmt.Errorf("board sync %s: wipe: %w", p.Mint, err)
		}
		if _, err := txr.ExecContext(ctx, `DELETE FROM notes WHERE project = ?`, p.Mint); err != nil {
			return fmt.Errorf("board sync %s: wipe: %w", p.Mint, err)
		}
		if _, err := txr.ExecContext(ctx, `DELETE FROM tasks WHERE project = ?`, p.Mint); err != nil {
			return fmt.Errorf("board sync %s: wipe: %w", p.Mint, err)
		}
	}
	if incomplete {
		if _, err := txr.ExecContext(ctx,
			`INSERT INTO project_incomplete (project) VALUES (?) ON CONFLICT(project) DO NOTHING`, p.Mint); err != nil {
			return fmt.Errorf("board sync %s: incomplete: %w", p.Mint, err)
		}
	} else {
		if _, err := txr.ExecContext(ctx, `DELETE FROM project_incomplete WHERE project = ?`, p.Mint); err != nil {
			return fmt.Errorf("board sync %s: incomplete: %w", p.Mint, err)
		}
	}
	if _, err := txr.ExecContext(ctx,
		`INSERT INTO project_sources (project, source) VALUES (?, ?)
		 ON CONFLICT(project) DO UPDATE SET source = excluded.source`,
		p.Mint, string(source)); err != nil {
		return fmt.Errorf("board sync %s: source: %w", p.Mint, err)
	}
	var maxSeq int64
	if err := txr.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM messages WHERE mint = ?`, p.Mint).Scan(&maxSeq); err != nil {
		return fmt.Errorf("board sync %s: %w", p.Mint, err)
	}
	next := maxSeq
	for _, r := range rows {
		seq := int64(r.MessageID)
		if source == client.SourceScan {
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
	if err := cacheLedger(ctx, txr, p.Mint, up); err != nil {
		return fmt.Errorf("board sync %s: %w", p.Mint, err)
	}
	if err := s.project(bound, p.Mint, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) project(bound context.Context, mint string, now time.Time) error {
	memos, err := cacheMemos(bound, mint)
	if err != nil {
		return fmt.Errorf("board fold %s: %w", mint, err)
	}
	ledger, err := ledgerIn(bound, mint)
	if err != nil {
		return fmt.Errorf("board fold %s: ledger: %w", mint, err)
	}
	tasks := Fold(mint, memos, ledger, now)
	if err := rewrite(bound, mint, tasks); err != nil {
		return err
	}
	return nil
}

func cacheMemos(bound context.Context, mint string) ([]Memo, error) {
	rows, err := domain.NewMessageDomain().WindowMessageByMint(bound, mint, 0, math.MaxInt64, 1<<30).Rows()
	if err != nil {
		return nil, err
	}
	memos := make([]Memo, 0, len(rows))
	for _, r := range rows {
		m, ok := ParseMemo(r.Memo)
		if !ok {
			continue
		}
		m.Sender = r.Sender
		m.At = r.CreatedAt
		m.Signature = r.Signature
		memos = append(memos, m)
	}
	return memos, nil
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
	if _, err := txr.Exec("DELETE FROM task_backing WHERE project = ?", mint); err != nil {
		return fmt.Errorf("board fold %s: backing: %w", mint, err)
	}
	td := domain.NewTaskDomain()
	nd := domain.NewNoteDomain()
	for _, t := range tasks {
		if _, err := td.InsertTask(bound, domain.Task{
			Project: t.Project, Id: strconv.Itoa(t.ID), Title: t.Title, Brief: t.Brief,
			Status: t.Status, Funder: t.Funder, Owner: t.Owner, ClaimedAt: t.ClaimedAt,
			CompletedAt: t.CompletedAt, AcceptedBy: t.AcceptedBy,
			RejectedBy: t.RejectedBy, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		}); err != nil {
			return fmt.Errorf("board fold %s: task %d: %w", mint, t.ID, err)
		}
		if t.Backing != "" || t.RejectShort > 0 {
			if _, err := txr.Exec(
				`INSERT INTO task_backing (project, id, backing, stake, vault, position_index, reject_short) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				mint, strconv.Itoa(t.ID), t.Backing, int64(t.Stake), t.Position.Vault, int64(t.Position.Index), int64(t.RejectShort)); err != nil {
				return fmt.Errorf("board fold %s: backing %d: %w", mint, t.ID, err)
			}
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

type act struct {
	shape   Shape
	carrier client.Carrier
	write   func(context.Context, string) (client.WriteResult, error)
}

func (s *Store) Act(ctx context.Context, p Project, shape Shape) (string, error) {
	tc, err := s.client()
	if err != nil {
		return "", err
	}
	return s.act(ctx, p, act{
		shape:   shape,
		carrier: client.Carrier{Lamports: client.MemoBuyLamports},
		write: func(ctx context.Context, memo string) (client.WriteResult, error) {
			market, err := s.Market(ctx, p.Mint)
			if err != nil {
				return client.WriteResult{}, err
			}
			return tc.WriteAction(ctx, market, client.ActionPost, memo, client.MemoBuyLamports)
		},
	})
}

func (s *Store) Contract(ctx context.Context, p Project, id int, lamports uint64) (string, error) {
	tc, err := s.client()
	if err != nil {
		return "", err
	}
	if lamports <= client.MemoBuyLamports {
		return "", fmt.Errorf("board contract: %d lamports is the memo stake or under; a contract is capital above it", lamports)
	}
	return s.act(ctx, p, act{
		shape:   Shape{Verb: "claim", ID: id},
		carrier: client.Carrier{Lamports: lamports},
		write: func(ctx context.Context, memo string) (client.WriteResult, error) {
			market, err := s.Market(ctx, p.Mint)
			if err != nil {
				return client.WriteResult{}, err
			}
			return tc.WriteAction(ctx, market, client.ActionBack, memo, lamports)
		},
	})
}

func (s *Store) Work(ctx context.Context, p Project, id int, collateral, minOut uint64) (string, error) {
	tc, err := s.client()
	if err != nil {
		return "", err
	}
	index, err := tc.NextPositionIndex(ctx, p.Mint, client.SideLong)
	if err != nil {
		return "", fmt.Errorf("board work: %w", err)
	}
	return s.act(ctx, p, act{
		shape:   Shape{Verb: "claim", ID: id},
		carrier: client.Carrier{Long: true, Vault: tc.VaultPDA(), Index: index, Collateral: collateral},
		write: func(ctx context.Context, memo string) (client.WriteResult, error) {
			market, err := s.Market(ctx, p.Mint)
			if err != nil {
				return client.WriteResult{}, err
			}
			return tc.WritePosition(ctx, market, client.PositionWrite{
				Side: client.SideLong, Open: true, Index: index, Amount: collateral, MinOut: minOut, Memo: memo,
			})
		},
	})
}

func (s *Store) Release(ctx context.Context, p Project, id int, repayBPS uint16, minOut uint64) (string, error) {
	tc, err := s.client()
	if err != nil {
		return "", err
	}
	if err := s.Sync(ctx, p, 100); err != nil {
		return "", err
	}
	held, err := s.Held(ctx, p, tc.AgentPublic())
	if err != nil {
		return "", err
	}
	var task *Task
	for i := range held {
		if held[i].ID == id {
			task = &held[i]
		}
	}
	if task == nil {
		return "", fmt.Errorf("board release: you do not hold task %d on %s", id, p.Mint)
	}
	if repayBPS == 0 {
		repayBPS = client.FullRepayBPS
	}
	market, err := s.Market(ctx, p.Mint)
	if err != nil {
		return "", fmt.Errorf("board release: %w", err)
	}
	if task.Backing == BackingWork && repayBPS < client.FullRepayBPS {
		res, err := tc.WritePosition(ctx, market, client.PositionWrite{
			Side: client.SideLong, Index: task.Position.Index, RepayBPS: repayBPS, MinOut: minOut,
		})
		if err != nil {
			return "", fmt.Errorf("board release: %w", err)
		}
		if err := client.WaitConfirmed(ctx, tc.RPC, res.Signature, 30*time.Second); err != nil {
			return "", fmt.Errorf("board release: %w", err)
		}
		return fmt.Sprintf("%s partial release of t%d (%d bps): the claim stands while the position does", res.Signature, id, repayBPS), nil
	}
	write := func(ctx context.Context, memo string) (client.WriteResult, error) {
		if task.Backing == BackingWork {
			return tc.WritePosition(ctx, market, client.PositionWrite{
				Side: client.SideLong, Index: task.Position.Index, RepayBPS: client.FullRepayBPS, MinOut: minOut, Memo: memo,
			})
		}
		return tc.WriteAction(ctx, market, client.ActionExit, memo, 0)
	}
	return s.act(ctx, p, act{shape: Shape{Verb: "release", ID: id}, write: write})
}

func (s *Store) ShortReject(ctx context.Context, p Project, id int, reason string, collateral, minOut uint64) (string, error) {
	tc, err := s.client()
	if err != nil {
		return "", err
	}
	index, err := tc.NextPositionIndex(ctx, p.Mint, client.SideShort)
	if err != nil {
		return "", fmt.Errorf("board short: %w", err)
	}
	return s.act(ctx, p, act{
		shape:   Shape{Verb: "reject", ID: id, Text: reason},
		carrier: client.Carrier{Short: true, Vault: tc.VaultPDA(), Index: index, Collateral: collateral},
		write: func(ctx context.Context, memo string) (client.WriteResult, error) {
			market, err := s.Market(ctx, p.Mint)
			if err != nil {
				return client.WriteResult{}, err
			}
			return tc.WritePosition(ctx, market, client.PositionWrite{
				Side: client.SideShort, Open: true, Index: index, Amount: collateral, MinOut: minOut, Memo: memo,
			})
		},
	})
}

func (s *Store) Held(ctx context.Context, p Project, owner string) ([]Task, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	tasks, err := s.foldIn(bound, p.Mint, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	var out []Task
	for _, t := range tasks {
		if t.Owner == owner && (t.Status == StatusActive || t.Status == StatusReview) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *Store) foldIn(bound context.Context, mint string, now time.Time) ([]Task, error) {
	memos, err := cacheMemos(bound, mint)
	if err != nil {
		return nil, err
	}
	ledger, err := ledgerIn(bound, mint)
	if err != nil {
		return nil, err
	}
	return Fold(mint, memos, ledger, now), nil
}

func (s *Store) act(ctx context.Context, p Project, a act) (string, error) {
	tc, err := s.client()
	if err != nil {
		return "", err
	}
	if err := s.Sync(ctx, p, 100); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	shape := a.shape
	if shape.Verb == "task" && shape.ID == 0 {
		id, err := s.NextID(ctx, p)
		if err != nil {
			return "", err
		}
		shape.ID = id
	}
	memo, err := MemoFor(shape)
	if err != nil {
		return "", err
	}
	if err := s.refuseForeign(ctx, p, memo, tc.AgentPublic(), a.carrier, now); err != nil {
		return "", err
	}
	res, err := a.write(ctx, memo)
	if err != nil {
		return "", fmt.Errorf("board act %s: %w", shape.Verb, err)
	}
	if err := client.WaitConfirmed(ctx, tc.RPC, res.Signature, 30*time.Second); err != nil {
		return "", fmt.Errorf("board act %s: %w", shape.Verb, err)
	}
	landed, err := s.awaitCached(ctx, p, res.Signature)
	if err != nil {
		return "", fmt.Errorf("board act %s: re-sync: %w", shape.Verb, err)
	}
	if !landed {
		if s.OnAct != nil {
			s.OnAct(ctx, shape)
		}
		return res.Signature + " " + memo + "\npending: the board has not seen it yet", nil
	}
	board, err := s.BoardFromCache(ctx, p)
	if err != nil {
		return "", err
	}
	if shape.Verb == "task" {
		if assigned, renumbered, err := s.assignedID(ctx, p, memo, tc.AgentPublic(), now); err == nil && renumbered {
			memoID := shape.ID
			shape.ID = assigned
			board = fmt.Sprintf("board: assigned id %d (the memo's id %d was taken; claim %d, not %d)\n%s",
				assigned, memoID, assigned, memoID, board)
		}
	}
	if s.OnAct != nil {
		s.OnAct(ctx, shape)
	}
	return res.Signature + " " + memo + "\n" + board, nil
}

func (s *Store) awaitCached(ctx context.Context, p Project, sig string) (bool, error) {
	budget := s.WaitBudget
	if budget <= 0 {
		budget = 15 * time.Second
	}
	deadline := time.Now().Add(budget)
	for {
		if err := s.Sync(ctx, p, 100); err != nil {
			return false, err
		}
		ok, err := s.messageCached(ctx, p, sig)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (s *Store) messageCached(ctx context.Context, p Project, sig string) (bool, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var one int
	err = tx.QueryRowContext(bound, `SELECT 1 FROM messages WHERE mint = ? AND signature = ?`, p.Mint, sig).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) assignedID(ctx context.Context, p Project, memo, sender string, now time.Time) (int, bool, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	candidate, ok := ParseMemo(memo)
	if !ok {
		return 0, false, fmt.Errorf("board: candidate %q does not parse", memo)
	}
	candidate.Sender = sender
	tasks, err := s.foldIn(bound, p.Mint, now)
	if err != nil {
		return 0, false, err
	}
	best := 0
	for _, t := range tasks {
		if t.Title == candidate.Text && t.Funder == candidate.Sender {
			if t.ID > best {
				best = t.ID
			}
		}
	}
	if best == 0 {
		return 0, false, nil
	}
	return best, best != candidate.ID, nil
}

const candidateSignature = "candidate"

func (s *Store) refuseForeign(ctx context.Context, p Project, memo, sender string, carrier client.Carrier, now time.Time) error {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	memos, err := cacheMemos(bound, p.Mint)
	if err != nil {
		return err
	}
	ledger, err := ledgerIn(bound, p.Mint)
	if err != nil {
		return err
	}
	candidate, ok := ParseMemo(memo)
	if !ok {
		return fmt.Errorf("board act: candidate %q does not parse", memo)
	}
	candidate.Sender = sender
	candidate.At = now.Format(time.RFC3339)
	candidate.Signature = candidateSignature
	if gated(candidate.Verb) && !ledger.Public {
		return fmt.Errorf("board act %s: the project is not public (tasks and claims land only on a public project; invest, note, and post stand)", candidate.Verb)
	}
	before := Fold(p.Mint, memos, ledger, now)
	after := Fold(p.Mint, append(append([]Memo{}, memos...), candidate), ledger.with(candidateSignature, carrier), now)
	if tasksEqual(before, after) {
		return fmt.Errorf("board act %s: the board refuses %s (wrong state or a stale id; read the board and retry)", candidate.Verb, memo)
	}
	return nil
}

func tasksEqual(a, b []Task) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !taskEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func taskEqual(a, b Task) bool {
	if a.Project != b.Project || a.ID != b.ID || a.Title != b.Title || a.Brief != b.Brief ||
		a.Status != b.Status || a.Funder != b.Funder || a.Owner != b.Owner ||
		a.ClaimedAt != b.ClaimedAt || a.CompletedAt != b.CompletedAt ||
		a.AcceptedBy != b.AcceptedBy || a.RejectedBy != b.RejectedBy ||
		a.CreatedAt != b.CreatedAt || a.UpdatedAt != b.UpdatedAt ||
		a.Backing != b.Backing || a.Stake != b.Stake || a.Position != b.Position ||
		a.RejectShort != b.RejectShort {
		return false
	}
	if len(a.Notes) != len(b.Notes) {
		return false
	}
	for i := range a.Notes {
		if a.Notes[i] != b.Notes[i] {
			return false
		}
	}
	return true
}

func (s *Store) Market(ctx context.Context, mint string) (client.MarketRow, error) {
	tc, err := s.client()
	if err != nil {
		return client.MarketRow{}, err
	}
	if tc.Indexer != "" {
		detail, err := tc.API.Market(ctx, mint)
		if err != nil {
			return client.MarketRow{}, err
		}
		return detail.Market, nil
	}
	return client.MarketFromRPC(ctx, tc.RPC, tc.ProgramID, mint)
}

func (s *Store) Board(ctx context.Context, p Project) (string, error) {
	err := s.Sync(ctx, p, 100)
	stale := errors.Is(err, ErrIndexerUnreachable)
	if err != nil && !stale {
		return "", err
	}
	board, err := s.BoardFromCache(ctx, p)
	if err != nil {
		return "", err
	}
	if stale {
		board += "\nindexer unreachable: board may be stale"
	}
	return board, nil
}

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
	incomplete, err := incompleteIn(bound, p.Mint)
	if err != nil {
		return "", err
	}
	backings, err := backingsIn(bound, p.Mint)
	if err != nil {
		return "", err
	}
	return renderBoard(s.label(p), rows, backings, goal, time.Now(), incomplete), nil
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

type Summary struct {
	Goal       string
	TotalTasks int
	DoneTasks  int
	OpenTasks  int
	OpenClaims int
}

func (s *Store) Summary(ctx context.Context, mint string) (Summary, bool, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return Summary{}, false, err
	}
	defer tx.Rollback()
	cached, err := cachedIn(bound, mint)
	if err != nil {
		return Summary{}, false, err
	}
	if !cached {
		return Summary{}, false, nil
	}
	goal, err := s.goalOf(bound, mint)
	if err != nil {
		return Summary{}, false, err
	}
	rows, err := domain.NewTaskDomain().WindowTaskByProject(bound, mint, "", "\uffff", 1<<30).Rows()
	if err != nil {
		return Summary{}, false, err
	}
	sum := Summary{Goal: goal}
	for _, r := range rows {
		sum.TotalTasks++
		switch r.Status {
		case StatusDone:
			sum.DoneTasks++
		case StatusActive:
			sum.OpenClaims++
		}
	}
	sum.OpenTasks = sum.TotalTasks - sum.DoneTasks
	return sum, true, nil
}

func (s *Store) Projects(ctx context.Context) ([]string, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(bound, `SELECT project FROM project_sources`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Accepts(ctx context.Context, owner string) (int, error) {
	projects, err := s.Projects(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, mint := range projects {
		bound, tx, err := s.DB.TxReadOnly(ctx)
		if err != nil {
			return 0, err
		}
		rows, err := domain.NewTaskDomain().WindowTaskByProject(bound, mint, "", "\uffff", 1<<30).Rows()
		tx.Rollback()
		if err != nil {
			return 0, err
		}
		for _, r := range rows {
			if r.Owner == owner && r.Status == StatusDone {
				n++
			}
		}
	}
	return n, nil
}

func cachedIn(bound context.Context, mint string) (bool, error) {
	tx, err := sqlx.TxFrom(bound)
	if err != nil {
		return false, err
	}
	var one int
	err = tx.QueryRowContext(bound, `SELECT 1 FROM project_sources WHERE project = ?`, mint).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

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

type TaskInfo struct {
	ID     string
	Title  string
	Brief  string
	Status string
	Owner  string
	Notes  []Note
}

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
	return pid8(p.Mint)
}

func pid8(mint string) string {
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

func (s *Store) Claims(ctx context.Context, p Project, owner string) (int, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := domain.NewTaskDomain().WindowTaskByProject(bound, p.Mint, "", "\uffff", 1<<30).Rows()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if r.Status == StatusActive && r.Owner == owner {
			n++
		}
	}
	return n, nil
}

func (s *Store) LastMemo(ctx context.Context, p Project, sender string) (Memo, bool, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return Memo{}, false, err
	}
	defer tx.Rollback()
	rows, err := domain.NewMessageDomain().WindowMessageByMint(bound, p.Mint, 0, math.MaxInt64, 1<<30).Rows()
	if err != nil {
		return Memo{}, false, err
	}
	var latest Memo
	found := false
	for _, r := range rows {
		if r.Sender != sender {
			continue
		}
		m, ok := ParseMemo(r.Memo)
		if !ok {
			continue
		}
		m.Sender = r.Sender
		m.At = r.CreatedAt
		latest = m
		found = true
	}
	return latest, found, nil
}
