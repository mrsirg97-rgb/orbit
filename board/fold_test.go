package board

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/orbit/board/domain"
	"github.com/mrsirg97-rgb/orbit/client"
)

const (
	arch  = "11111111111111111111111111111111111111111"
	workA = "22222222222222222222222222222222222222222"
	workB = "33333333333333333333333333333333333333333"
	rev   = "44444444444444444444444444444444444444444"
	vault = "55555555555555555555555555555555555555555"
)

func public(memos []Memo) ([]Memo, Ledger) {
	ledger := Ledger{Public: true, Carriers: map[string]client.Carrier{}, Ends: map[client.PositionKey]string{}}
	for i := range memos {
		if memos[i].Signature == "" {
			memos[i].Signature = fmt.Sprintf("s%d", i)
		}
		if memos[i].Verb == "claim" {
			ledger.Carriers[memos[i].Signature] = client.Carrier{Lamports: 5 * client.MemoBuyLamports}
		}
	}
	return memos, ledger
}

func at(base time.Time, d time.Duration) string { return base.Add(d).Format(time.RFC3339) }

func TestFoldEveryTransition(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	now := base.Add(5 * time.Minute)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "brief", ID: 1, Text: "Measure compact vs full", Sender: arch, At: at(base, time.Minute)},
		{Verb: "task", ID: 2, Text: "Sentiment alpha", Sender: arch, At: at(base, 2*time.Minute)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, 3*time.Minute)},
		{Verb: "note", ID: 1, Text: "42 fires folded", Sender: workA, At: at(base, 4*time.Minute)},
		{Verb: "complete", ID: 1, Sender: workA, At: at(base, 5*time.Minute)},
		{Verb: "accept", ID: 1, Sender: arch, At: now.Format(time.RFC3339)},
		{Verb: "claim", ID: 2, Sender: workB, At: at(now, time.Minute)},
	})
	tasks := Fold("p", memos, ledger, now.Add(2*time.Minute))
	if len(tasks) != 2 {
		t.Fatalf("tasks: %d, want 2", len(tasks))
	}
	t1 := tasks[0]
	if t1.Title != "Context compaction" || t1.Brief != "Measure compact vs full" {
		t.Errorf("task 1 title/brief: %q %q", t1.Title, t1.Brief)
	}
	if t1.Status != StatusDone {
		t.Errorf("task 1 status: %s, want done", t1.Status)
	}
	if t1.Funder != arch || t1.Owner != workA || t1.AcceptedBy != arch {
		t.Errorf("task 1 funder/owner/accepted: %q %q %q", t1.Funder, t1.Owner, t1.AcceptedBy)
	}
	if len(t1.Notes) != 1 || t1.Notes[0].Text != "42 fires folded" {
		t.Errorf("task 1 notes: %+v", t1.Notes)
	}
	t2 := tasks[1]
	if t2.Status != StatusActive || t2.Owner != workB || t2.Backing != BackingContract || t2.Stake != 5*client.MemoBuyLamports {
		t.Errorf("task 2 status/owner/backing: %s %q %s %d", t2.Status, t2.Owner, t2.Backing, t2.Stake)
	}
}

func TestFoldWorkVerbsForeignUntilPublic(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Fund me first", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute)},
	})
	private := ledger
	private.Public = false
	if tasks := Fold("p", memos, private, base.Add(2*time.Minute)); len(tasks) != 0 {
		t.Errorf("a task memo on a private project folded: %+v", tasks)
	}
	tasks := Fold("p", memos, ledger, base.Add(2*time.Minute))
	if len(tasks) != 1 || tasks[0].Status != StatusActive {
		t.Errorf("the same log after migration: %+v", tasks)
	}
}

func TestFoldClaimNeedsCapital(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute), Signature: "memoOnly"},
		{Verb: "claim", ID: 1, Sender: workB, At: at(base, 2*time.Minute), Signature: "buy"},
	})
	ledger.Carriers["memoOnly"] = client.Carrier{Lamports: client.MemoBuyLamports}
	ledger.Carriers["buy"] = client.Carrier{Lamports: 20_000_000}
	tasks := Fold("p", memos, ledger, base.Add(3*time.Minute))
	t1 := tasks[0]
	if t1.Owner != workB || t1.Backing != BackingContract || t1.Stake != 20_000_000 {
		t.Errorf("memo-only claim honoured or contract not named: %+v", t1)
	}
	memos, ledger = public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute), Signature: "long"},
	})
	ledger.Carriers["long"] = client.Carrier{Long: true, Vault: vault, Index: 2, Collateral: 1_000}
	tasks = Fold("p", memos, ledger, base.Add(3*time.Minute))
	t1 = tasks[0]
	if t1.Status != StatusActive || t1.Backing != BackingWork || t1.Position != (client.PositionKey{Vault: vault, Index: 2}) {
		t.Errorf("work claim did not name the position: %+v", t1)
	}
}

func TestFoldReleaseFromHolderOnly(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute)},
		{Verb: "release", ID: 1, Sender: workB, At: at(base, 2*time.Minute)},
	})
	tasks := Fold("p", memos, ledger, base.Add(3*time.Minute))
	if tasks[0].Status != StatusActive || tasks[0].Owner != workA {
		t.Errorf("a stranger's release moved the state: %+v", tasks[0])
	}
	memos = append(memos, Memo{Verb: "release", ID: 1, Sender: workA, At: at(base, 3*time.Minute), Signature: "r"})
	tasks = Fold("p", memos, ledger, base.Add(4*time.Minute))
	if tasks[0].Status != StatusPending || tasks[0].Owner != "" || tasks[0].Backing != "" {
		t.Errorf("the holder's release did not free the task: %+v", tasks[0])
	}
	memos = append(memos, Memo{Verb: "claim", ID: 1, Sender: workB, At: at(base, 5*time.Minute), Signature: "c2"})
	ledger.Carriers["c2"] = client.Carrier{Lamports: 3 * client.MemoBuyLamports}
	tasks = Fold("p", memos, ledger, base.Add(6*time.Minute))
	if tasks[0].Status != StatusActive || tasks[0].Owner != workB {
		t.Errorf("a released task takes a new claim: %+v", tasks[0])
	}
}

func TestFoldLedgerReleasesEndedPosition(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute), Signature: "long"},
	})
	ledger.Carriers["long"] = client.Carrier{Long: true, Vault: vault, Index: 0}
	key := client.PositionKey{Vault: vault, Index: 0}
	held := Fold("p", memos, ledger, base.Add(48*time.Hour))
	if held[0].Status != StatusActive {
		t.Errorf("a work claim expired by the lease: %+v", held[0])
	}
	ledger.Ends[key] = at(base, 30*time.Minute)
	washed := Fold("p", memos, ledger, base.Add(time.Hour))
	if washed[0].Status != StatusPending || washed[0].Owner != "" {
		t.Errorf("a liquidated position did not release the claim: %+v", washed[0])
	}
	memos = append(memos, Memo{Verb: "claim", ID: 1, Sender: workB, At: at(base, 40*time.Minute), Signature: "c2"})
	ledger.Carriers["c2"] = client.Carrier{Lamports: 3 * client.MemoBuyLamports}
	next := Fold("p", memos, ledger, base.Add(time.Hour))
	if next[0].Status != StatusActive || next[0].Owner != workB {
		t.Errorf("a claim after the liquidation is foreign: %+v", next[0])
	}
}

func TestFoldAcceptSurvivesTheWorkersLaterRelease(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute), Signature: "long"},
		{Verb: "complete", ID: 1, Sender: workA, At: at(base, 2*time.Minute)},
		{Verb: "accept", ID: 1, Sender: arch, At: at(base, 3*time.Minute)},
		{Verb: "release", ID: 1, Sender: workA, At: at(base, 4*time.Minute)},
	})
	ledger.Carriers["long"] = client.Carrier{Long: true, Vault: vault, Index: 0}
	ledger.Ends[client.PositionKey{Vault: vault, Index: 0}] = at(base, 4*time.Minute)
	tasks := Fold("p", memos, ledger, base.Add(time.Hour))
	if tasks[0].Status != StatusDone || tasks[0].Owner != workA || tasks[0].AcceptedBy != arch {
		t.Errorf("the release after accept moved the state: %+v", tasks[0])
	}
}

func TestFoldRejectRidingAShortRecordsIt(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute)},
		{Verb: "complete", ID: 1, Sender: workA, At: at(base, 2*time.Minute)},
		{Verb: "reject", ID: 1, Text: "No proof", Sender: rev, At: at(base, 3*time.Minute), Signature: "short"},
	})
	ledger.Carriers["short"] = client.Carrier{Short: true, Vault: vault, Index: 1, Collateral: 50_000_000}
	tasks := Fold("p", memos, ledger, base.Add(4*time.Minute))
	t1 := tasks[0]
	if t1.Status != StatusPending || t1.RejectedBy != rev || t1.RejectShort != 50_000_000 {
		t.Errorf("reject riding a short: %+v", t1)
	}
	if len(t1.Notes) != 1 || t1.Notes[0].Text != "No proof" {
		t.Errorf("reject reason: %+v", t1.Notes)
	}
}

func TestFoldAcceptFromNonFunderIgnored(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute)},
		{Verb: "complete", ID: 1, Sender: workA, At: at(base, 2*time.Minute)},
		{Verb: "accept", ID: 1, Sender: rev, At: at(base, 3*time.Minute)},
	})
	tasks := Fold("p", memos, ledger, base.Add(4*time.Minute))
	if len(tasks) != 1 {
		t.Fatalf("tasks: %d, want 1", len(tasks))
	}
	t1 := tasks[0]
	if t1.Status != StatusReview {
		t.Errorf("non-funder accept moved the state: %s, want review", t1.Status)
	}
	if t1.AcceptedBy != "" {
		t.Errorf("non-funder accept recorded %q", t1.AcceptedBy)
	}
}

func TestFoldFunderAcceptLands(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute)},
		{Verb: "complete", ID: 1, Sender: workA, At: at(base, 2*time.Minute)},
		{Verb: "accept", ID: 1, Sender: arch, At: at(base, 3*time.Minute)},
	})
	tasks := Fold("p", memos, ledger, base.Add(4*time.Minute))
	if tasks[0].Status != StatusDone || tasks[0].AcceptedBy != arch {
		t.Errorf("funder accept: %s %q", tasks[0].Status, tasks[0].AcceptedBy)
	}
}

func TestFoldAnyWalletClaimsAndCompletes(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute)},
		{Verb: "complete", ID: 1, Sender: workB, At: at(base, 2*time.Minute)},
	})
	tasks := Fold("p", memos, ledger, base.Add(3*time.Minute))
	t1 := tasks[0]
	if t1.Status != StatusReview {
		t.Errorf("any wallet's complete folded as %s, want review", t1.Status)
	}
	if t1.Owner != workA {
		t.Errorf("claim owner %q, want the claimer", t1.Owner)
	}
}

func TestFoldBriefFromNonFunderIgnored(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "brief", ID: 1, Text: "Foreign brief", Sender: workA, At: at(base, time.Minute)},
		{Verb: "brief", ID: 1, Text: "The funder's brief", Sender: arch, At: at(base, 2*time.Minute)},
	})
	tasks := Fold("p", memos, ledger, base.Add(3*time.Minute))
	if tasks[0].Brief != "The funder's brief" {
		t.Errorf("brief %q, want the funder's", tasks[0].Brief)
	}
}

func TestFoldForeignClaim(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	now := base.Add(10 * time.Minute)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute)},
		{Verb: "claim", ID: 1, Sender: workB, At: at(base, 2*time.Minute)},
		{Verb: "task", ID: 1, Text: "Re-own", Sender: arch, At: at(base, 4*time.Minute)},
	})
	tasks := Fold("p", memos, ledger, now)
	if len(tasks) != 2 {
		t.Fatalf("tasks: %d, want 2 (the reused id mints a fresh one)", len(tasks))
	}
	t1 := tasks[0]
	if t1.ID != 1 || t1.Status != StatusActive || t1.Owner != workA {
		t.Errorf("foreign claim moved the state: %s %q", t1.Status, t1.Owner)
	}
	if t1.Title != "Context compaction" {
		t.Errorf("foreign create re-owned the title: %q", t1.Title)
	}
	t2 := tasks[1]
	if t2.ID != 2 || t2.Status != StatusPending || t2.Title != "Re-own" || t2.Funder != arch {
		t.Errorf("stale task memo did not mint a fresh id: %+v", t2)
	}
}

func TestFoldStaleTaskMemoMintsFreshID(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 6, Text: "The real task 6", Sender: "walletA", At: at(base, 0)},
		{Verb: "task", ID: 6, Text: "Stale cache task", Sender: "walletB", At: at(base, time.Minute)},
		{Verb: "claim", ID: 6, Sender: "walletA", At: at(base, 2*time.Minute)},
		{Verb: "claim", ID: 7, Sender: "walletA", At: at(base, 3*time.Minute)},
	})
	tasks := Fold("p", memos, ledger, base.Add(4*time.Minute))
	if len(tasks) != 2 {
		t.Fatalf("tasks: %d, want 2", len(tasks))
	}
	t1, t2 := tasks[0], tasks[1]
	if t1.ID != 6 || t1.Title != "The real task 6" || t1.Status != StatusActive {
		t.Errorf("task 6: %+v", t1)
	}
	if t2.ID != 7 || t2.Title != "Stale cache task" || t2.Status != StatusActive || t2.Owner != "walletA" {
		t.Errorf("stale task: %+v", t2)
	}
}

func TestFoldRejectFromAnyone(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute)},
		{Verb: "complete", ID: 1, Sender: workA, At: at(base, 2*time.Minute)},
		{Verb: "reject", ID: 1, Text: "No proof in the memo", Sender: rev, At: at(base, 3*time.Minute)},
	})
	tasks := Fold("p", memos, ledger, base.Add(4*time.Minute))
	t1 := tasks[0]
	if t1.Status != StatusPending {
		t.Errorf("reject from anyone folded as %s, want pending", t1.Status)
	}
	if t1.RejectedBy != rev || len(t1.Notes) != 1 || t1.Notes[0].Text != "No proof in the memo" {
		t.Errorf("reject recorded: rejected %q notes %+v", t1.RejectedBy, t1.Notes)
	}
	if t1.RejectShort != 0 {
		t.Errorf("a memo-only reject recorded a short: %d", t1.RejectShort)
	}
}

func TestFoldGolden(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "brief", ID: 1, Text: "Measure compact vs full", Sender: arch, At: at(base, time.Minute)},
		{Verb: "task", ID: 2, Text: "Sentiment alpha", Sender: arch, At: at(base, 2*time.Minute)},
		{Verb: "task", ID: 3, Text: "Depth ladder", Sender: arch, At: at(base, 3*time.Minute)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, 3*time.Minute)},
		{Verb: "note", ID: 1, Text: "42 fires folded", Sender: workA, At: at(base, 4*time.Minute)},
		{Verb: "complete", ID: 1, Sender: workA, At: at(base, 5*time.Minute)},
		{Verb: "accept", ID: 1, Sender: arch, At: at(base, 6*time.Minute)},
		{Verb: "claim", ID: 2, Sender: workB, At: at(base, 6*time.Minute), Signature: "long"},
		{Verb: "claim", ID: 3, Sender: workA, At: at(base, 6*time.Minute), Signature: "buy"},
	})
	ledger.Carriers["long"] = client.Carrier{Long: true, Vault: vault, Index: 1}
	ledger.Carriers["buy"] = client.Carrier{Lamports: 25_000_000}
	tasks := Fold("p", memos, ledger, base.Add(7*time.Minute))
	rows := make([]domain.Task, 0, len(tasks))
	backings := map[string]Backing{}
	for _, t := range tasks {
		rows = append(rows, domain.Task{
			Project: "p", Id: itoa(t.ID), Title: t.Title, Brief: t.Brief,
			Status: t.Status, Funder: t.Funder, Owner: t.Owner, ClaimedAt: t.ClaimedAt,
			CompletedAt: t.CompletedAt, AcceptedBy: t.AcceptedBy,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		})
		backings[itoa(t.ID)] = Backing{Backing: t.Backing, Stake: t.Stake, Position: t.Position, RejectShort: t.RejectShort}
	}
	got := renderBoard("torch test", rows, backings, "Research context compaction.", base.Add(7*time.Minute), false)
	want := readGolden(t, "fold_full.txt")
	if got != want {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFoldLeaseExpiry(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, time.Minute)},
	})
	inside := Fold("p", memos, ledger, base.Add(Lease+time.Minute-2*time.Second))
	if inside[0].Status != StatusActive {
		t.Errorf("claim inside the lease folded as %s, want active", inside[0].Status)
	}
	outside := Fold("p", memos, ledger, base.Add(Lease+time.Minute+2*time.Second))
	if outside[0].Status != StatusPending {
		t.Errorf("expired claim folded as %s, want pending", outside[0].Status)
	}
	if outside[0].Owner != "" {
		t.Errorf("expired claim kept the owner %q", outside[0].Owner)
	}
}

func TestFoldClaimLifecycleSurvivesLease(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, 10*time.Minute)},
		{Verb: "complete", ID: 1, Sender: workA, At: at(base, 20*time.Minute)},
		{Verb: "accept", ID: 1, Sender: arch, At: at(base, 30*time.Minute)},
	})
	for _, h := range []time.Duration{time.Hour, 25 * time.Hour} {
		tasks := Fold("p", memos, ledger, base.Add(h))
		if len(tasks) != 1 {
			t.Fatalf("fold at +%s: tasks %d, want 1", h, len(tasks))
		}
		t1 := tasks[0]
		if t1.Status != StatusDone {
			t.Errorf("fold at +%s: status %s, want done (complete/accept must not be dropped)", h, t1.Status)
		}
		if t1.Owner != workA || t1.AcceptedBy != arch {
			t.Errorf("fold at +%s: owner/accepted %q %q", h, t1.Owner, t1.AcceptedBy)
		}
	}
}

func TestFoldClaimAloneExpiresAtTwentyFiveHours(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, 10*time.Minute)},
	})
	tasks := Fold("p", memos, ledger, base.Add(25*time.Hour))
	if len(tasks) != 1 {
		t.Fatalf("tasks: %d, want 1", len(tasks))
	}
	t1 := tasks[0]
	if t1.Status != StatusPending {
		t.Errorf("status %s, want pending (the claim alone is expired)", t1.Status)
	}
	if t1.Owner != "" || t1.ClaimedAt != "" {
		t.Errorf("expired claim kept owner %q claimed_at %q", t1.Owner, t1.ClaimedAt)
	}
}

func TestFoldReclaimAfterLeaseExpiry(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos, ledger := public([]Memo{
		{Verb: "task", ID: 1, Text: "Context compaction", Sender: arch, At: at(base, 0)},
		{Verb: "claim", ID: 1, Sender: workA, At: at(base, 10*time.Minute)},
		{Verb: "claim", ID: 1, Sender: workB, At: at(base, 26*time.Hour)},
	})
	tasks := Fold("p", memos, ledger, base.Add(26*time.Hour))
	if len(tasks) != 1 {
		t.Fatalf("tasks: %d, want 1", len(tasks))
	}
	t1 := tasks[0]
	if t1.Status != StatusActive || t1.Owner != workB {
		t.Errorf("reclaim after expiry: %s %q, want active by %s", t1.Status, t1.Owner, workB)
	}
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "golden", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
