package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var _ = strings.TrimSpace

// serveFixtures hosts the recorded testdata under the indexer routes.
func serveFixtures(t *testing.T) (*httptest.Server, API) {
	t.Helper()
	dir := filepath.Join("..", "testdata")
	mux := http.NewServeMux()
	file := func(name string) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("content-type", "application/json")
			w.Write(b)
		}
	}
	mux.HandleFunc("/api/markets", file("markets.json"))
	mux.HandleFunc("/api/markets/EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG", file("market_detail.json"))
	mux.HandleFunc("/api/messages", file("messages.json"))
	mux.HandleFunc("/api/positions", file("positions.json"))
	mux.HandleFunc("/api/user-pnl/So11111111111111111111111111111111111111112", file("user-pnl.json"))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, NewAPI(srv.URL)
}

func TestMarketsList(t *testing.T) {
	_, api := serveFixtures(t)
	markets, err := api.Markets(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(markets) != 2 {
		t.Fatalf("got %d markets, want 2", len(markets))
	}
	m := markets[0]
	if m.Mint != "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG" {
		t.Errorf("mint: %s", m.Mint)
	}
	if m.Name != "Torch Test" || m.Symbol != "TST" {
		t.Errorf("name/symbol: %s %s", m.Name, m.Symbol)
	}
	if m.Status != StatusBonding {
		t.Errorf("status: %s", m.Status)
	}
	if m.SolTarget != 200_000_000_000 || m.VirtualSol != 150_000_000_000 {
		t.Errorf("sol target/virtual: %d %d", m.SolTarget, m.VirtualSol)
	}
	if m.VirtualToken != 1_000_000_000_000_000 {
		t.Errorf("virtual token: %d", m.VirtualToken)
	}
	if m.IsCommunityToken {
		t.Error("is_community_token should be false")
	}
	if m.DeepPoolPubkey != nil {
		t.Error("bonding market should have no deep pool")
	}
	m2 := markets[1]
	if m2.Status != StatusMigrated || m2.DeepPoolPubkey == nil {
		t.Errorf("migrated market: %s %v", m2.Status, m2.DeepPoolPubkey)
	}
	if *m2.DeepPoolPubkey != "H5uVsoK2KydBthZYgMhh5Z2mJBRNfr7ZCZpLiVb9zjCf" {
		t.Errorf("deep pool: %v", m2.DeepPoolPubkey)
	}
}

func TestMarketDetail(t *testing.T) {
	_, api := serveFixtures(t)
	detail, err := api.Market(context.Background(), "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Market.Mint != "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG" {
		t.Errorf("mint: %s", detail.Market.Mint)
	}
	if detail.Reserves != nil {
		t.Error("bonding market reserves should be null")
	}
}

func TestMessages(t *testing.T) {
	_, api := serveFixtures(t)
	msgs, err := api.Messages(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].MemoText != "backed up. strong join" || msgs[0].ActionKind == nil || *msgs[0].ActionKind != "buy" {
		t.Errorf("message 0: %+v", msgs[0])
	}
	if msgs[1].Sender != "So11111111111111111111111111111111111111112" {
		t.Errorf("sender: %s", msgs[1].Sender)
	}
}

func TestPositions(t *testing.T) {
	_, api := serveFixtures(t)
	positions, err := api.Positions(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 1 {
		t.Fatalf("got %d positions", len(positions))
	}
	p := positions[0]
	if p.Side != SideLong || p.Health != HealthHealthy || !p.IsActive || !p.OwnerIsVault {
		t.Errorf("position: %+v", p)
	}
}

func TestUserPnl(t *testing.T) {
	_, api := serveFixtures(t)
	pnl, err := api.Pnl(context.Background(), "So11111111111111111111111111111111111111112", "")
	if err != nil {
		t.Fatal(err)
	}
	if pnl.TotalRealizedPnl != 2_500_000 {
		t.Errorf("total realized: %d", pnl.TotalRealizedPnl)
	}
	if len(pnl.ByMint) != 1 {
		t.Fatalf("by_mint: %d", len(pnl.ByMint))
	}
	m := pnl.ByMint[0]
	if m.Mint != "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG" || m.TokensRemaining != 123_456_789 {
		t.Errorf("by_mint: %+v", m)
	}
	if m.CostBasisRemaining != 9_000_000 {
		t.Errorf("cost basis: %d", m.CostBasisRemaining)
	}
}

func TestLimitClamp(t *testing.T) {
	if n := clampLimit(0); n != 50 {
		t.Errorf("default limit: %d", n)
	}
	if n := clampLimit(10000); n != 500 {
		t.Errorf("max limit: %d", n)
	}
	if n := clampLimit(7); n != 7 {
		t.Errorf("limit: %d", n)
	}
}

func TestFramesDecode(t *testing.T) {
	row := MessageRow{}
	if err := DecodeFrame(Frame{Kind: "message", Raw: []byte(`{"message_id":1}`)}, &row); err != nil {
		t.Fatal(err)
	}
	if row.MessageID != 1 {
		t.Errorf("message id: %d", row.MessageID)
	}
}

func TestEventsResyncSentinel(t *testing.T) {
	// The server's resync frame surfaces as ErrResync at the consumer loop.
	if !strings.Contains(ErrResync.Error(), "re-read state") {
		t.Errorf("sentinel text: %s", ErrResync)
	}
}
