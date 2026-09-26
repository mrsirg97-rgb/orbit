package board

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/rig/store"

	"github.com/mrsirg97-rgb/orbit/board/domain"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/idl"
	"github.com/mrsirg97-rgb/orbit/sol"
)

const testMint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"

type boardFakeRPC struct {
	accounts  map[string]client.AccountInfo
	sent      []string
	sig       int
	msgs      []client.SignatureInfo
	txs       map[string]*client.Transaction
	now       func() time.Time
	visible   int // -1 = all sent messages; else only the first N
	lag       int // a sent message is revealed after N more scan calls
	syncCalls int
	sentAt    map[string]int
}

func (f *boardFakeRPC) GetLatestBlockhash(context.Context) (string, error) {
	return "11111111111111111111111111111111", nil
}
func (f *boardFakeRPC) SendTransaction(ctx context.Context, signed []byte) (string, error) {
	f.sig++
	sig := "sigBoard" + itoa(f.sig)
	f.sent = append(f.sent, sol.EncodeTx(signed))
	tx := decodeSentTx(signed)
	if tx == nil {
		return "", errors.New("fake rpc: sent tx did not decode")
	}
	slot := int64(len(f.msgs) + 1)
	tx.Slot = slot
	at := f.now().Unix()
	tx.BlockTime = &at
	if f.txs == nil {
		f.txs = map[string]*client.Transaction{}
	}
	if f.sentAt == nil {
		f.sentAt = map[string]int{}
	}
	f.txs[sig] = tx
	f.sentAt[sig] = f.syncCalls
	f.msgs = append(f.msgs, client.SignatureInfo{Signature: sig, BlockTime: &at})
	return sig, nil
}
func (f *boardFakeRPC) GetAccountInfo(ctx context.Context, pubkey string) (client.AccountInfo, error) {
	return f.accounts[pubkey], nil
}
func (f *boardFakeRPC) GetTokenAccountsByOwner(context.Context, string, string) ([]client.TokenAccount, error) {
	return nil, nil
}
func (f *boardFakeRPC) GetBalance(context.Context, string) (uint64, error) { return 0, nil }
func (f *boardFakeRPC) GetSignatureStatus(context.Context, string) (client.SignatureStatus, error) {
	return client.SignatureStatus{Exists: true, Confirmed: true}, nil
}
func (f *boardFakeRPC) RequestAirdrop(context.Context, string, uint64) (string, error) {
	return "", nil
}
func (f *boardFakeRPC) GetSignaturesForAddress(context.Context, string, int) ([]client.SignatureInfo, error) {
	f.syncCalls++
	out := make([]client.SignatureInfo, 0, len(f.msgs))
	for i, m := range f.msgs {
		if f.visible >= 0 && i >= f.visible {
			continue
		}
		if f.lag > 0 && f.syncCalls < f.sentAt[m.Signature]+f.lag {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}
func (f *boardFakeRPC) GetTransaction(ctx context.Context, sig string) (*client.Transaction, error) {
	tx, ok := f.txs[sig]
	if !ok {
		return nil, nil
	}
	return tx, nil
}

func decodeSentTx(signed []byte) *client.Transaction {
	if len(signed) < 67 || signed[0] != 1 || signed[65] != 0x80 {
		return nil
	}
	msg := signed[66 : len(signed)-1]
	pos := 3
	readShort := func() (int, bool) {
		if pos >= len(msg) {
			return 0, false
		}
		v := int(msg[pos])
		pos++
		if v < 0x80 {
			return v, true
		}
		v &= 0x7f
		if pos >= len(msg) {
			return 0, false
		}
		lo := int(msg[pos])
		pos++
		if lo < 0x80 {
			return v | (lo << 7), true
		}
		if pos >= len(msg) {
			return 0, false
		}
		lo &= 0x7f
		hi := int(msg[pos])
		pos++
		return v | (lo << 7) | (hi << 14), true
	}
	keyCount, ok := readShort()
	if !ok || keyCount == 0 || pos+keyCount*32+32 > len(msg) {
		return nil
	}
	keys := make([]string, 0, keyCount)
	for i := 0; i < keyCount; i++ {
		keys = append(keys, sol.Encode(msg[pos:pos+32]))
		pos += 32
	}
	pos += 32 // blockhash
	ixCount, ok := readShort()
	if !ok {
		return nil
	}
	tx := &client.Transaction{Keys: keys}
	for i := 0; i < ixCount; i++ {
		if pos+2 > len(msg) {
			return nil
		}
		progIdx := int(msg[pos])
		acctCount := int(msg[pos+1])
		pos += 2
		if pos+acctCount > len(msg) {
			return nil
		}
		pos += acctCount
		dataLen, ok := readShort()
		if !ok || pos+dataLen > len(msg) {
			return nil
		}
		if progIdx < len(keys) {
			tx.Ixs = append(tx.Ixs, client.TxInstruction{
				ProgramID: keys[progIdx], Data: msg[pos : pos+dataLen],
			})
		}
		pos += dataLen
	}
	return tx
}

func boardTestClient(t *testing.T, mint string) (*client.TorchClient, *boardFakeRPC) {
	t.Helper()
	rpc := newBoardFakeRPC(t, mint)
	return boardClientFor(t, mint, "orbit-board-swarm-seed-00000000", rpc), rpc
}

func newBoardFakeRPC(t *testing.T, mint string) *boardFakeRPC {
	t.Helper()
	rpc := &boardFakeRPC{
		accounts: map[string]client.AccountInfo{},
		txs:      map[string]*client.Transaction{},
		now:      time.Now,
		visible:  -1,
	}
	creator := "So11111111111111111111111111111111111111112"
	curve := make([]byte, 133)
	cb, _ := sol.Decode(creator)
	mb, _ := sol.Decode(mint)
	copy(curve[8:40], mb)
	copy(curve[40:72], cb)
	binary.LittleEndian.PutUint64(curve[72:80], 150_000_000_000)
	binary.LittleEndian.PutUint64(curve[80:88], 1_000_000_000_000_000)
	binary.LittleEndian.PutUint64(curve[88:96], 50_000_000_000)
	binary.LittleEndian.PutUint64(curve[96:104], 200_000_000_000_000)
	binary.LittleEndian.PutUint64(curve[104:112], 0)
	curve[112] = 0
	binary.LittleEndian.PutUint64(curve[113:121], 0)
	binary.LittleEndian.PutUint64(curve[125:133], 200_000_000_000)
	rpc.accounts[client.BondingCurvePDA(client.DevnetProgramID, mint)] = client.AccountInfo{Exists: true, Data: curve}
	rpc.accounts[client.GlobalConfigPDA(client.DevnetProgramID)] = globalConfigRPC(t)
	return rpc
}

func boardClientFor(t *testing.T, mint, seed string, rpc *boardFakeRPC) *client.TorchClient {
	t.Helper()
	raw := make([]byte, 64)
	seedBytes := make([]byte, 32)
	copy(seedBytes, seed)
	copy(raw[:32], seedBytes)
	pub, err := sol.PublicFromSeed(seedBytes)
	if err != nil {
		t.Fatal(err)
	}
	copy(raw[32:], pub)
	kp, err := sol.KeypairFromSecret(sol.Encode(raw))
	if err != nil {
		t.Fatal(err)
	}
	id, err := idl.LoadIDL()
	if err != nil {
		t.Fatal(err)
	}
	return &client.TorchClient{
		Config: client.Config{
			RPC: "http://127.0.0.1:1", ProgramID: client.DevnetProgramID,
			VaultCreator: "So11111111111111111111111111111111111111112", AgentKey: kp, AllowWrite: true,
		},
		IDL: id, API: &nilAPI{}, RPC: rpc,
	}
}

func openBoardStoreWith(t *testing.T, tc *client.TorchClient) (store.DB, *Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "board.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return db, &Store{Client: func() (*client.TorchClient, error) { return tc, nil }, DB: db}
}

func globalConfigRPC(t *testing.T) client.AccountInfo {
	t.Helper()
	data := make([]byte, 8+32+32+32+2+8+8+1)
	dev, _ := sol.Decode("So11111111111111111111111111111111111111112")
	copy(data[8+64:8+64+32], dev)
	data[8+96] = 50
	return client.AccountInfo{Exists: true, Data: data}
}

type nilAPI struct{}

func (nilAPI) Markets(context.Context, url.Values) ([]client.MarketRow, error) { return nil, nil }
func (nilAPI) Market(context.Context, string) (client.MarketDetail, error) {
	return client.MarketDetail{}, os.ErrNotExist
}
func (nilAPI) Messages(context.Context, url.Values) ([]client.MessageRow, error) { return nil, nil }
func (nilAPI) Trades(context.Context, url.Values) ([]client.TradeRow, error)     { return nil, nil }
func (nilAPI) Positions(context.Context, url.Values) ([]client.PositionRow, error) {
	return nil, nil
}
func (nilAPI) PositionEvents(context.Context, url.Values) ([]client.PositionEventRow, error) {
	return nil, nil
}
func (nilAPI) Migrations(context.Context, url.Values) ([]client.MigrationRow, error) {
	return nil, nil
}
func (nilAPI) Pnl(context.Context, string, string) (client.PnlSummary, error) {
	return client.PnlSummary{}, nil
}
func (nilAPI) Swaps(context.Context, url.Values) ([]client.SwapRow, error) { return nil, nil }
func openBoardStore(t *testing.T) (store.DB, *Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "board.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	tc, _ := boardTestClient(t, testMint)
	st := &Store{Client: func() (*client.TorchClient, error) { return tc, nil }, DB: db}
	return db, st
}

func seedRecordedLog(t *testing.T, db store.DB, project string, rows []client.MessageRow) {
	t.Helper()
	bound, tx, err := db.Tx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	md := domain.NewMessageDomain()
	for _, r := range rows {
		if _, err := md.InsertMessage(bound, domain.Message{
			Mint: r.Mint, Seq: int64(r.MessageID), Sender: r.Sender, Memo: r.MemoText,
			ActionKind: r.ActionKind, Slot: r.Slot, Signature: r.Signature,
			CreatedAt: r.CreatedAt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestSwarmDrainAgainstRecordedLog(t *testing.T) {
	db, st := openBoardStore(t)
	defer db.DB.Close()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "board_log.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []client.MessageRow
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	seedRecordedLog(t, db, testMint, rows)
	project := Project{Mint: testMint, Label: "torch test"}

	claim, err := st.Claim(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(claim, "claim 1") || !strings.HasPrefix(claim, "sigBoard") {
		t.Errorf("claim reply: %s", claim)
	}
	complete, err := st.Complete(context.Background(), project, "1", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(complete, "complete 1") {
		t.Errorf("complete reply: %s", complete)
	}
	accept, err := st.Accept(context.Background(), project, "1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(accept, "accept 1") {
		t.Errorf("accept reply: %s", accept)
	}
	board, err := st.BoardFromCache(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(board, "t1 done") || !strings.Contains(board, "t2 pending") {
		t.Errorf("final board:\n%s", board)
	}
	if !strings.Contains(board, "1/2 done") {
		t.Errorf("summary:\n%s", board)
	}
	if len(dbFakeSent(st)) != 3 {
		t.Errorf("sent txs: %d, want 3", len(dbFakeSent(st)))
	}
	for _, memo := range []string{"claim 1", "complete 1", "accept 1"} {
		if !strings.Contains(lastTxMemos(st), memo) {
			t.Errorf("memo %q missing from the sent txs", memo)
		}
	}
}

func TestSwarmReapExpiredClaim(t *testing.T) {
	db, st := openBoardStore(t)
	defer db.DB.Close()
	now := time.Now().UTC()
	old := now.Add(-Lease - time.Hour).Format(time.RFC3339)
	seedRecordedLog(t, db, testMint, []client.MessageRow{
		{Mint: testMint, MessageID: 1, Sender: "11111111111111111111111111111111111111111",
			MemoText: "task 3: Reap me", Slot: 1, Signature: "sigT3", CreatedAt: old},
		{Mint: testMint, MessageID: 2, Sender: "22222222222222222222222222222222222222222",
			MemoText: "claim 3", Slot: 2, Signature: "sigC3", CreatedAt: old},
	})
	project := Project{Mint: testMint, Label: "torch test"}
	reap, err := st.Reap(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reap, "1 expired claim") {
		t.Errorf("reap reply: %s", reap)
	}
	claim, err := st.Claim(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(claim, "claim 3") {
		t.Errorf("re-claim reply: %s", claim)
	}
	if !strings.Contains(claim, "t3 active") {
		t.Errorf("re-claim did not land as active (the dead claim must expire first):\n%s", claim)
	}
}

func dbFakeSent(st *Store) []string {
	tc, _ := st.client()
	rpc := tc.RPC.(*boardFakeRPC)
	return rpc.sent
}

func lastTxMemos(st *Store) string {
	tc, _ := st.client()
	rpc := tc.RPC.(*boardFakeRPC)
	var out []string
	for _, tx := range rpc.sent {
		raw, err := sol.Decode(tx)
		if err != nil {
			continue
		}
		for _, i := range walkIxs(raw) {
			out = append(out, string(i))
		}
	}
	return strings.Join(out, "\n")
}

func walkIxs(raw []byte) [][]byte {
	b := raw
	if len(b) < 67 || b[0] != 0x01 || b[65] != 0x80 {
		return nil
	}
	b = b[66:]
	if len(b) == 0 {
		return nil
	}
	b = b[:len(b)-1]
	if len(b) < 3 {
		return nil
	}
	b = b[3:]
	n, b, ok := compact(b)
	if !ok || len(b) < n*32+32 {
		return nil
	}
	b = b[n*32+32:]
	count, b, ok := compact(b)
	if !ok {
		return nil
	}
	var out [][]byte
	for i := 0; i < count; i++ {
		if len(b) < 2 {
			return out
		}
		b = b[1:]
		nAccts, rest, ok := compact(b)
		if !ok {
			return out
		}
		b = rest
		if len(b) < nAccts {
			return out
		}
		b = b[nAccts:]
		dataLen, rest, ok := compact(b)
		if !ok {
			return out
		}
		b = rest
		if len(b) < dataLen {
			return out
		}
		out = append(out, b[:dataLen])
		b = b[dataLen:]
	}
	return out
}

func compact(b []byte) (int, []byte, bool) {
	if len(b) == 0 {
		return 0, nil, false
	}
	v := int(b[0])
	if v&0x80 != 0 {
		if len(b) < 2 {
			return 0, nil, false
		}
		return (v & 0x7f) | int(b[1])<<7, b[2:], true
	}
	return v, b[1:], true
}

func TestTwoClientsFoldContestedVerdictAgree(t *testing.T) {
	ctx := context.Background()
	rpc := newBoardFakeRPC(t, testMint)
	tcA := boardClientFor(t, testMint, "orbit-board-client-A-seed-0000000", rpc)
	tcB := boardClientFor(t, testMint, "orbit-board-client-B-seed-0000000", rpc)
	dbA, stA := openBoardStoreWith(t, tcA)
	dbB, stB := openBoardStoreWith(t, tcB)
	defer dbA.DB.Close()
	defer dbB.DB.Close()
	base := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	arch, worker := tcA.AgentPublic(), tcB.AgentPublic()
	baseLog := []client.MessageRow{
		{Mint: testMint, MessageID: 1, Sender: arch, MemoText: "task 1: Contested verdict", Slot: 1, Signature: "sigT1", CreatedAt: base},
		{Mint: testMint, MessageID: 2, Sender: worker, MemoText: "claim 1", Slot: 2, Signature: "sigC1", CreatedAt: base},
		{Mint: testMint, MessageID: 3, Sender: worker, MemoText: "complete 1", Slot: 3, Signature: "sigP1", CreatedAt: base},
	}
	seedRecordedLog(t, dbA, testMint, baseLog)
	seedRecordedLog(t, dbB, testMint, baseLog)
	project := Project{Mint: testMint, Label: "torch test"}

	// Both clients write a verdict on the same review task from the same
	// pre-write view (A's memo not yet visible to B): the log ends up with
	// both, and both clients fold the same log without a (mint, seq) wedge.
	if _, err := stA.Accept(ctx, project, "1"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	rpc.visible = 0
	stB.WaitBudget = 100 * time.Millisecond
	rejectReply, err := stB.Reject(ctx, project, "1", "No proof")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if !strings.Contains(rejectReply, "pending: not yet indexed") {
		t.Errorf("a memo the cache has not seen yet must reply pending:\n%s", rejectReply)
	}
	rpc.visible = -1
	boardA, err := stA.Board(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	boardB, err := stB.Board(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if boardA != boardB {
		t.Errorf("clients disagree on the contested verdict:\n--- A ---\n%s\n--- B ---\n%s", boardA, boardB)
	}
	if len(rpc.sent) != 2 {
		t.Errorf("sent txs: %d, want 2 (both verdicts land)", len(rpc.sent))
	}
	if !strings.Contains(boardA, "t1 done") {
		t.Errorf("verdict board missing the done task:\n%s", boardA)
	}
}

func TestActStaleTaskGetsFreshID(t *testing.T) {
	ctx := context.Background()
	rpc := newBoardFakeRPC(t, testMint)
	tc := boardClientFor(t, testMint, "orbit-board-swarm-seed-00000000", rpc)
	db, st := openBoardStoreWith(t, tc)
	defer db.DB.Close()
	base := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	seedRecordedLog(t, db, testMint, []client.MessageRow{
		{Mint: testMint, MessageID: 1, Sender: "walletX", MemoText: "task 6: The real task 6", Slot: 1, Signature: "sigT6", CreatedAt: base},
	})
	project := Project{Mint: testMint, Label: "torch test"}
	// The caller minted id 6 from a cache that predates the sync above.
	reply, err := st.Act(ctx, project, Shape{Verb: "task", ID: 6, Text: "Stale cache task"})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !strings.Contains(reply, "assigned id 7") {
		t.Errorf("the reply does not name the assigned id:\n%s", reply)
	}
	board, err := st.BoardFromCache(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(board, "t6 pending") || !strings.Contains(board, "The real task 6") {
		t.Errorf("the real task 6 is missing:\n%s", board)
	}
	if !strings.Contains(board, "t7 pending") || !strings.Contains(board, "Stale cache task") {
		t.Errorf("the stale task did not get a fresh id:\n%s", board)
	}
}

func TestActTaskThenClaimWithLagSucceeds(t *testing.T) {
	ctx := context.Background()
	rpc := newBoardFakeRPC(t, testMint)
	rpc.lag = 2 // a sent memo becomes visible after two more scans
	tc := boardClientFor(t, testMint, "orbit-board-swarm-seed-00000000", rpc)
	db, st := openBoardStoreWith(t, tc)
	defer db.DB.Close()
	project := Project{Mint: testMint, Label: "torch test"}

	reply, err := st.Act(ctx, project, Shape{Verb: "task", ID: 0, Text: "Lag task"})
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	if !strings.Contains(reply, "t1 pending") {
		t.Errorf("task reply must show the act once the memo lands:\n%s", reply)
	}
	reply, err = st.Claim(ctx, project)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !strings.Contains(reply, "t1 active") {
		t.Errorf("claim reply must show the claim:\n%s", reply)
	}
	if len(rpc.sent) != 2 {
		t.Errorf("sent txs: %d, want 2 (one task + one claim, no retry double-spend)", len(rpc.sent))
	}
	taskMemos := 0
	for _, m := range rpc.msgs {
		for _, ix := range rpc.txs[m.Signature].Ixs {
			if ix.ProgramID == client.MemoProgram && strings.HasPrefix(string(ix.Data), "task ") {
				taskMemos++
			}
		}
	}
	if taskMemos != 1 {
		t.Errorf("task memos on chain: %d, want 1 (the lagged memo must not be re-written)", taskMemos)
	}
}

func TestActRefusesForeignAcceptBeforeSpend(t *testing.T) {
	ctx := context.Background()
	rpc := newBoardFakeRPC(t, testMint)
	tc := boardClientFor(t, testMint, "orbit-board-swarm-seed-00000000", rpc)
	db, st := openBoardStoreWith(t, tc)
	defer db.DB.Close()
	base := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	seedRecordedLog(t, db, testMint, []client.MessageRow{
		{Mint: testMint, MessageID: 1, Sender: "walletX", MemoText: "task 1: Funded by someone else", Slot: 1, Signature: "sigT1", CreatedAt: base},
		{Mint: testMint, MessageID: 2, Sender: "walletY", MemoText: "claim 1", Slot: 2, Signature: "sigC1", CreatedAt: base},
		{Mint: testMint, MessageID: 3, Sender: "walletY", MemoText: "complete 1", Slot: 3, Signature: "sigP1", CreatedAt: base},
	})
	project := Project{Mint: testMint, Label: "torch test"}
	if _, err := st.Accept(ctx, project, "1"); err == nil || !strings.Contains(err.Error(), "refuse") {
		t.Fatalf("non-funder accept: %v", err)
	}
	if len(rpc.sent) != 0 {
		t.Errorf("sent txs: %d, want 0 (a refused act must not spend)", len(rpc.sent))
	}
}

func TestOpenMigratesV1Cache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite")
	old := []string{
		`CREATE TABLE IF NOT EXISTS tasks (
			project TEXT NOT NULL,
			id TEXT NOT NULL,
			title TEXT NOT NULL,
			brief TEXT NOT NULL,
			status TEXT NOT NULL,
			owner TEXT NOT NULL,
			claimed_at TEXT NOT NULL,
			completed_at TEXT NOT NULL,
			accepted_by TEXT NOT NULL,
			rejected_by TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (project, id)
		)`,
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT)`,
	}
	db, _, _, err := store.Open(path, old, 1)
	if err != nil {
		t.Fatal(err)
	}
	db.DB.Close()
	open, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer open.DB.Close()
	if _, err := open.DB.Exec(`INSERT INTO tasks (project, id, title, brief, status, funder, owner, claimed_at, completed_at, accepted_by, rejected_by, created_at, updated_at)
		VALUES ('p', '1', 't', '', 'pending', 'f', '', '', '', '', '', '', '')`); err != nil {
		t.Fatalf("migrated cache rejects the funder column: %v", err)
	}
}

func TestBoardTaskRowsOrderNumerically(t *testing.T) {
	ctx := context.Background()
	rpc := newBoardFakeRPC(t, testMint)
	tc := boardClientFor(t, testMint, "orbit-board-swarm-seed-00000000", rpc)
	db, st := openBoardStoreWith(t, tc)
	defer db.DB.Close()
	base := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	seedRecordedLog(t, db, testMint, []client.MessageRow{
		{Mint: testMint, MessageID: 1, Sender: "walletA", MemoText: "task 1: First", Slot: 1, Signature: "sigT1", CreatedAt: base},
		{Mint: testMint, MessageID: 2, Sender: "walletA", MemoText: "task 10: Tenth", Slot: 2, Signature: "sigT10", CreatedAt: base},
		{Mint: testMint, MessageID: 3, Sender: "walletA", MemoText: "task 2: Second", Slot: 3, Signature: "sigT2", CreatedAt: base},
	})
	board, err := st.Board(ctx, Project{Mint: testMint, Label: "torch test"})
	if err != nil {
		t.Fatal(err)
	}
	pos := func(sub string) int {
		for i, line := range strings.Split(board, "\n") {
			if strings.Contains(line, sub) {
				return i
			}
		}
		return -1
	}
	p1, p2, p10 := pos("t1 pending First"), pos("t2 pending Second"), pos("t10 pending Tenth")
	if p1 < 0 || p2 < 0 || p10 < 0 {
		t.Fatalf("the board lost a task:\n%s", board)
	}
	if !(p1 < p2 && p2 < p10) {
		t.Errorf("task rows must order numerically (t1, t2, t10), got t1@%d t2@%d t10@%d:\n%s", p1, p2, p10, board)
	}
}
