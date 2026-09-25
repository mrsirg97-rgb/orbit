package project

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/idl"
	"github.com/mrsirg97-rgb/orbit/sol"
)

func TestGoalMemoRoundTrip(t *testing.T) {
	memo, err := GoalMemo("Research whether sentiment predicts price.")
	if err != nil {
		t.Fatal(err)
	}
	if memo != "[architect] goal: Research whether sentiment predicts price." {
		t.Errorf("memo: %q", memo)
	}
	goal, ok := GoalFrom(memo)
	if !ok || goal != "Research whether sentiment predicts price." {
		t.Errorf("goal: %q %v", goal, ok)
	}
	if _, ok := GoalFrom("[worker] claim: compress the transcript"); ok {
		t.Error("worker memo parsed as a goal")
	}
	if _, ok := GoalFrom("research notes"); ok {
		t.Error("plain memo parsed as a goal")
	}
}

func TestGoalMemoRejects(t *testing.T) {
	if _, err := GoalMemo(""); err == nil {
		t.Error("empty goal accepted")
	}
	if _, err := GoalMemo("line one\nline two"); err == nil {
		t.Error("multi-paragraph goal accepted")
	}
	long := strings.Repeat("x", MaxGoalLen+1)
	if _, err := GoalMemo(long); err == nil {
		t.Error("over-long goal accepted")
	}
}

func TestSymbolFor(t *testing.T) {
	cases := map[string]string{
		"Context Compaction":    "CONTEX",
		"Sentiment Alpha":       "SENTIM",
		"Treasury Accumulation": "TREASU",
		"42 Bots":               "42BOTS",
		"!!!!":                  "PROJ",
		"":                      "PROJ",
	}
	for in, want := range cases {
		if got := SymbolFor(in); got != want {
			t.Errorf("SymbolFor(%q) = %q, want %q", in, got, want)
		}
	}
}

type fixtureRPC struct {
	accounts map[string]client.AccountInfo
}

func (f *fixtureRPC) GetLatestBlockhash(ctx context.Context) (string, error) {
	return "11111111111111111111111111111111", nil
}
func (f *fixtureRPC) SendTransaction(ctx context.Context, signed []byte) (string, error) {
	return "sig", nil
}
func (f *fixtureRPC) GetAccountInfo(ctx context.Context, pubkey string) (client.AccountInfo, error) {
	return f.accounts[pubkey], nil
}
func (f *fixtureRPC) GetTokenAccountsByOwner(ctx context.Context, owner, programID string) ([]client.TokenAccount, error) {
	return nil, nil
}
func (f *fixtureRPC) GetBalance(ctx context.Context, pubkey string) (uint64, error) {
	return 0, nil
}
func (f *fixtureRPC) GetSignatureStatus(ctx context.Context, signature string) (client.SignatureStatus, error) {
	return client.SignatureStatus{Exists: true, Confirmed: true}, nil
}
func (f *fixtureRPC) RequestAirdrop(ctx context.Context, pubkey string, lamports uint64) (string, error) {
	return "airdrop", nil
}
func (f *fixtureRPC) GetSignaturesForAddress(ctx context.Context, address string, limit int) ([]client.SignatureInfo, error) {
	return nil, nil
}
func (f *fixtureRPC) GetTransaction(ctx context.Context, signature string) (*client.Transaction, error) {
	return nil, nil
}

func serveProjectsFixture(t *testing.T) (client.API, client.RPC) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Markets  []client.MarketRow  `json:"markets"`
		Messages []client.MessageRow `json:"messages"`
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/markets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(fixture.Markets)
	})
	mux.HandleFunc("/api/messages", func(w http.ResponseWriter, r *http.Request) {
		mint := r.URL.Query().Get("mint")
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
		var out []client.MessageRow
		for _, m := range fixture.Messages {
			if mint == "" || m.Mint == mint {
				out = append(out, m)
			}
			if len(out) >= limit {
				break
			}
		}
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	treasuries := map[string]uint64{
		"EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG": 1_000_000_000,
		"7kuT1dfMhUysWcLEV1eYk8ir7RTjszHmsUdrrPQNThcv": 2_000_000_000,
		"EWo1KkENqJgXTfLz6tGRqfu8XJVsELwmkHHUgPtHB1sc": 500_000_000,
	}
	accounts := map[string]client.AccountInfo{}
	for mint, sol := range treasuries {
		accounts[client.TreasurySolVaultPDA(client.DevnetProgramID, mint)] = client.AccountInfo{
			Lamports: client.RentExemptZeroData + sol, Exists: true,
		}
	}
	return client.NewAPI(srv.URL), &fixtureRPC{accounts: accounts}
}

func TestListAgainstFixture(t *testing.T) {
	api, rpc := serveProjectsFixture(t)
	rows, err := List(context.Background(), api, rpc, client.DevnetProgramID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[0].Mint != "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG" ||
		rows[0].Name != "Context Compaction" || rows[0].Symbol != "CONTEX" ||
		rows[0].Status != "BONDING" {
		t.Errorf("row 0: %+v", rows[0])
	}
	if rows[0].TreasurySOL != 1_000_000_000 {
		t.Errorf("row 0 treasury: %d", rows[0].TreasurySOL)
	}
	wantGoal := "Research how much conversation history a small model needs to keep a coherent trading stance."
	if rows[0].Goal != wantGoal {
		t.Errorf("row 0 goal: %q", rows[0].Goal)
	}
	if rows[1].Goal != "Research whether the message-board sentiment score predicts short-horizon price moves on devnet markets." {
		t.Errorf("row 1 goal: %q", rows[1].Goal)
	}
	if rows[1].TreasurySOL != 2_000_000_000 {
		t.Errorf("row 1 treasury: %d", rows[1].TreasurySOL)
	}
	if rows[2].Goal != "" {
		t.Errorf("row 2 goal: %q, want empty", rows[2].Goal)
	}
	if rows[2].TreasurySOL != 500_000_000 {
		t.Errorf("row 2 treasury: %d", rows[2].TreasurySOL)
	}
}

func TestListPrefersFirstGoalMemo(t *testing.T) {
	api, rpc := serveProjectsFixture(t)
	rows, err := List(context.Background(), api, rpc, client.DevnetProgramID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rows[0].Goal, "Research how much") {
		t.Errorf("goal not the first [architect] memo: %q", rows[0].Goal)
	}
}

type createRPC struct {
	fixtureRPC
	sent [][]byte
}

func (f *createRPC) GetLatestBlockhash(ctx context.Context) (string, error) {
	return "11111111111111111111111111111111", nil
}

func (f *createRPC) SendTransaction(ctx context.Context, signed []byte) (string, error) {
	f.sent = append(f.sent, append([]byte{}, signed...))
	return "sig" + strconv.Itoa(len(f.sent)), nil
}

type createAPI struct{}

func (createAPI) Markets(ctx context.Context, q url.Values) ([]client.MarketRow, error) {
	return nil, nil
}
func (createAPI) Market(ctx context.Context, mint string) (client.MarketDetail, error) {
	return client.MarketDetail{Market: client.MarketRow{
		Mint: mint, Name: "T", Symbol: "T", Status: client.StatusBonding,
		VirtualSol: 30_000_000_000, VirtualToken: 1_000_000_000_000_000,
		RealSol: 0, SolTarget: 200_000_000_000,
	}}, nil
}
func (createAPI) Messages(ctx context.Context, q url.Values) ([]client.MessageRow, error) {
	return nil, nil
}
func (createAPI) Trades(ctx context.Context, q url.Values) ([]client.TradeRow, error) {
	return nil, nil
}
func (createAPI) Positions(ctx context.Context, q url.Values) ([]client.PositionRow, error) {
	return nil, nil
}
func (createAPI) PositionEvents(ctx context.Context, q url.Values) ([]client.PositionEventRow, error) {
	return nil, nil
}
func (createAPI) Migrations(ctx context.Context, q url.Values) ([]client.MigrationRow, error) {
	return nil, nil
}
func (createAPI) Pnl(ctx context.Context, wallet, vault string) (client.PnlSummary, error) {
	return client.PnlSummary{}, nil
}
func (createAPI) Swaps(ctx context.Context, q url.Values) ([]client.SwapRow, error) {
	return nil, nil
}

func TestCreateSendsCreateThenBuyWithGoalMemo(t *testing.T) {
	operator := testKeypair("project-operator")
	rpc := &createRPC{fixtureRPC: fixtureRPC{accounts: map[string]client.AccountInfo{}}}
	program := client.DevnetProgramID
	devWallet := testKeypair("project-devwallet").PublicBase58()
	global := client.GlobalConfigPDA(program)
	rpc.accounts[global] = client.AccountInfo{
		Exists: true, Data: encodeGlobalConfig(t, devWallet),
	}
	rpc.accounts[client.VaultSolPDA(program, operator.PublicBase58())] = client.AccountInfo{
		Lamports: client.RentExemptZeroData + 10_000_000_000, Exists: true,
	}
	tc := &client.TorchClient{
		Config: client.Config{
			ProgramID: program, VaultCreator: operator.PublicBase58(), AllowWrite: true,
		},
		API: createAPI{}, RPC: rpc,
	}
	id, err := idl.LoadIDL()
	if err != nil {
		t.Fatal(err)
	}
	tc.IDL = id
	res, err := Create(context.Background(), tc, operator, "Context Compaction",
		"Research whether sentiment predicts price.", 1_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(rpc.sent) != 2 {
		t.Fatalf("sent %d txs, want 2", len(rpc.sent))
	}
	if res.CreateSignature != "sig1" || res.BuySignature != "sig2" {
		t.Errorf("signatures: %s %s", res.CreateSignature, res.BuySignature)
	}
	if res.Symbol != "CONTEX" {
		t.Errorf("symbol: %s", res.Symbol)
	}
	if res.Goal != "[architect] goal: Research whether sentiment predicts price." {
		t.Errorf("goal memo: %q", res.Goal)
	}
	create := rpc.sent[0]
	if !bytes.Contains(create, mustB58(t, operator.PublicBase58())) {
		t.Error("create tx lacks the operator signer")
	}
	if !bytes.Contains(create, mustB58(t, res.Mint)) {
		t.Error("create tx lacks the mint signer")
	}
	disc, err := tc.IDL.Discriminator("create_token")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(create, disc) {
		t.Error("create tx lacks the create_token discriminator")
	}
	buy := rpc.sent[1]
	if !bytes.Contains(buy, []byte("[architect] goal: Research whether sentiment predicts price.")) {
		t.Error("buy tx lacks the goal memo")
	}
	buyDisc, err := tc.IDL.Discriminator("buy_via_vault")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buy, buyDisc) {
		t.Error("buy tx lacks the buy_via_vault discriminator")
	}
}

func encodeGlobalConfig(t *testing.T, devWallet string) []byte {
	t.Helper()
	out := make([]byte, 8+32+32+32+2+8+8+1)
	copy(out[8:40], mustB58(t, "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy"))
	copy(out[40:72], mustB58(t, devWallet))
	copy(out[72:104], mustB58(t, devWallet))
	return out
}

func testKeypair(seed string) sol.Keypair {
	b := make([]byte, 32)
	copy(b, seed)
	derived, _ := sol.PublicFromSeed(b)
	kp, err := sol.KeypairFromSecret(sol.Encode(append(append([]byte{}, b...), derived...)))
	if err != nil {
		panic(err)
	}
	return kp
}

func mustB58(t *testing.T, s string) []byte {
	t.Helper()
	b, err := sol.Decode(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
