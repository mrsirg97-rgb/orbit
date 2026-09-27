package projects

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/orbit/client"
)

const (
	mintA = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	mintB = "7kuT1dfMhUysWcLEV1eYk8ir7RTjszHmsUdrrPQNThcv"
	mintC = "EWo1KkENqJgXTfLz6tGRqfu8XJVsELwmkHHUgPtHB1sc"
	mintD = "9xQeW3gT4yUioP7aRsDfGhJkLzXcVbNqMwE1rT2yUioP7a"
)

type fakeIndexer struct {
	markets  []client.MarketRow
	messages map[string][]client.MessageRow
}

func (f *fakeIndexer) Markets(context.Context, url.Values) ([]client.MarketRow, error) {
	return f.markets, nil
}
func (f *fakeIndexer) Market(_ context.Context, mint string) (client.MarketDetail, error) {
	for _, m := range f.markets {
		if m.Mint == mint {
			return client.MarketDetail{Market: m}, nil
		}
	}
	return client.MarketDetail{}, fmt.Errorf("no market %s", mint)
}
func (f *fakeIndexer) Messages(_ context.Context, q url.Values) ([]client.MessageRow, error) {
	mint := q.Get("mint")
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows := f.messages[mint]
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}
func (f *fakeIndexer) Trades(context.Context, url.Values) ([]client.TradeRow, error) {
	return nil, nil
}
func (f *fakeIndexer) Positions(context.Context, url.Values) ([]client.PositionRow, error) {
	return nil, nil
}
func (f *fakeIndexer) PositionEvents(context.Context, url.Values) ([]client.PositionEventRow, error) {
	return nil, nil
}
func (f *fakeIndexer) Migrations(context.Context, url.Values) ([]client.MigrationRow, error) {
	return nil, nil
}
func (f *fakeIndexer) Pnl(context.Context, string, string) (client.PnlSummary, error) {
	return client.PnlSummary{}, nil
}
func (f *fakeIndexer) Swaps(context.Context, url.Values) ([]client.SwapRow, error) {
	return nil, nil
}

type fakeRPC struct {
	accounts map[string]client.AccountInfo
}

func (f *fakeRPC) GetAccountInfo(_ context.Context, pubkey string) (client.AccountInfo, error) {
	return f.accounts[pubkey], nil
}
func (f *fakeRPC) GetLatestBlockhash(context.Context) (string, error)      { return "", nil }
func (f *fakeRPC) SendTransaction(context.Context, []byte) (string, error) { return "", nil }
func (f *fakeRPC) GetTokenAccountsByOwner(context.Context, string, string) ([]client.TokenAccount, error) {
	return nil, nil
}
func (f *fakeRPC) GetBalance(context.Context, string) (uint64, error) { return 0, nil }
func (f *fakeRPC) GetSignatureStatus(context.Context, string) (client.SignatureStatus, error) {
	return client.SignatureStatus{}, nil
}
func (f *fakeRPC) RequestAirdrop(context.Context, string, uint64) (string, error) {
	return "", nil
}
func (f *fakeRPC) GetSignaturesForAddress(context.Context, string, int, string) ([]client.SignatureInfo, error) {
	return nil, nil
}
func (f *fakeRPC) GetTransaction(context.Context, string) (*client.Transaction, error) {
	return nil, nil
}

func indexerFixture() *fakeIndexer {
	now := time.Now().UTC().Format(time.RFC3339)
	return &fakeIndexer{
		markets: []client.MarketRow{
			{Mint: mintA, Name: "Context Compaction", Symbol: "CONTEX", Status: client.StatusBonding},
			{Mint: mintB, Name: "Sentiment Alpha", Symbol: "SENTIM", Status: client.StatusComplete},
			{Mint: mintC, Name: "Treasury Accumulation", Symbol: "TREASU", Status: client.StatusMigrated},
			{Mint: mintD, Name: "Reclaimed Noise", Symbol: "NOISE", Status: client.StatusReclaimed},
		},
		messages: map[string][]client.MessageRow{
			mintA: {
				{MessageID: 9, Mint: mintA, Sender: "walletX", MemoText: "note 2: Notes on the draft.", CreatedAt: now},
				{MessageID: 8, Mint: mintA, Sender: "walletY", MemoText: "claim 3", CreatedAt: now},
				{MessageID: 7, Mint: mintA, Sender: "walletX", MemoText: "task 3: Summarize the log.", CreatedAt: now},
				{MessageID: 6, Mint: mintA, Sender: "walletX", MemoText: "task 2: Draft the research memo.", CreatedAt: now},
				{MessageID: 5, Mint: mintA, Sender: "funder", MemoText: "accept 1", CreatedAt: now},
				{MessageID: 4, Mint: mintA, Sender: "walletY", MemoText: "complete 1", CreatedAt: now},
				{MessageID: 3, Mint: mintA, Sender: "walletY", MemoText: "claim 1", CreatedAt: now},
				{MessageID: 2, Mint: mintA, Sender: "funder", MemoText: "task 1: Read the transcript.", CreatedAt: now},
				{MessageID: 1, Mint: mintA, Sender: "funder", MemoText: "goal: Research whether sentiment predicts price.", CreatedAt: now},
			},
			mintB: {
				{MessageID: 2, Mint: mintB, Sender: "funder", MemoText: "task 1: Build the sentiment scorer.", CreatedAt: now},
				{MessageID: 1, Mint: mintB, Sender: "funder", MemoText: "goal: Research whether the board sentiment scores predict moves.", CreatedAt: now},
			},
			mintC: {
				{MessageID: 1, Mint: mintC, Sender: "funder", MemoText: "goal: Accumulate the treasury.", CreatedAt: now},
			},
		},
	}
}

func fixtureClient(t *testing.T) *client.TorchClient {
	t.Helper()
	api := indexerFixture()
	treasuries := map[string]uint64{
		mintA: 12_340_000_000,
		mintB: 5_000_000_000,
		mintC: 25_000_000_000,
		mintD: 1_000_000_000,
	}
	accounts := map[string]client.AccountInfo{}
	for mint, sol := range treasuries {
		accounts[client.TreasurySolVaultPDA(client.DevnetProgramID, mint)] = client.AccountInfo{
			Lamports: client.RentExemptZeroData + sol, Exists: true,
		}
	}
	rpc := &fakeRPC{accounts: accounts}
	return &client.TorchClient{
		Config: client.Config{Indexer: "http://127.0.0.1:1", RPC: "http://127.0.0.1:2", ProgramID: client.DevnetProgramID},
		API:    api, RPC: rpc,
	}
}

func TestListFoldsOpenTasksAndSortsByTreasury(t *testing.T) {
	tc := fixtureClient(t)
	rows, err := List(context.Background(), tc, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows: %d, want 4", len(rows))
	}
	if rows[0].Mint != mintC || rows[1].Mint != mintA || rows[2].Mint != mintB || rows[3].Mint != mintD {
		t.Errorf("rows not sorted by treasury: %s %s %s %s", fid8(rows[0].Mint), fid8(rows[1].Mint), fid8(rows[2].Mint), fid8(rows[3].Mint))
	}
	if rows[0].TreasurySOL != 25_000_000_000 || rows[1].TreasurySOL != 12_340_000_000 {
		t.Errorf("treasury floats: %d %d", rows[0].TreasurySOL, rows[1].TreasurySOL)
	}
	if rows[1].OpenTasks != 2 {
		t.Errorf("open tasks on the bonded project: %d, want 2 (1 done of 3)", rows[1].OpenTasks)
	}
	if rows[1].Goal != "Research whether sentiment predicts price." {
		t.Errorf("goal: %q", rows[1].Goal)
	}
	if rows[2].OpenTasks != 1 {
		t.Errorf("open tasks on the ready project: %d, want 1", rows[2].OpenTasks)
	}
	if rows[3].Goal != "" || rows[3].OpenTasks != 0 {
		t.Errorf("reclaimed project: goal %q open %d, want none", rows[3].Goal, rows[3].OpenTasks)
	}
}

func TestListFiltersStatusAndGoal(t *testing.T) {
	tc := fixtureClient(t)
	ctx := context.Background()
	bonding, err := List(ctx, tc, Filter{Status: "bonding"})
	if err != nil {
		t.Fatal(err)
	}
	if len(bonding) != 1 || bonding[0].Mint != mintA {
		t.Errorf("bonding filter: %+v", bonding)
	}
	ready, err := List(ctx, tc, Filter{Status: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0].Mint != mintB {
		t.Errorf("ready filter: %+v", ready)
	}
	goalOnly, err := List(ctx, tc, Filter{GoalOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(goalOnly) != 3 {
		t.Errorf("goal-only rows: %d, want 3", len(goalOnly))
	}
	for _, r := range goalOnly {
		if r.Goal == "" {
			t.Errorf("goal-only row without a goal: %s", fid8(r.Mint))
		}
	}
}

func TestShowNamesGoalSummaryAndLastMemos(t *testing.T) {
	tc := fixtureClient(t)
	s, err := Show(context.Background(), tc, "oxPkrZBG")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Context Compaction" || s.Status != string(client.StatusBonding) {
		t.Errorf("show header: %s %s", s.Name, s.Status)
	}
	if s.Goal != "Research whether sentiment predicts price." {
		t.Errorf("goal: %q", s.Goal)
	}
	if s.TreasurySOL != 12_340_000_000 {
		t.Errorf("treasury: %d", s.TreasurySOL)
	}
	if s.TotalTasks != 3 || s.DoneTasks != 1 || s.OpenClaims != 1 {
		t.Errorf("board summary: %d/%d done, %d claims", s.DoneTasks, s.TotalTasks, s.OpenClaims)
	}
	if len(s.Memos) != 3 {
		t.Fatalf("memos: %d, want 3", len(s.Memos))
	}
	if s.Memos[0].Text != "note 2: Notes on the draft." || s.Memos[1].Text != "claim 3" || s.Memos[2].Text != "task 3: Summarize the log." {
		t.Errorf("last three memos: %q %q %q", s.Memos[0].Text, s.Memos[1].Text, s.Memos[2].Text)
	}
}

func TestCommandRendersOneRowPerProject(t *testing.T) {
	tc := fixtureClient(t)
	cmd := &Command{Client: func() (*client.TorchClient, error) { return tc, nil }}
	out, err := cmd.Run(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 5 {
		t.Fatalf("table lines: %d, want header + 4 rows:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "FID") || !strings.Contains(lines[0], "GOAL") {
		t.Errorf("header: %s", lines[0])
	}
	for i, want := range []string{"gPtHB1sc", "oxPkrZBG", "rPQNThcv", fid8(mintD)} {
		if !strings.Contains(lines[i+1], want) {
			t.Errorf("row %d missing fid %s:\n%s", i, want, lines[i+1])
		}
	}
}

func TestCommandShowByFIDNamesTheGoal(t *testing.T) {
	tc := fixtureClient(t)
	cmd := &Command{Client: func() (*client.TorchClient, error) { return tc, nil }}
	out, err := cmd.Run(context.Background(), "show oxPkrZBG", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Research whether sentiment predicts price.") {
		t.Errorf("show did not name the goal:\n%s", out)
	}
	if !strings.Contains(out, "1/3 done") || !strings.Contains(out, "1 open claim") {
		t.Errorf("show did not name the board summary:\n%s", out)
	}
}

func TestSubNamesTheVerbs(t *testing.T) {
	subs := (&Command{}).Sub()
	got := map[string]bool{}
	for _, s := range subs {
		got[s.Name] = true
		if strings.TrimSpace(s.Desc) == "" {
			t.Errorf("sub %q has no description", s.Name)
		}
	}
	for _, verb := range []string{"list", "show"} {
		if !got[verb] {
			t.Errorf("Sub() missing %q", verb)
		}
	}
	if len(subs) != 2 {
		t.Errorf("Sub() has %d entries, want 2", len(subs))
	}
}

func TestCommandListFilterTokenAndUnknownFilter(t *testing.T) {
	tc := fixtureClient(t)
	cmd := &Command{Client: func() (*client.TorchClient, error) { return tc, nil }}
	out, err := cmd.Run(context.Background(), "bonding", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "\n") != 2 {
		t.Errorf("bonding list rows: %d, want header + 1:\n%s", strings.Count(out, "\n"), out)
	}
	if _, err := cmd.Run(context.Background(), "list hype", nil); err == nil || !strings.Contains(err.Error(), "unknown filter") {
		t.Fatalf("unknown filter: %v", err)
	}
}
