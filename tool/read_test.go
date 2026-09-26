package tool

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/client"
	solpkg "github.com/mrsirg97-rgb/orbit/sol"
)

type resolveMintAPI struct {
	marketsCalls int
	marketCalls  int
}

func (a *resolveMintAPI) Messages(context.Context, url.Values) ([]client.MessageRow, error) {
	return nil, nil
}
func (a *resolveMintAPI) Trades(context.Context, url.Values) ([]client.TradeRow, error) {
	return nil, nil
}
func (a *resolveMintAPI) Positions(context.Context, url.Values) ([]client.PositionRow, error) {
	return nil, nil
}
func (a *resolveMintAPI) PositionEvents(context.Context, url.Values) ([]client.PositionEventRow, error) {
	return nil, nil
}
func (a *resolveMintAPI) Migrations(context.Context, url.Values) ([]client.MigrationRow, error) {
	return nil, nil
}
func (a *resolveMintAPI) Pnl(context.Context, string, string) (client.PnlSummary, error) {
	return client.PnlSummary{}, nil
}
func (a *resolveMintAPI) Swaps(context.Context, url.Values) ([]client.SwapRow, error) {
	return nil, nil
}
func (a *resolveMintAPI) Markets(context.Context, url.Values) ([]client.MarketRow, error) {
	a.marketsCalls++
	return nil, nil
}
func (a *resolveMintAPI) Market(context.Context, string) (client.MarketDetail, error) {
	a.marketCalls++
	return client.MarketDetail{}, nil
}

type resolveMintRPC struct {
	calls int
}

func (r *resolveMintRPC) GetAccountInfo(context.Context, string) (client.AccountInfo, error) {
	r.calls++
	return client.AccountInfo{}, nil
}
func (r *resolveMintRPC) GetLatestBlockhash(context.Context) (string, error)      { return "", nil }
func (r *resolveMintRPC) SendTransaction(context.Context, []byte) (string, error) { return "", nil }
func (r *resolveMintRPC) GetTokenAccountsByOwner(context.Context, string, string) ([]client.TokenAccount, error) {
	return nil, nil
}
func (r *resolveMintRPC) GetBalance(context.Context, string) (uint64, error) { return 0, nil }
func (r *resolveMintRPC) GetSignatureStatus(context.Context, string) (client.SignatureStatus, error) {
	return client.SignatureStatus{}, nil
}
func (r *resolveMintRPC) RequestAirdrop(context.Context, string, uint64) (string, error) {
	return "", nil
}
func (r *resolveMintRPC) GetSignaturesForAddress(context.Context, string, int) ([]client.SignatureInfo, error) {
	return nil, nil
}
func (r *resolveMintRPC) GetTransaction(context.Context, string) (*client.Transaction, error) {
	return nil, nil
}

func TestResolveMintAcceptsOnlyDecodedPubkeys(t *testing.T) {
	ctx := context.Background()
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	api := &resolveMintAPI{}
	tc := &client.TorchClient{Config: client.Config{Indexer: "https://api.torchmarket.dev"}, API: api}

	got, err := resolveMint(ctx, tc, mint)
	if err != nil || got != mint {
		t.Fatalf("resolveMint(%s) = %q, %v", mint, got, err)
	}
	if api.marketCalls != 1 {
		t.Errorf("market calls: %d, want 1 (a decoded 32-byte mint goes to the mint path)", api.marketCalls)
	}

	invalid := strings.Repeat("!", 44)
	if _, err := resolveMint(ctx, tc, invalid); err == nil || !strings.Contains(err.Error(), "no project with FID") {
		t.Fatalf("a 44-char non-base58 string must not be treated as a mint: %v", err)
	}
	if api.marketCalls != 1 {
		t.Errorf("market calls: %d, want still 1 (the invalid string must not hit the mint path)", api.marketCalls)
	}
	if api.marketsCalls != 1 {
		t.Errorf("markets calls: %d, want 1 (the invalid string resolves by FID)", api.marketsCalls)
	}

	// The board's resolveMint refuses the same way, before any RPC.
	rpc := &resolveMintRPC{}
	tc2 := &client.TorchClient{Config: client.Config{Indexer: "https://api.torchmarket.dev"}, API: api, RPC: rpc}
	b := &Board{Client: func() (*client.TorchClient, error) { return tc2, nil }, Store: &board.Store{Client: func() (*client.TorchClient, error) { return tc2, nil }}}
	if _, err := b.resolveMint(ctx, invalid); err == nil || !strings.Contains(err.Error(), "no project with FID") {
		t.Fatalf("board resolveMint accepted a non-base58 44-char string: %v", err)
	}
	if rpc.calls != 0 {
		t.Errorf("rpc calls: %d, want 0 (the invalid string must not reach the chain)", rpc.calls)
	}

	if _, err := solpkg.Decode(mint); err != nil {
		t.Fatalf("the test mint must decode: %v", err)
	}
}

func TestCostBasisIsLamportsEverywhere(t *testing.T) {
	// SPEC_BRIEF: unrealized = holdings value - cost_basis_remaining
	// (lamports). The tools must use the same unit — /1e9 to SOL, not /1e6.
	pnl := &client.PnlSummary{ByMint: []client.PnlByMint{{
		Mint: "m", CostBasisRemaining: 9_000_000, RealizedPnl: 1_000_000,
	}}}
	got := pnlFor(pnl, "m", 0.1234)
	want := 0.1234 - 9_000_000/1e9 + 1_000_000/1e9
	if got != want {
		t.Errorf("pnlFor = %v, want %v (cost basis in lamports)", got, want)
	}
}
