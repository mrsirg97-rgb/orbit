package board

import (
	"sort"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/orbit/client"
)

const Lease = 24 * time.Hour

const (
	StatusPending = "pending"
	StatusActive  = "active"
	StatusReview  = "review"
	StatusDone    = "done"
)

type Note struct {
	Sender string
	Text   string
	At     string
}

type Task struct {
	Project     string
	ID          int
	Title       string
	Brief       string
	Status      string
	Funder      string
	Owner       string
	ClaimedAt   string
	CompletedAt string
	AcceptedBy  string
	RejectedBy  string
	CreatedAt   string
	UpdatedAt   string
	Notes       []Note
	Backing     string
	Stake       uint64
	Position    client.PositionKey
	RejectShort uint64
}

func gated(verb string) bool {
	switch verb {
	case "task", "claim", "complete", "accept", "reject", "release":
		return true
	}
	return false
}

func Fold(project string, memos []Memo, ledger Ledger, now time.Time) []Task {
	states := map[int]*Task{}
	maxID := 0
	for _, m := range memos {
		if !ParseAllowed(m) {
			continue
		}
		if gated(m.Verb) && !ledger.Public {
			continue
		}
		if m.Verb == "task" && states[m.ID] != nil {
			m.ID = maxID + 1
		}
		st, ok := states[m.ID]
		if !ok {
			if m.Verb != "task" {
				continue
			}
			states[m.ID] = &Task{
				Project: project, ID: m.ID, Title: m.Text, Status: StatusPending,
				Funder: m.Sender, CreatedAt: m.At, UpdatedAt: m.At,
			}
			if m.ID > maxID {
				maxID = m.ID
			}
			continue
		}
		expire(st, ledger, parseAt(m.At))
		switch m.Verb {
		case "task":
			continue
		case "brief":
			if st.Brief != "" || m.Sender != st.Funder {
				continue
			}
			st.Brief = m.Text
			st.UpdatedAt = m.At
		case "claim":
			if st.Status != StatusPending {
				continue
			}
			c := ledger.Carriers[m.Signature]
			backing := backingOf(c)
			if backing == "" {
				continue
			}
			st.Status = StatusActive
			st.Owner = m.Sender
			st.ClaimedAt = m.At
			st.UpdatedAt = m.At
			st.Backing = backing
			st.Stake = c.Lamports
			if backing == BackingWork {
				st.Position = client.PositionKey{Vault: c.Vault, Index: c.Index}
			}
		case "note":
			st.Notes = append(st.Notes, Note{Sender: m.Sender, Text: m.Text, At: m.At})
			st.UpdatedAt = m.At
		case "complete":
			if st.Status != StatusActive {
				continue
			}
			st.Status = StatusReview
			st.CompletedAt = m.At
			st.UpdatedAt = m.At
		case "accept":
			if st.Status != StatusReview || m.Sender != st.Funder {
				continue
			}
			st.Status = StatusDone
			st.AcceptedBy = m.Sender
			st.UpdatedAt = m.At
		case "reject":
			if st.Status != StatusReview {
				continue
			}
			release(st)
			st.RejectedBy = m.Sender
			st.RejectShort = 0
			if c := ledger.Carriers[m.Signature]; c.Short {
				st.RejectShort = c.Collateral
			}
			st.UpdatedAt = m.At
			st.Notes = append(st.Notes, Note{Sender: m.Sender, Text: m.Text, At: m.At})
		case "release":
			if (st.Status != StatusActive && st.Status != StatusReview) || m.Sender != st.Owner {
				continue
			}
			release(st)
			st.UpdatedAt = m.At
		}
	}
	for _, st := range states {
		expire(st, ledger, now)
	}
	tasks := make([]Task, 0, len(states))
	for _, st := range states {
		tasks = append(tasks, *st)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return tasks
}

func expire(st *Task, ledger Ledger, ref time.Time) {
	if st.Backing == BackingWork {
		if st.Status != StatusActive && st.Status != StatusReview {
			return
		}
		if end, ok := ledger.Ends[st.Position]; ok && parseAt(end).Before(ref) {
			release(st)
		}
		return
	}
	if st.Status == StatusActive && leaseExpired(st.ClaimedAt, ref) {
		release(st)
	}
}

func release(st *Task) {
	st.Status = StatusPending
	st.Owner = ""
	st.ClaimedAt = ""
	st.Backing = ""
	st.Stake = 0
	st.Position = client.PositionKey{}
}

func leaseExpired(claimedAt string, ref time.Time) bool {
	return ref.Sub(parseAt(claimedAt)) > Lease
}

func ParseAllowed(m Memo) bool {
	if !validVerb(m.Verb) {
		return false
	}
	if m.ID <= 0 {
		return false
	}
	return true
}

func parseAt(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func NextID(tasks []Task) int {
	max := 0
	for _, t := range tasks {
		if t.ID > max {
			max = t.ID
		}
	}
	return max + 1
}

func GoalFrom(memo string) (string, bool) {
	rest := strings.TrimSpace(memo)
	const tag = "goal:"
	if !strings.HasPrefix(rest, tag) {
		return "", false
	}
	goal := strings.TrimSpace(strings.TrimPrefix(rest, tag))
	return goal, goal != ""
}
