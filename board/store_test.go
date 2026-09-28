package board

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/rig/v2/store"

	"github.com/mrsirg97-rgb/orbit/client"
)

type flakyAPI struct {
	messages []client.MessageRow
	err      error
	calls    int
}

func (a *flakyAPI) Markets(context.Context, url.Values) ([]client.MarketRow, error) { return nil, nil }
func (a *flakyAPI) Market(_ context.Context, mint string) (client.MarketDetail, error) {
	return migratedDetail(mint), nil
}
func (a *flakyAPI) Messages(context.Context, url.Values) ([]client.MessageRow, error) {
	a.calls++
	return a.messages, a.err
}
func (a *flakyAPI) Trades(context.Context, url.Values) ([]client.TradeRow, error) { return nil, nil }
func (a *flakyAPI) Positions(context.Context, url.Values) ([]client.PositionRow, error) {
	return nil, nil
}
func (a *flakyAPI) PositionEvents(context.Context, url.Values) ([]client.PositionEventRow, error) {
	return nil, nil
}
func (a *flakyAPI) Migrations(context.Context, url.Values) ([]client.MigrationRow, error) {
	return nil, nil
}
func (a *flakyAPI) Pnl(context.Context, string, string) (client.PnlSummary, error) {
	return client.PnlSummary{}, nil
}
func (a *flakyAPI) Swaps(context.Context, url.Values) ([]client.SwapRow, error) { return nil, nil }

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	var buf bytes.Buffer
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		io.Copy(&buf, r)
	}()
	defer func() {
		os.Stdout = old
		r.Close()
	}()
	fn()
	w.Close()
	<-readDone
	return buf.String()
}

func seedScanLog(rpc *boardFakeRPC, mint string, rows []client.MessageRow) {
	at := int64(1767268980)
	for _, r := range rows {
		tx := &client.Transaction{Slot: r.Slot, BlockTime: &at, Keys: []string{r.Sender}}
		tx.Ixs = append(tx.Ixs, client.TxInstruction{ProgramID: client.MemoProgram, Data: []byte(r.MemoText)})
		rpc.txs[r.Signature] = tx
		rpc.msgs = append(rpc.msgs, client.SignatureInfo{Signature: r.Signature, BlockTime: &at})
	}
}

func storeWithAPI(t *testing.T, api client.API) (store.DB, *Store, *boardFakeRPC) {
	t.Helper()
	rpc := newBoardFakeRPC(t, testMint)
	tc := boardClientFor(t, testMint, "orbit-board-store-seed-0000000", rpc)
	tc.API = api
	tc.Config.Indexer = "http://127.0.0.1:1"
	db, st := openBoardStoreWith(t, tc)
	return db, st, rpc
}

func recordedSource(t *testing.T, db store.DB, mint string) (string, bool) {
	t.Helper()
	bound, tx, err := db.TxReadOnly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var src string
	err = tx.QueryRowContext(bound, `SELECT source FROM project_sources WHERE project = ?`, mint).Scan(&src)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return src, true
}

func TestSyncFallsBackToScanOnIndexerUnreachable(t *testing.T) {
	ctx := context.Background()
	api := &flakyAPI{err: &client.HTTPStatusError{Path: "/api/messages", Code: 503, Body: "down"}}
	db, st, rpc := storeWithAPI(t, api)
	defer db.DB.Close()
	seedScanLog(rpc, testMint, []client.MessageRow{
		{Mint: testMint, Sender: "9BsnjSj5gNrkKKpCmWqSAPrDKtnH3yEwzJf5YzCX7MDz",
			MemoText: "task 1: Scan sourced", Slot: 110, Signature: "sigScan1"},
	})
	out := captureStdout(t, func() {
		if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
			t.Fatalf("sync: %v", err)
		}
	})
	if !strings.Contains(out, "source switched from indexer to RPC scan") {
		t.Errorf("the fallback must print one switch line: %q", out)
	}
	if src, ok := recordedSource(t, db, testMint); !ok || src != string(client.SourceScan) {
		t.Errorf("recorded source: %q (%v), want scan", src, ok)
	}
	board, err := st.BoardFromCache(ctx, Project{Mint: testMint})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(board, "t1 pending") || !strings.Contains(board, "Scan sourced") {
		t.Errorf("the scan read did not serve the board:\n%s", board)
	}
}

func TestScanSourceStickyAfterIndexerRecovery(t *testing.T) {
	ctx := context.Background()
	api := &flakyAPI{err: &client.HTTPStatusError{Path: "/api/messages", Code: 503, Body: "down"}}
	db, st, rpc := storeWithAPI(t, api)
	defer db.DB.Close()
	seedScanLog(rpc, testMint, []client.MessageRow{
		{Mint: testMint, Sender: "9BsnjSj5gNrkKKpCmWqSAPrDKtnH3yEwzJf5YzCX7MDz",
			MemoText: "task 1: Scan sourced", Slot: 110, Signature: "sigScan1"},
	})
	captureStdout(t, func() {
		if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
			t.Fatalf("sync: %v", err)
		}
	})

	api.err = nil
	api.messages = []client.MessageRow{
		{Mint: testMint, MessageID: 1, Sender: "walletX",
			MemoText: "task 1: Indexer sourced", Slot: 111, Signature: "sigIndexer1"},
	}
	seedScanLog(rpc, testMint, []client.MessageRow{
		{Mint: testMint, Sender: "9BsnjSj5gNrkKKpCmWqSAPrDKtnH3yEwzJf5YzCX7MDz",
			MemoText: "task 2: Still scanned", Slot: 115, Signature: "sigScan2"},
	})
	out := captureStdout(t, func() {
		if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
			t.Fatalf("re-sync: %v", err)
		}
	})
	if strings.Contains(out, "source switched") {
		t.Errorf("a recovered indexer must not re-source a scanned mint: %q", out)
	}
	if api.calls != 1 {
		t.Errorf("indexer calls: %d, want 1 (the recovered indexer must not be re-tried)", api.calls)
	}
	if src, ok := recordedSource(t, db, testMint); !ok || src != string(client.SourceScan) {
		t.Errorf("recorded source: %q (%v), want scan", src, ok)
	}
	board, err := st.BoardFromCache(ctx, Project{Mint: testMint})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(board, "Scan sourced") || !strings.Contains(board, "Still scanned") {
		t.Errorf("the mint must keep its scan source:\n%s", board)
	}
	if strings.Contains(board, "Indexer sourced") {
		t.Errorf("an indexer-sourced row leaked into a scanned mint:\n%s", board)
	}
}

func TestSyncDoesNotFallBackOnIndexer404(t *testing.T) {
	ctx := context.Background()
	api := &flakyAPI{err: &client.HTTPStatusError{Path: "/api/messages", Code: 404, Body: "unknown mint"}}
	db, st, rpc := storeWithAPI(t, api)
	defer db.DB.Close()
	seedScanLog(rpc, testMint, []client.MessageRow{
		{Mint: testMint, Sender: "9BsnjSj5gNrkKKpCmWqSAPrDKtnH3yEwzJf5YzCX7MDz",
			MemoText: "task 1: Never scanned", Slot: 110, Signature: "sigScan1"},
	})
	out := captureStdout(t, func() {
		err := st.Sync(ctx, Project{Mint: testMint}, 100)
		if err == nil || !strings.Contains(err.Error(), "status 404") {
			t.Fatalf("a 404 must fail loudly: %v", err)
		}
	})
	if strings.Contains(out, "source switched") {
		t.Errorf("a 4xx must never fall back: %q", out)
	}
	if _, ok := recordedSource(t, db, testMint); ok {
		t.Error("a failed sync must not record a source")
	}
	var n int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM messages WHERE mint = ?`, testMint).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a 404 must not serve the scan: %d rows", n)
	}
}

func setSource(t *testing.T, db store.DB, mint, source string) {
	t.Helper()
	bound, tx, err := db.Tx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(bound, `INSERT INTO project_sources (project, source) VALUES (?, ?)`, mint, source); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func messageSeqs(t *testing.T, db store.DB, mint string) []int64 {
	t.Helper()
	rows, err := db.DB.Query(`SELECT seq FROM messages WHERE mint = ? ORDER BY seq`, mint)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var seq int64
		if err := rows.Scan(&seq); err != nil {
			t.Fatal(err)
		}
		out = append(out, seq)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOutageOnWarmIndexerMintServesCacheAndRefusesAct(t *testing.T) {
	ctx := context.Background()
	api := &flakyAPI{}
	base := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	api.messages = []client.MessageRow{
		{Mint: testMint, MessageID: 1, Sender: "walletX", MemoText: "task 1: Warm from indexer",
			Slot: 1, Signature: "sigWarm1", CreatedAt: base},
	}
	db, st, rpc := storeWithAPI(t, api)
	defer db.DB.Close()
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatalf("warm sync: %v", err)
	}
	api.err = &client.HTTPStatusError{Path: "/api/messages", Code: 503, Body: "down"}
	project := Project{Mint: testMint}
	board, err := st.Board(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(board, "Warm from indexer") || !strings.Contains(board, "indexer unreachable: board may be stale") {
		t.Errorf("the outage must serve the cache with the stale line:\n%s", board)
	}
	if got := messageSeqs(t, db, testMint); len(got) != 1 || got[0] != 1 {
		t.Errorf("the outage must insert nothing: seqs %v", got)
	}
	if src, ok := recordedSource(t, db, testMint); !ok || src != string(client.SourceIndexer) {
		t.Errorf("recorded source: %q (%v), want indexer", src, ok)
	}
	if _, err := st.Act(ctx, project, Shape{Verb: "task", Text: "Outage act"}); err == nil || !strings.Contains(err.Error(), "indexer unreachable") {
		t.Fatalf("an act on a stale board must refuse: %v", err)
	}
	if len(rpc.sent) != 0 {
		t.Errorf("a refused act must not spend: %d sent txs", len(rpc.sent))
	}
}

func TestConfigChangeUnsetIndexerWipesAndRewalks(t *testing.T) {
	ctx := context.Background()
	rpc := newBoardFakeRPC(t, testMint)
	seedScanLog(rpc, testMint, []client.MessageRow{
		{Mint: testMint, Sender: "9BsnjSj5gNrkKKpCmWqSAPrDKtnH3yEwzJf5YzCX7MDz",
			MemoText: "task 1: Rewalked", Slot: 110, Signature: "sigScan1"},
		{Mint: testMint, Sender: "9BsnjSj5gNrkKKpCmWqSAPrDKtnH3yEwzJf5YzCX7MDz",
			MemoText: "task 2: Rewalked", Slot: 115, Signature: "sigScan2"},
	})
	api := &flakyAPI{}
	tc := boardClientFor(t, testMint, "orbit-board-store-seed-0000000", rpc)
	tc.API = api
	db, st := openBoardStoreWith(t, tc)
	defer db.DB.Close()
	base := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	seedRecordedLog(t, db, testMint, []client.MessageRow{
		{Mint: testMint, MessageID: 100, Sender: "walletX", MemoText: "task 1: Warm from indexer",
			Slot: 1, Signature: "sigWarm1", CreatedAt: base},
		{Mint: testMint, MessageID: 101, Sender: "walletX", MemoText: "task 2: Warm from indexer",
			Slot: 2, Signature: "sigWarm2", CreatedAt: base},
	})
	setSource(t, db, testMint, string(client.SourceIndexer))
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if api.calls != 0 {
		t.Errorf("indexer calls: %d, want 0 (unset means scan only)", api.calls)
	}
	if got := messageSeqs(t, db, testMint); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Errorf("the rewalk must renumber from 1: %v", got)
	}
	if src, ok := recordedSource(t, db, testMint); !ok || src != string(client.SourceScan) {
		t.Errorf("recorded source: %q (%v), want scan", src, ok)
	}
	board, err := st.BoardFromCache(ctx, Project{Mint: testMint})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(board, "Rewalked") || strings.Contains(board, "Warm from indexer") {
		t.Errorf("the config change must wipe and rewalk:\n%s", board)
	}
}

func TestMintNeverMixesSources(t *testing.T) {
	ctx := context.Background()
	api := &flakyAPI{err: &client.HTTPStatusError{Path: "/api/messages", Code: 503, Body: "down"}}
	db, st, rpc := storeWithAPI(t, api)
	defer db.DB.Close()
	seedScanLog(rpc, testMint, []client.MessageRow{
		{Mint: testMint, Sender: "9BsnjSj5gNrkKKpCmWqSAPrDKtnH3yEwzJf5YzCX7MDz",
			MemoText: "task 3: Scan after", Slot: 120, Signature: "sigScan3"},
	})
	base := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	seedRecordedLog(t, db, testMint, []client.MessageRow{
		{Mint: testMint, MessageID: 7, Sender: "walletX", MemoText: "task 1: Warm from indexer",
			Slot: 1, Signature: "sigWarm1", CreatedAt: base},
	})
	setSource(t, db, testMint, string(client.SourceIndexer))
	if _, err := st.Board(ctx, Project{Mint: testMint}); err != nil {
		t.Fatal(err)
	}
	if got := messageSeqs(t, db, testMint); !reflect.DeepEqual(got, []int64{7}) {
		t.Errorf("an outage must not land scan rows: %v", got)
	}
	tc, _ := st.client()
	tc.Config.Indexer = ""
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := messageSeqs(t, db, testMint); !reflect.DeepEqual(got, []int64{1}) {
		t.Errorf("the rewalk must leave only scan rows: %v", got)
	}
}
