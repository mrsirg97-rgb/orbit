package board

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/mrsirg97-rgb/orbit/board/domain"
	"github.com/mrsirg97-rgb/orbit/identity"
)

// The swarm surface: rig's board seam, chain-backed. The vocabulary is the
// todo store's swarm surface verbatim — claim / note / complete / accept /
// reject / reap — so a drain worker works on a chain board with no change
// to rig: claim is the memo, the lease is the fold's expiry, and reap
// returns expired claims to pending. The roles are the surface's own (a
// worker drain claims and completes, a reviewer drain verdicts, the
// supervisor's notes are the architect's), never the agent's row.

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
	return s.Act(ctx, p, Shape{Role: string(identity.Worker), Verb: "claim", ID: id})
}

// Note appends the supervisor's finding to a task: notes are how agents
// talk about shared work, so no hold is needed.
func (s *Store) Note(ctx context.Context, p Project, id, text, session string) (string, error) {
	taskID, ok := parseMemoID(id)
	if !ok {
		return "", fmt.Errorf("board note: bad task id %q", id)
	}
	return s.Act(ctx, p, Shape{Role: string(identity.Architect), Verb: "note", ID: taskID, Text: text})
}

// Complete submits a claimed task for review. A worker drain completes
// with worker=true; the interactive path lands done directly (the accept
// memo follows the complete, so the log stays uniform).
func (s *Store) Complete(ctx context.Context, p Project, id, session string, worker bool) (string, error) {
	taskID, ok := parseMemoID(id)
	if !ok {
		return "", fmt.Errorf("board complete: bad task id %q", id)
	}
	reply, err := s.Act(ctx, p, Shape{Role: string(identity.Worker), Verb: "complete", ID: taskID})
	if err != nil {
		return "", err
	}
	if !worker {
		accept, err := s.Act(ctx, p, Shape{Role: string(identity.Architect), Verb: "accept", ID: taskID})
		if err != nil {
			return "", err
		}
		reply += "\n" + accept
	}
	return reply, nil
}

// Accept verdicts a review task as done.
func (s *Store) Accept(ctx context.Context, p Project, id, session string) (string, error) {
	taskID, ok := parseMemoID(id)
	if !ok {
		return "", fmt.Errorf("board accept: bad task id %q", id)
	}
	return s.Act(ctx, p, Shape{Role: string(identity.Reviewer), Verb: "accept", ID: taskID})
}

// Reject verdicts a review task back to pending; the reason rides the
// reject memo and lands in the task's notes.
func (s *Store) Reject(ctx context.Context, p Project, id, reason, session string) (string, error) {
	taskID, ok := parseMemoID(id)
	if !ok {
		return "", fmt.Errorf("board reject: bad task id %q", id)
	}
	return s.Act(ctx, p, Shape{Role: string(identity.Reviewer), Verb: "reject", ID: taskID, Text: reason})
}

// Reap returns expired claims to pending. The lease is the fold's expiry:
// the projection already folds an expired claim as pending, and the door
// reports the claims the log shows as expired (a claim older than the
// lease whose task is pending — the expiry materialized). The ended-session
// arm is the todo store's, not the chain's: the chain has no sessions, only
// leases.
func (s *Store) Reap(ctx context.Context, p Project, ended []string, architect string) (string, error) {
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
