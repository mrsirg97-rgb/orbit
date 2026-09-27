package board

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/orbit/client"
)

func workHarness(t *testing.T) (*Store, *boardFakeRPC, Project) {
	t.Helper()
	rpc := newBoardFakeRPC(t, testMint)
	tc := boardClientFor(t, testMint, "orbit-board-work-seed-000000000", rpc)
	db, st := openBoardStoreWith(t, tc)
	t.Cleanup(func() { db.DB.Close() })
	base := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	seedRecordedLog(t, db, testMint, []client.MessageRow{
		{Mint: testMint, MessageID: 1, Sender: "walletX", MemoText: "task 1: Ship the ledger", Slot: 1, Signature: "sigT1", CreatedAt: base},
	})
	return st, rpc, Project{Mint: testMint, Label: "torch test"}
}

func sentDisc(t *testing.T, st *Store, name string) int {
	t.Helper()
	tc, _ := st.client()
	disc, err := tc.IDL.Discriminator(name)
	if err != nil {
		t.Fatal(err)
	}
	rpc := tc.RPC.(*boardFakeRPC)
	n := 0
	for _, tx := range rpc.txs {
		for _, ix := range tx.Ixs {
			if ix.ProgramID == client.DevnetProgramID && len(ix.Data) >= 8 && string(ix.Data[:8]) == string(disc) {
				n++
			}
		}
	}
	return n
}

func TestContractLandsHeldAndNamesTheStake(t *testing.T) {
	st, rpc, project := workHarness(t)
	ctx := context.Background()
	if _, err := st.Contract(ctx, project, 1, client.MemoBuyLamports); err == nil || !strings.Contains(err.Error(), "memo stake") {
		t.Fatalf("a contract at the memo stake: %v", err)
	}
	reply, err := st.Contract(ctx, project, 1, 5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "claim 1") || !strings.Contains(reply, "t1 active") || !strings.Contains(reply, "contract 0.0050 SOL") {
		t.Errorf("contract reply:\n%s", reply)
	}
	if len(rpc.sent) != 1 || sentDisc(t, st, "vault_swap") != 1 {
		t.Errorf("sent %d txs, swaps %d; want one buy carrying the claim", len(rpc.sent), sentDisc(t, st, "vault_swap"))
	}
	tc, _ := st.client()
	held, err := st.Held(ctx, project, tc.AgentPublic())
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 1 || held[0].Backing != BackingContract || held[0].Stake != 5_000_000 {
		t.Errorf("held: %+v", held)
	}
}

func TestWorkOpensALongAndReleaseClosesIt(t *testing.T) {
	st, rpc, project := workHarness(t)
	ctx := context.Background()
	tc, _ := st.client()
	reply, err := st.Work(ctx, project, 1, 1_000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "t1 active") || !strings.Contains(reply, "work #0") {
		t.Errorf("work reply:\n%s", reply)
	}
	if sentDisc(t, st, "open_long_via_vault") != 1 {
		t.Errorf("no open_long_via_vault among %d sent txs", len(rpc.sent))
	}
	held, err := st.Held(ctx, project, tc.AgentPublic())
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 1 || held[0].Backing != BackingWork || held[0].Position != (client.PositionKey{Vault: tc.VaultPDA(), Index: 0}) {
		t.Errorf("held: %+v", held)
	}
	partial, err := st.Release(ctx, project, 1, 5_000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(partial, "partial release") {
		t.Errorf("partial reply: %s", partial)
	}
	board, err := st.Board(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(board, "t1 active") {
		t.Errorf("a partial release freed the task:\n%s", board)
	}
	full, err := st.Release(ctx, project, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full, "release 1") || !strings.Contains(full, "t1 pending") {
		t.Errorf("release reply:\n%s", full)
	}
	if sentDisc(t, st, "close_long_via_vault") != 2 {
		t.Errorf("close_long_via_vault sent %d times, want 2 (partial + full)", sentDisc(t, st, "close_long_via_vault"))
	}
	if _, err := st.Release(ctx, project, 1, 0, 0); err == nil || !strings.Contains(err.Error(), "do not hold") {
		t.Errorf("release of a task not held: %v", err)
	}
}

func TestShortRejectRecordsTheShort(t *testing.T) {
	st, rpc, project := workHarness(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
	tc, _ := st.client()
	db := st.DB
	seedRecordedLog(t, db, testMint, []client.MessageRow{
		{Mint: testMint, MessageID: 2, Sender: "walletY", MemoText: "claim 1", Slot: 2, Signature: "sigC1", CreatedAt: base},
		{Mint: testMint, MessageID: 3, Sender: "walletY", MemoText: "complete 1", Slot: 3, Signature: "sigP1", CreatedAt: base},
	})
	seedCarrier(t, db, testMint, "sigC1", contract())
	reply, err := st.ShortReject(ctx, project, 1, "No proof", 20_000_000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "reject 1: No proof") || !strings.Contains(reply, "t1 pending") || !strings.Contains(reply, "short 0.0200 SOL") {
		t.Errorf("short reject reply:\n%s", reply)
	}
	if sentDisc(t, st, "open_short_via_vault") != 1 || len(rpc.sent) != 1 {
		t.Errorf("sent %d txs, shorts %d", len(rpc.sent), sentDisc(t, st, "open_short_via_vault"))
	}
	_ = tc
}

func TestActRefusesWorkVerbsOnAPrivateProject(t *testing.T) {
	st, rpc, project := workHarness(t)
	ctx := context.Background()
	curve := rpc.accounts[client.BondingCurvePDA(client.DevnetProgramID, testMint)]
	curve.Data[104] = 0
	curve.Data[113] = 0
	rpc.accounts[client.BondingCurvePDA(client.DevnetProgramID, testMint)] = curve
	_, err := st.Act(ctx, project, Shape{Verb: "task", ID: 0, Text: "Too early"})
	if err == nil || !strings.Contains(err.Error(), "not public") {
		t.Fatalf("task on a private project: %v", err)
	}
	if _, err := st.Contract(ctx, project, 1, 5_000_000); err == nil || !strings.Contains(err.Error(), "not public") {
		t.Fatalf("contract on a private project: %v", err)
	}
	if len(rpc.sent) != 0 {
		t.Errorf("a refused act spent: %d txs", len(rpc.sent))
	}
	board, err := st.Board(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(board, "No tasks yet") {
		t.Errorf("a private project folded a task:\n%s", board)
	}
}
