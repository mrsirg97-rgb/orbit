package board

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/mrsirg97-rgb/orbit/board/domain"
)

// The swarm surface: rig's board seam, chain-backed. The vocabulary is the
// todo store's swarm surface verbatim — claim / note / complete / accept /
// reject / reap — so a drain worker works on a chain board with no change
// to rig: claim is the memo, the lease is the fold's expiry, and reap
// returns expired claims to pending. The surface has no roles: any wallet
// may claim, note, complete, and reject; accept is honoured only from the
// task's funder (the fold decides).

// Claim takes the first pending task on the live board and writes the
// claim memo + micro buy. Nothing to do when no task is pending.
func (s *Store) Claim(ctx context.Context, p Project, session string) (string, error) {
	if err := s.Sync(ctx, p, 100); err != nil {
		return "", err
	}
	id, err := s.nextPending(ctx, p)
	if err != nil {
		return "", err
	}
	if id == 0 {
		return "nothing to do", nil
	}
	return s.Act(ctx, p, Shape{Verb: "claim", ID: id})
}

// Note appends a contributor's finding to a task: notes are how agents
// talk about shared work, so no hold is needed.
func (s *Store) Note(ctx context.Context, p Project, id, text, session string) (string, error) {
	taskID, ok := parseMemoID(id)
	if !ok {
		return "", fmt.Errorf("board note: bad task id %q", id)
	}
	return s.Act(ctx, p, Shape{Verb: "note", ID: taskID, Text: text})
}

// Complete submits a claimed task for review. A drain completes with
// worker=true; the interactive path lands done directly (the accept memo
// follows the complete, so the log stays uniform) — the fold honours the
// accept only when the caller is the task's funder.
func (s *Store) Complete(ctx context.Context, p Project, id, session string, worker bool) (string, error) {
	taskID, ok := parseMemoID(id)
	if !ok {
		return "", fmt.Errorf("board complete: bad task id %q", id)
	}
	reply, err := s.Act(ctx, p, Shape{Verb: "complete", ID: taskID})
	if err != nil {
		return "", err
	}
	if !worker {
		accept, err := s.Act(ctx, p, Shape{Verb: "accept", ID: taskID})
		if err != nil {
			return "", err
		}
		reply += "\n" + accept
	}
	return reply, nil
}

// Accept writes the accept memo. The fold honours it only from the task's
// funder — a non-funder's accept is a memo that does not move the state.
func (s *Store) Accept(ctx context.Context, p Project, id, session string) (string, error) {
	taskID, ok := parseMemoID(id)
	if !ok {
		return "", fmt.Errorf("board accept: bad task id %q", id)
	}
	return s.Act(ctx, p, Shape{Verb: "accept", ID: taskID})
}

// Reject writes the dissent memo; the reason rides the reject memo and
// lands in the task's notes. The fold honours it from anyone.
func (s *Store) Reject(ctx context.Context, p Project, id, reason, session string) (string, error) {
	taskID, ok := parseMemoID(id)
	if !ok {
		return "", fmt.Errorf("board reject: bad task id %q", id)
	}
	return s.Act(ctx, p, Shape{Verb: "reject", ID: taskID, Text: reason})
}

// Reap returns expired claims to pending. The lease is the fold's expiry:
// the projection already folds an expired claim as pending, and the door
// reports the claims the log shows as expired (a claim older than the
// lease whose task is pending — the expiry materialized). The ended-session
// arm is the todo store's, not the chain's: the chain has no sessions, only
// leases.
func (s *Store) Reap(ctx context.Context, p Project, ended []string, funder string) (string, error) {
	if err := s.Sync(ctx, p, 100); err != nil {
		return "", err
	}
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	messages, err := domain.NewMessageDomain().WindowMessageByMint(bound, p.Mint, 0, math.MaxInt64, 1<<30).Rows()
	if err != nil {
		return "", err
	}
	expired := map[string]bool{}
	for _, r := range messages {
		m, ok := ParseMemo(r.Memo)
		if !ok || m.Verb != "claim" {
			continue
		}
		if claimExpired(r.CreatedAt) {
			expired[fmt.Sprint(m.ID)] = true
		}
	}
	rows, err := domain.NewTaskDomain().WindowTaskByProject(bound, p.Mint, "", "\uffff", 1<<30).Rows()
	if err != nil {
		return "", err
	}
	count := 0
	for _, r := range rows {
		if r.Status == StatusPending && expired[r.Id] {
			count++
		}
	}
	if count == 0 {
		return "board reap: no expired claims", nil
	}
	return fmt.Sprintf("board reap: %d expired claim%s returned to pending", count, plural(count)), nil
}

func claimExpired(claimedAt string) bool {
	t, err := time.Parse(time.RFC3339, claimedAt)
	if err != nil {
		return true
	}
	return time.Since(t) > Lease
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
