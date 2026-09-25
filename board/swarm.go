package board

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/mrsirg97-rgb/orbit/board/domain"
)

func (s *Store) Claim(ctx context.Context, p Project) (string, error) {
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

func (s *Store) Note(ctx context.Context, p Project, id, text string) (string, error) {
	taskID, ok := parseMemoID(id)
	if !ok {
		return "", fmt.Errorf("board note: bad task id %q", id)
	}
	return s.Act(ctx, p, Shape{Verb: "note", ID: taskID, Text: text})
}

func (s *Store) Complete(ctx context.Context, p Project, id string, worker bool) (string, error) {
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

func (s *Store) Accept(ctx context.Context, p Project, id string) (string, error) {
	taskID, ok := parseMemoID(id)
	if !ok {
		return "", fmt.Errorf("board accept: bad task id %q", id)
	}
	return s.Act(ctx, p, Shape{Verb: "accept", ID: taskID})
}

func (s *Store) Reject(ctx context.Context, p Project, id, reason string) (string, error) {
	taskID, ok := parseMemoID(id)
	if !ok {
		return "", fmt.Errorf("board reject: bad task id %q", id)
	}
	return s.Act(ctx, p, Shape{Verb: "reject", ID: taskID, Text: reason})
}

func (s *Store) Reap(ctx context.Context, p Project) (string, error) {
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
