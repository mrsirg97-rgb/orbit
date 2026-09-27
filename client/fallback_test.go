package client

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"
)

func TestIndexerUnreachable(t *testing.T) {
	status := func(code int) error {
		return &HTTPStatusError{Path: "/api/messages", Code: code, Body: "x"}
	}
	if !IndexerUnreachable(status(500)) {
		t.Error("a 5xx is unreachable")
	}
	if !IndexerUnreachable(status(503)) {
		t.Error("a 503 is unreachable")
	}
	if IndexerUnreachable(status(404)) {
		t.Error("a 404 is not unreachable")
	}
	if IndexerUnreachable(status(400)) {
		t.Error("a 400 is not unreachable")
	}
	connect := &url.Error{Op: "Get", URL: "http://127.0.0.1:1", Err: errors.New("dial tcp: connection refused")}
	if !IndexerUnreachable(connect) {
		t.Error("a connect error is unreachable")
	}
	dns := &url.Error{Op: "Get", URL: "http://x", Err: &net.DNSError{Err: "no such host", Name: "x"}}
	if !IndexerUnreachable(dns) {
		t.Error("a dns error is unreachable")
	}
	if !IndexerUnreachable(&url.Error{Op: "Get", URL: "http://x", Err: context.DeadlineExceeded}) {
		t.Error("a timeout is unreachable")
	}
	if !IndexerUnreachable(context.DeadlineExceeded) {
		t.Error("a bare deadline is unreachable")
	}
	if !IndexerUnreachable(&net.OpError{Op: "dial", Err: errors.New("refused")}) {
		t.Error("a bare transport error is unreachable")
	}
	if IndexerUnreachable(errors.New("api /api/messages: decode: unexpected EOF")) {
		t.Error("a decode error is not unreachable")
	}
	if IndexerUnreachable(context.Canceled) {
		t.Error("a canceled request is not unreachable")
	}
	if IndexerUnreachable(&url.Error{Op: "Get", URL: "http://x", Err: context.Canceled}) {
		t.Error("a wrapped cancellation is not unreachable")
	}
}

type countAPI struct {
	messages int
}

func (a *countAPI) Markets(context.Context, url.Values) ([]MarketRow, error) { return nil, nil }
func (a *countAPI) Market(context.Context, string) (MarketDetail, error)     { return MarketDetail{}, nil }
func (a *countAPI) Messages(context.Context, url.Values) ([]MessageRow, error) {
	a.messages++
	return nil, nil
}
func (a *countAPI) Trades(context.Context, url.Values) ([]TradeRow, error)       { return nil, nil }
func (a *countAPI) Positions(context.Context, url.Values) ([]PositionRow, error) { return nil, nil }
func (a *countAPI) PositionEvents(context.Context, url.Values) ([]PositionEventRow, error) {
	return nil, nil
}
func (a *countAPI) Migrations(context.Context, url.Values) ([]MigrationRow, error) { return nil, nil }
func (a *countAPI) Pnl(context.Context, string, string) (PnlSummary, error) {
	return PnlSummary{}, nil
}
func (a *countAPI) Swaps(context.Context, url.Values) ([]SwapRow, error) { return nil, nil }

type beforeAPI struct {
	countAPI
	q url.Values
}

func (a *beforeAPI) Messages(ctx context.Context, q url.Values) ([]MessageRow, error) {
	a.countAPI.messages++
	a.q = q
	return nil, nil
}

func TestMessagesCarriesBeforeForTheIndexer(t *testing.T) {
	api := &beforeAPI{}
	rpc := serveRecordedTxs(t)
	tc := &TorchClient{Config: Config{RPC: rpc.base, ProgramID: DevnetProgramID}, API: api, RPC: rpc}
	before := "2026-01-01T12:00:00Z"
	if _, err := tc.MessagesPage(context.Background(), "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG", 10, SourceIndexer, before); err != nil {
		t.Fatal(err)
	}
	if api.q.Get("before") != before {
		t.Errorf("indexer query before = %q, want %q", api.q.Get("before"), before)
	}
}

func TestMessagesRoutesBySource(t *testing.T) {
	api := &countAPI{}
	rpc := serveRecordedTxs(t)
	tc := &TorchClient{Config: Config{RPC: rpc.base, ProgramID: DevnetProgramID}, API: api, RPC: rpc}
	ctx := context.Background()

	if _, err := tc.MessagesPage(ctx, "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG", 10, SourceIndexer, ""); err != nil {
		t.Fatal(err)
	}
	if api.messages != 1 {
		t.Errorf("indexer source must route to the API: %d calls", api.messages)
	}

	page, err := tc.MessagesPage(ctx, "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG", 10, SourceScan, "")
	if err != nil {
		t.Fatal(err)
	}
	if api.messages != 1 {
		t.Errorf("scan source must not hit the API: %d calls", api.messages)
	}
	if len(page.Rows) != 2 {
		t.Errorf("scan rows: %d, want 2", len(page.Rows))
	}
	if len(page.Signatures) != 3 {
		t.Errorf("scanned signatures: %d, want 3 (the failed tx still counts as scanned)", len(page.Signatures))
	}
	if page.OldestSignature != "sigBoard3" {
		t.Errorf("oldest scanned signature: %q, want sigBoard3", page.OldestSignature)
	}

	if _, err := tc.MessagesPage(ctx, "x", 10, Source("other"), ""); err == nil {
		t.Error("an unknown source must refuse")
	}
}
