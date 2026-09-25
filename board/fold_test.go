package board

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/orbit/board/domain"
)

const (
	arch  = "11111111111111111111111111111111111111111"
	workA = "22222222222222222222222222222222222222222"
	workB = "33333333333333333333333333333333333333333"
	rev   = "44444444444444444444444444444444444444444"
)

func TestFoldEveryTransition(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	now := base.Add(5 * time.Minute)
	memos := []Memo{
		Memo{Verb: "task", ID: 1, Text: "Context compaction", Role: "architect", Sender: arch, At: base.Format(time.RFC3339)},
		Memo{Verb: "brief", ID: 1, Text: "Measure compact vs full", Role: "architect", Sender: arch, At: base.Add(time.Minute).Format(time.RFC3339)},
		Memo{Verb: "task", ID: 2, Text: "Sentiment alpha", Role: "architect", Sender: arch, At: base.Add(2 * time.Minute).Format(time.RFC3339)},
		Memo{Verb: "claim", ID: 1, Role: "worker", Sender: workA, At: base.Add(3 * time.Minute).Format(time.RFC3339)},
		Memo{Verb: "note", ID: 1, Text: "42 fires folded", Role: "worker", Sender: workA, At: base.Add(4 * time.Minute).Format(time.RFC3339)},
		Memo{Verb: "complete", ID: 1, Role: "worker", Sender: workA, At: base.Add(5 * time.Minute).Format(time.RFC3339)},
		Memo{Verb: "accept", ID: 1, Role: "reviewer", Sender: rev, At: now.Format(time.RFC3339)},
		Memo{Verb: "claim", ID: 2, Role: "worker", Sender: workB, At: now.Add(time.Minute).Format(time.RFC3339)},
	}
	tasks := Fold("p", memos, now.Add(2*time.Minute))
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
	if t1.Owner != workA || t1.AcceptedBy != rev {
		t.Errorf("task 1 owner/accepted: %q %q", t1.Owner, t1.AcceptedBy)
	}
	if len(t1.Notes) != 1 || t1.Notes[0].Text != "42 fires folded" {
		t.Errorf("task 1 notes: %+v", t1.Notes)
	}
	t2 := tasks[1]
	if t2.Status != StatusActive || t2.Owner != workB {
		t.Errorf("task 2 status/owner: %s %q", t2.Status, t2.Owner)
	}
}

func TestFoldForeignClaim(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	now := base.Add(10 * time.Minute)
	memos := []Memo{
		Memo{Verb: "task", ID: 1, Text: "Context compaction", Role: "architect", Sender: arch, At: base.Format(time.RFC3339)},
		Memo{Verb: "claim", ID: 1, Role: "worker", Sender: workA, At: base.Add(time.Minute).Format(time.RFC3339)},
		Memo{Verb: "claim", ID: 1, Role: "worker", Sender: workB, At: base.Add(2 * time.Minute).Format(time.RFC3339)},
		Memo{Verb: "complete", ID: 1, Role: "worker", Sender: workB, At: base.Add(3 * time.Minute).Format(time.RFC3339)},
		Memo{Verb: "task", ID: 1, Text: "Re-own", Role: "architect", Sender: arch, At: base.Add(4 * time.Minute).Format(time.RFC3339)},
	}
	tasks := Fold("p", memos, now)
	if len(tasks) != 1 {
		t.Fatalf("tasks: %d, want 1", len(tasks))
	}
	t1 := tasks[0]
	if t1.Status != StatusActive || t1.Owner != workA {
		t.Errorf("foreign claim moved the state: %s %q", t1.Status, t1.Owner)
	}
	if t1.Title != "Context compaction" {
		t.Errorf("foreign create re-owned the title: %q", t1.Title)
	}
}

func TestFoldGolden(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos := []Memo{
		Memo{Verb: "task", ID: 1, Text: "Context compaction", Role: "architect", Sender: arch, At: base.Format(time.RFC3339)},
		Memo{Verb: "brief", ID: 1, Text: "Measure compact vs full", Role: "architect", Sender: arch, At: base.Add(time.Minute).Format(time.RFC3339)},
		Memo{Verb: "task", ID: 2, Text: "Sentiment alpha", Role: "architect", Sender: arch, At: base.Add(2 * time.Minute).Format(time.RFC3339)},
		Memo{Verb: "claim", ID: 1, Role: "worker", Sender: workA, At: base.Add(3 * time.Minute).Format(time.RFC3339)},
		Memo{Verb: "note", ID: 1, Text: "42 fires folded", Role: "worker", Sender: workA, At: base.Add(4 * time.Minute).Format(time.RFC3339)},
		Memo{Verb: "complete", ID: 1, Role: "worker", Sender: workA, At: base.Add(5 * time.Minute).Format(time.RFC3339)},
		Memo{Verb: "accept", ID: 1, Role: "reviewer", Sender: rev, At: base.Add(6 * time.Minute).Format(time.RFC3339)},
	}
	tasks := Fold("p", memos, base.Add(7*time.Minute))
	rows := make([]domain.Task, 0, len(tasks))
	for _, t := range tasks {
		rows = append(rows, domain.Task{
			Project: "p", Id: itoa(t.ID), Title: t.Title, Brief: t.Brief,
			Status: t.Status, Owner: t.Owner, ClaimedAt: t.ClaimedAt,
			CompletedAt: t.CompletedAt, AcceptedBy: t.AcceptedBy,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		})
	}
	got := renderBoard("torch test", rows, "Research context compaction.", base.Add(7*time.Minute))
	want := readGolden(t, "fold_full.txt")
	if got != want {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFoldLeaseExpiry(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	memos := []Memo{
		Memo{Verb: "task", ID: 1, Text: "Context compaction", Role: "architect", Sender: arch, At: base.Format(time.RFC3339)},
		Memo{Verb: "claim", ID: 1, Role: "worker", Sender: workA, At: base.Add(time.Minute).Format(time.RFC3339)},
	}
	inside := Fold("p", memos, base.Add(Lease+time.Minute-2*time.Second))
	if inside[0].Status != StatusActive {
		t.Errorf("claim inside the lease folded as %s, want active", inside[0].Status)
	}
	outside := Fold("p", memos, base.Add(Lease+time.Minute+2*time.Second))
	if outside[0].Status != StatusPending {
		t.Errorf("expired claim folded as %s, want pending", outside[0].Status)
	}
	if outside[0].Owner != "" {
		t.Errorf("expired claim kept the owner %q", outside[0].Owner)
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
