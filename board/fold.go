package board

import (
	"sort"
	"strings"
	"time"
)

// Lease is the claim's expiry: a claim older than this is inapplicable at
// fold time, so the task folds as pending — the todo store's stale-claim
// window, the board's reap.
const Lease = 24 * time.Hour

// Task statuses.
const (
	StatusPending = "pending"
	StatusActive  = "active"
	StatusReview  = "review"
	StatusDone    = "done"
)

// Note is one note/reason row on a task.
type Note struct {
	Sender string
	Text   string
	At     string
}

// Task is the fold's state for one task id on a project. Funder is the
// wallet that posted and funded the task — the wallet whose accept counts.
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
}

// Fold is the pure board fold: a project's parsed memo log in log order
// applied to the task board. Ownership and stake decide, never a label:
// the task memo's sender is the task's funder (brief and accept count only
// from the funder), while claim, note, complete, and reject are honoured
// from anyone whose memo is in the log — gated only by task state.
// Malformed, foreign, or inapplicable memos are skipped, never thrown; the
// lease is the one stateful-looking rule and it is pure — a claim older
// than Lease does not apply.
func Fold(project string, memos []Memo, now time.Time) []Task {
	states := map[int]*Task{}
	for _, m := range memos {
		if !ParseAllowed(m) {
			continue
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
			continue
		}
		switch m.Verb {
		case "task":
			// The id exists: a second create is inapplicable (never re-own).
			continue
		case "brief":
			// The brief is the funder's: ownership, not a role.
			if st.Brief != "" || m.Sender != st.Funder {
				continue
			}
			st.Brief = m.Text
			st.UpdatedAt = m.At
		case "claim":
			if st.Status != StatusPending {
				continue
			}
			if now.Sub(parseAt(m.At)) > Lease {
				continue
			}
			st.Status = StatusActive
			st.Owner = m.Sender
			st.ClaimedAt = m.At
			st.UpdatedAt = m.At
		case "note":
			st.Notes = append(st.Notes, Note{Sender: m.Sender, Text: m.Text, At: m.At})
			st.UpdatedAt = m.At
		case "complete":
			// Honoured from anyone who paid for the memo: only the state
			// gate stays — the task must be active.
			if st.Status != StatusActive {
				continue
			}
			st.Status = StatusReview
			st.CompletedAt = m.At
			st.UpdatedAt = m.At
		case "accept":
			// Only the wallet that posted and funded the task accepts.
			if st.Status != StatusReview || m.Sender != st.Funder {
				continue
			}
			st.Status = StatusDone
			st.AcceptedBy = m.Sender
			st.UpdatedAt = m.At
		case "reject":
			// Reject is dissent, honoured from anyone who paid; the reason
			// lands in the notes. A short (reject plus a vault-routed short
			// on the project) is a later PR.
			if st.Status != StatusReview {
				continue
			}
			st.Status = StatusPending
			st.Owner = ""
			st.ClaimedAt = ""
			st.RejectedBy = m.Sender
			st.UpdatedAt = m.At
			st.Notes = append(st.Notes, Note{Sender: m.Sender, Text: m.Text, At: m.At})
		}
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

// ParseAllowed reports a memo whose shape is in the grammar (verb + id):
// the fold applies only allowed shapes. Goal memos and malformed rows
// return false — replay is total, inapplicable rows are skipped.
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

// NextID is the fold's id mint: the next task number for a project.
func NextID(tasks []Task) int {
	max := 0
	for _, t := range tasks {
		if t.ID > max {
			max = t.ID
		}
	}
	return max + 1
}

// GoalFrom parses the project goal memo.
func GoalFrom(memo string) (string, bool) {
	rest := strings.TrimSpace(memo)
	const tag = "goal:"
	if !strings.HasPrefix(rest, tag) {
		return "", false
	}
	goal := strings.TrimSpace(strings.TrimPrefix(rest, tag))
	return goal, goal != ""
}
