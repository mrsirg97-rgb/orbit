package board

import (
	"context"
	"encoding/binary"
	"encoding/json"
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
	accounts map[string]client.AccountInfo
	sent     []string
	sig      int
}

func (f *boardFakeRPC) GetLatestBlockhash(context.Context) (string, error) {
	return "11111111111111111111111111111111", nil
}
func (f *boardFakeRPC) SendTransaction(ctx context.Context, signed []byte) (string, error) {
	f.sig++
	f.sent = append(f.sent, sol.EncodeTx(signed))
	return "sigBoard" + itoa(f.sig), nil
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
	return nil, nil
}
func (f *boardFakeRPC) GetTransaction(context.Context, string) (*client.Transaction, error) {
	return nil, nil
}

func boardTestClient(t *testing.T, mint string) (*client.TorchClient, *boardFakeRPC) {
	t.Helper()
	rpc := &boardFakeRPC{accounts: map[string]client.AccountInfo{}}
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
	seed := make([]byte, 32)
	copy(seed, "orbit-board-swarm-seed-00000000")
	raw := make([]byte, 64)
	copy(raw[:32], seed)
	pub, _ := sol.PublicFromSeed(seed)
	copy(raw[32:], pub)
	kp, err := sol.KeypairFromSecret(sol.Encode(raw))
	if err != nil {
		t.Fatal(err)
	}
	id, err := idl.LoadIDL()
	if err != nil {
		t.Fatal(err)
	}
	tc := &client.TorchClient{
		Config: client.Config{
			RPC: "http://127.0.0.1:1", ProgramID: client.DevnetProgramID,
			VaultCreator: creator, AgentKey: kp, AllowWrite: true,
		},
		IDL: id, API: &nilAPI{}, RPC: rpc,
	}
	return tc, rpc
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
	// The memos rode the txs: the recorded signatures carry the memo bytes.
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
