package board

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/rig/store"

	"github.com/mrsirg97-rgb/orbit/client"
)

type pagingAPI struct {
	messages []client.MessageRow
	calls    int
	last     url.Values
}

func (a *pagingAPI) Markets(context.Context, url.Values) ([]client.MarketRow, error) {
	return nil, nil
}
func (a *pagingAPI) Market(context.Context, string) (client.MarketDetail, error) {
	return client.MarketDetail{}, nil
}
func (a *pagingAPI) Messages(ctx context.Context, q url.Values) ([]client.MessageRow, error) {
	a.calls++
	a.last = q
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil {
		return nil, err
	}
	before := q.Get("before")
	out := make([]client.MessageRow, 0, limit)
	for i := len(a.messages) - 1; i >= 0; i-- {
		m := a.messages[i]
		if before != "" && m.CreatedAt >= before {
			continue
		}
		out = append(out, m)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
func (a *pagingAPI) Trades(context.Context, url.Values) ([]client.TradeRow, error) {
	return nil, nil
}
func (a *pagingAPI) Positions(context.Context, url.Values) ([]client.PositionRow, error) {
	return nil, nil
}
func (a *pagingAPI) PositionEvents(context.Context, url.Values) ([]client.PositionEventRow, error) {
	return nil, nil
}
func (a *pagingAPI) Migrations(context.Context, url.Values) ([]client.MigrationRow, error) {
	return nil, nil
}
func (a *pagingAPI) Pnl(context.Context, string, string) (client.PnlSummary, error) {
	return client.PnlSummary{}, nil
}
func (a *pagingAPI) Swaps(context.Context, url.Values) ([]client.SwapRow, error) {
	return nil, nil
}

func walkLog(n int, sameSecond bool) []client.MessageRow {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	out := make([]client.MessageRow, 0, n)
	for i := 0; i < n; i++ {
		at := base.Add(time.Duration(i) * time.Second)
		if sameSecond {
			at = base.Add(time.Duration(i/10) * time.Second)
		}
		out = append(out, client.MessageRow{
			MessageID: int32(i + 1),
			Mint:      testMint,
			Sender:    "walletX",
			MemoText:  fmt.Sprintf("goal: walk memo %d", i),
			Slot:      int64(i),
			Signature: fmt.Sprintf("sigWalk%d", i),
			CreatedAt: at.UTC().Format(time.RFC3339),
		})
	}
	return out
}

func storeWithPagingAPI(t *testing.T, api client.API) (store.DB, *Store, *client.TorchClient) {
	t.Helper()
	rpc := newBoardFakeRPC(t, testMint)
	tc := boardClientFor(t, testMint, "orbit-board-walk-seed-00000000", rpc)
	tc.API = api
	tc.Config.Indexer = "http://127.0.0.1:1"
	db, st := openBoardStoreWith(t, tc)
	return db, st, tc
}

func cachedCount(t *testing.T, db store.DB, mint string) int {
	t.Helper()
	bound, tx, err := db.TxReadOnly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(bound, `SELECT COUNT(*) FROM messages WHERE mint = ?`, mint).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWalkSyncsFullLogFromEmptyInThreePages(t *testing.T) {
	ctx := context.Background()
	api := &pagingAPI{messages: walkLog(250, false)}
	db, st, _ := storeWithPagingAPI(t, api)
	defer db.DB.Close()
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatal(err)
	}
	if api.calls != 3 {
		t.Errorf("indexer calls: %d, want 3 (100 + 100 + 50, the short page stops the walk)", api.calls)
	}
	if got := cachedCount(t, db, testMint); got != 250 {
		t.Errorf("cached messages: %d, want 250", got)
	}
	board, err := st.BoardFromCache(ctx, Project{Mint: testMint})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(board, "incomplete") {
		t.Errorf("a walk to genesis must not mark the cache incomplete:\n%s", board)
	}
}

func TestWalkSameSecondBoundaryLosesNothing(t *testing.T) {
	ctx := context.Background()
	api := &pagingAPI{messages: walkLog(250, true)}
	db, st, _ := storeWithPagingAPI(t, api)
	defer db.DB.Close()
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatal(err)
	}
	if api.calls != 3 {
		t.Errorf("indexer calls: %d, want 3 (the boundary second is re-fetched, signatures dedupe)", api.calls)
	}
	if got := cachedCount(t, db, testMint); got != 250 {
		t.Errorf("cached messages: %d, want 250 (no memo lost at a same-second boundary)", got)
	}
	if before := api.last.Get("before"); before != "2026-01-01T12:00:07Z" {
		t.Errorf("the walk must page with before=<oldest seen + 1s>, got %q", before)
	}
}

func TestWalkWarmCacheReadsOnePage(t *testing.T) {
	ctx := context.Background()
	api := &pagingAPI{messages: walkLog(250, false)}
	db, st, _ := storeWithPagingAPI(t, api)
	defer db.DB.Close()
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatal(err)
	}
	first := api.calls
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatal(err)
	}
	if api.calls-first != 1 {
		t.Errorf("warm sync requests: %d, want 1 (the first page already holds a cached signature)", api.calls-first)
	}
	if got := cachedCount(t, db, testMint); got != 250 {
		t.Errorf("cached messages: %d, want 250", got)
	}
}

func TestWalkBoundMarksCacheIncomplete(t *testing.T) {
	ctx := context.Background()
	api := &pagingAPI{messages: walkLog(5100, false)}
	db, st, _ := storeWithPagingAPI(t, api)
	defer db.DB.Close()
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatal(err)
	}
	if api.calls != WalkBound {
		t.Errorf("indexer calls: %d, want %d (the walk bound)", api.calls, WalkBound)
	}
	want := WalkBound*100 - (WalkBound - 1)
	if got := cachedCount(t, db, testMint); got != want {
		t.Errorf("cached messages: %d, want %d (50 pages, one boundary row deduped per page)", got, want)
	}
	board, err := st.Board(ctx, Project{Mint: testMint})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(board, "incomplete") {
		t.Errorf("the board must say the cache is incomplete:\n%s", board)
	}
}

func TestWalkGoalIsTheFirstGoalMemoInTheWalkedLog(t *testing.T) {
	ctx := context.Background()
	rows := walkLog(250, false)
	rows[0].MemoText = "goal: Original goal."
	rows[150].MemoText = "goal: Later goal."
	api := &pagingAPI{messages: rows}
	db, st, _ := storeWithPagingAPI(t, api)
	defer db.DB.Close()
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatal(err)
	}
	board, err := st.Board(ctx, Project{Mint: testMint})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(board, "Original goal") || strings.Contains(board, "Later goal") {
		t.Errorf("the goal must be the first goal memo in the walked log:\n%s", board)
	}
}

type scanPagingRPC struct {
	*boardFakeRPC
	calls int
}

func (f *scanPagingRPC) GetSignaturesForAddress(ctx context.Context, address string, limit int, before string) ([]client.SignatureInfo, error) {
	f.calls++
	start := len(f.msgs)
	if before != "" {
		found := false
		for i, m := range f.msgs {
			if m.Signature == before {
				start = i
				found = true
				break
			}
		}
		if !found {
			return nil, nil
		}
	}
	out := make([]client.SignatureInfo, 0, limit)
	for i := start - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, f.msgs[i])
	}
	return out, nil
}

func TestWalkScanPagesWithSignatureCursor(t *testing.T) {
	ctx := context.Background()
	rpc := newBoardFakeRPC(t, testMint)
	scan := &scanPagingRPC{boardFakeRPC: rpc}
	rows := make([]client.MessageRow, 250)
	for i := range rows {
		rows[i] = client.MessageRow{
			Mint: testMint, Sender: "walletX", MemoText: fmt.Sprintf("goal: scan memo %d", i),
			Slot: int64(i), Signature: fmt.Sprintf("sigWalkScan%d", i),
		}
	}
	seedScanLog(rpc, testMint, rows)
	tc := boardClientFor(t, testMint, "orbit-board-walk-scan-00000000", rpc)
	tc.RPC = scan
	db, st := openBoardStoreWith(t, tc)
	defer db.DB.Close()
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatal(err)
	}
	if scan.calls != 3 {
		t.Errorf("scan calls: %d, want 3 (the signature cursor pages to genesis)", scan.calls)
	}
	if got := cachedCount(t, db, testMint); got != 250 {
		t.Errorf("cached messages: %d, want 250", got)
	}
	bound, tx, err := db.TxReadOnly(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var first string
	if err := tx.QueryRowContext(bound, `SELECT signature FROM messages WHERE mint = ? ORDER BY seq LIMIT 1`, testMint).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if first != "sigWalkScan0" {
		t.Errorf("seq 1 = %s, want sigWalkScan0 (the scan's local seq follows log order)", first)
	}
}

type memoCurveRPC struct {
	*boardFakeRPC
	sigs  []string
	memos map[string]string
	calls int
}

func (f *memoCurveRPC) GetSignaturesForAddress(ctx context.Context, address string, limit int, before string) ([]client.SignatureInfo, error) {
	f.calls++
	start := len(f.sigs)
	if before != "" {
		found := false
		for i, s := range f.sigs {
			if s == before {
				start = i
				found = true
				break
			}
		}
		if !found {
			return nil, nil
		}
	}
	out := make([]client.SignatureInfo, 0, limit)
	for i := start - 1; i >= 0 && len(out) < limit; i-- {
		at := int64(1767268980 + i)
		out = append(out, client.SignatureInfo{Signature: f.sigs[i], BlockTime: &at})
	}
	return out, nil
}

func (f *memoCurveRPC) GetTransaction(ctx context.Context, sig string) (*client.Transaction, error) {
	ix := -1
	for i, s := range f.sigs {
		if s == sig {
			ix = i
			break
		}
	}
	if ix < 0 {
		return nil, nil
	}
	at := int64(1767268980 + ix)
	tx := &client.Transaction{Slot: int64(ix), Keys: []string{"walletX"}, BlockTime: &at}
	if memo, ok := f.memos[sig]; ok {
		tx.Ixs = []client.TxInstruction{{ProgramID: client.MemoProgram, Data: []byte(memo)}}
	}
	return tx, nil
}

func memoCurve(t *testing.T, sigs int, memoAt map[int]string) *memoCurveRPC {
	t.Helper()
	rpc := newBoardFakeRPC(t, testMint)
	f := &memoCurveRPC{boardFakeRPC: rpc, memos: map[string]string{}}
	for i := 0; i < sigs; i++ {
		sig := fmt.Sprintf("sigCurve%d", i)
		f.sigs = append(f.sigs, sig)
		if memo, ok := memoAt[i]; ok {
			f.memos[sig] = memo
		}
	}
	return f
}

func TestWalkScanContinuesPastPagesWithNoMemos(t *testing.T) {
	ctx := context.Background()
	fake := memoCurve(t, 300, map[int]string{
		5:   "task 1: Oldest memo",
		150: "task 2: Middle memo",
		290: "task 3: Newest memo",
	})
	tc := boardClientFor(t, testMint, "orbit-board-walk-scan-sparse-0000", fake.boardFakeRPC)
	tc.RPC = fake
	db, st := openBoardStoreWith(t, tc)
	defer db.DB.Close()
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 4 {
		t.Errorf("scan calls: %d, want 4 (three full signature pages plus the empty genesis page)", fake.calls)
	}
	if got := cachedCount(t, db, testMint); got != 3 {
		t.Errorf("cached messages: %d, want 3 (only the memo signatures)", got)
	}
	board, err := st.Board(ctx, Project{Mint: testMint})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Oldest memo", "Middle memo", "Newest memo"} {
		if !strings.Contains(board, want) {
			t.Errorf("the board misses %q:\n%s", want, board)
		}
	}
}

func TestWalkScanWarmCacheStopsAfterOnePage(t *testing.T) {
	ctx := context.Background()
	fake := memoCurve(t, 300, map[int]string{
		5:   "task 1: Oldest memo",
		150: "task 2: Middle memo",
		290: "task 3: Newest memo",
	})
	tc := boardClientFor(t, testMint, "orbit-board-walk-scan-sparse-0001", fake.boardFakeRPC)
	tc.RPC = fake
	db, st := openBoardStoreWith(t, tc)
	defer db.DB.Close()
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatal(err)
	}
	before := fake.calls
	if err := st.Sync(ctx, Project{Mint: testMint}, 100); err != nil {
		t.Fatal(err)
	}
	if fake.calls-before != 1 {
		t.Errorf("warm scan sync requests: %d, want 1 (the first page holds a cached signature)", fake.calls-before)
	}
	if got := cachedCount(t, db, testMint); got != 3 {
		t.Errorf("cached messages: %d, want 3", got)
	}
}
