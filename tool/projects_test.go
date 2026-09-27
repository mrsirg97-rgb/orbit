package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/orbit/client"
)

const (
	projectsMintA = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	projectsMintB = "7kuT1dfMhUysWcLEV1eYk8ir7RTjszHmsUdrrPQNThcv"
)

type projectsIndexer struct {
	markets  []client.MarketRow
	messages map[string][]client.MessageRow
}

func (f *projectsIndexer) Markets(context.Context, url.Values) ([]client.MarketRow, error) {
	return f.markets, nil
}
func (f *projectsIndexer) Market(_ context.Context, mint string) (client.MarketDetail, error) {
	for _, m := range f.markets {
		if m.Mint == mint {
			return client.MarketDetail{Market: m}, nil
		}
	}
	return client.MarketDetail{}, fmt.Errorf("no market %s", mint)
}
func (f *projectsIndexer) Messages(_ context.Context, q url.Values) ([]client.MessageRow, error) {
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows := f.messages[q.Get("mint")]
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}
func (f *projectsIndexer) Trades(context.Context, url.Values) ([]client.TradeRow, error) {
	return nil, nil
}
func (f *projectsIndexer) Positions(context.Context, url.Values) ([]client.PositionRow, error) {
	return nil, nil
}
func (f *projectsIndexer) PositionEvents(context.Context, url.Values) ([]client.PositionEventRow, error) {
	return nil, nil
}
func (f *projectsIndexer) Migrations(context.Context, url.Values) ([]client.MigrationRow, error) {
	return nil, nil
}
func (f *projectsIndexer) Pnl(context.Context, string, string) (client.PnlSummary, error) {
	return client.PnlSummary{}, nil
}
func (f *projectsIndexer) Swaps(context.Context, url.Values) ([]client.SwapRow, error) {
	return nil, nil
}

type projectsRPC struct {
	accounts map[string]client.AccountInfo
}

func (f *projectsRPC) GetAccountInfo(_ context.Context, pubkey string) (client.AccountInfo, error) {
	return f.accounts[pubkey], nil
}
func (f *projectsRPC) GetLatestBlockhash(context.Context) (string, error)      { return "", nil }
func (f *projectsRPC) SendTransaction(context.Context, []byte) (string, error) { return "", nil }
func (f *projectsRPC) GetTokenAccountsByOwner(context.Context, string, string) ([]client.TokenAccount, error) {
	return nil, nil
}
func (f *projectsRPC) GetBalance(context.Context, string) (uint64, error) { return 0, nil }
func (f *projectsRPC) GetSignatureStatus(context.Context, string) (client.SignatureStatus, error) {
	return client.SignatureStatus{}, nil
}
func (f *projectsRPC) RequestAirdrop(context.Context, string, uint64) (string, error) {
	return "", nil
}
func (f *projectsRPC) GetSignaturesForAddress(context.Context, string, int, string) ([]client.SignatureInfo, error) {
	return nil, nil
}
func (f *projectsRPC) GetTransaction(context.Context, string) (*client.Transaction, error) {
	return nil, nil
}

func projectsToolClient(t *testing.T) *client.TorchClient {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	api := &projectsIndexer{
		markets: []client.MarketRow{
			{Mint: projectsMintA, Name: "Context Compaction", Symbol: "CONTEX", Status: client.StatusBonding},
			{Mint: projectsMintB, Name: "Sentiment Alpha", Symbol: "SENTIM", Status: client.StatusComplete},
		},
		messages: map[string][]client.MessageRow{
			projectsMintA: {
				{MessageID: 7, Mint: projectsMintA, Sender: "funder", MemoText: "task 3: Summarize the log.", CreatedAt: now},
				{MessageID: 6, Mint: projectsMintA, Sender: "funder", MemoText: "accept 1", CreatedAt: now},
				{MessageID: 5, Mint: projectsMintA, Sender: "walletX", MemoText: "complete 1", CreatedAt: now},
				{MessageID: 4, Mint: projectsMintA, Sender: "walletX", MemoText: "claim 1", CreatedAt: now},
				{MessageID: 3, Mint: projectsMintA, Sender: "funder", MemoText: "task 2: Draft the research memo.", CreatedAt: now},
				{MessageID: 2, Mint: projectsMintA, Sender: "funder", MemoText: "task 1: Read the transcript.", CreatedAt: now},
				{MessageID: 1, Mint: projectsMintA, Sender: "funder", MemoText: "goal: Research whether sentiment predicts price.", CreatedAt: now},
			},
			projectsMintB: {
				{MessageID: 2, Mint: projectsMintB, Sender: "funder", MemoText: "task 1: Build the sentiment scorer.", CreatedAt: now},
				{MessageID: 1, Mint: projectsMintB, Sender: "funder", MemoText: "back 0.01: first stake", CreatedAt: now},
			},
		},
	}
	accounts := map[string]client.AccountInfo{
		client.TreasurySolVaultPDA(client.DevnetProgramID, projectsMintA): {
			Lamports: client.RentExemptZeroData + 12_340_000_000, Exists: true,
		},
		client.TreasurySolVaultPDA(client.DevnetProgramID, projectsMintB): {
			Lamports: client.RentExemptZeroData + 5_000_000_000, Exists: true,
		},
	}
	return &client.TorchClient{
		Config: client.Config{Indexer: "http://127.0.0.1:1", RPC: "http://127.0.0.1:2", ProgramID: client.DevnetProgramID},
		API:    api, RPC: &projectsRPC{accounts: accounts},
	}
}

func TestProjectsToolListsFoldedOpenTasks(t *testing.T) {
	tc := projectsToolClient(t)
	p := &Projects{Client: func() (*client.TorchClient, error) { return tc, nil }}
	out, err := p.Exec(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[oxPkrZBG]") || !strings.Contains(out, "Context Compaction") {
		t.Errorf("list missed the bonded project:\n%s", out)
	}
	if !strings.Contains(out, "2 open") {
		t.Errorf("the bonded project's open task count is missing:\n%s", out)
	}
	if !strings.Contains(out, "1 open") {
		t.Errorf("the ready project's open task count is missing:\n%s", out)
	}
	if !strings.Contains(out, "12.340000000") || !strings.Contains(out, "5.000000000") {
		t.Errorf("treasury floats missing:\n%s", out)
	}
}

func TestProjectsToolFilters(t *testing.T) {
	tc := projectsToolClient(t)
	p := &Projects{Client: func() (*client.TorchClient, error) { return tc, nil }}
	out, err := p.Exec(context.Background(), json.RawMessage(`{"action":"list","status":"bonding"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Sentiment Alpha") {
		t.Errorf("bonding filter kept the ready project:\n%s", out)
	}
	if !strings.Contains(out, "Context Compaction") {
		t.Errorf("bonding filter dropped the bonded project:\n%s", out)
	}
	goalOnly, err := p.Exec(context.Background(), json.RawMessage(`{"action":"list","goal":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(goalOnly, "Sentiment Alpha") {
		t.Errorf("goal-only filter kept the project without a goal:\n%s", goalOnly)
	}
}

func TestProjectsToolShowNamesTheGoal(t *testing.T) {
	tc := projectsToolClient(t)
	p := &Projects{Client: func() (*client.TorchClient, error) { return tc, nil }}
	out, err := p.Exec(context.Background(), json.RawMessage(`{"action":"show","mint":"oxPkrZBG"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Research whether sentiment predicts price.") {
		t.Errorf("show did not name the goal:\n%s", out)
	}
	if !strings.Contains(out, "1/3 done") || !strings.Contains(out, "0 open claims") {
		t.Errorf("show did not name the board summary:\n%s", out)
	}
	if !strings.Contains(out, "task 3: Summarize the log.") || !strings.Contains(out, "accept 1") || !strings.Contains(out, "complete 1") {
		t.Errorf("show did not name the last three memos:\n%s", out)
	}
}

func TestProjectsToolDescriptionNamesWhereToPick(t *testing.T) {
	desc := (&Projects{}).Description()
	if !strings.Contains(desc, "before it claims") || !strings.Contains(desc, "list") || !strings.Contains(desc, "show") {
		t.Errorf("description = %q", desc)
	}
}
