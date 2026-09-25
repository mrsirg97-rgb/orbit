package board

import (
	"sort"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/orbit/identity"
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

// Task is the fold's state for one task id on a project.
type Task struct {
	Project     string
	ID          int
	Title       string
	Brief       string
	Status      string
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
// applied to the task board. Malformed, foreign, or inapplicable memos are
// skipped, never thrown; the lease is the one stateful-looking rule and it
// is pure — a claim older than Lease does not apply.
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
				CreatedAt: m.At, UpdatedAt: m.At,
			}
			continue
		}
		switch m.Verb {
		case "task":
			// The id exists: a second create is inapplicable (never re-own).
			continue
		case "brief":
			if st.Brief != "" || m.Role != string(identity.Architect) {
				continue
			}
			st.Brief = m.Text
			st.UpdatedAt = m.At
		case "claim":
			if st.Status != StatusPending || m.Role != string(identity.Worker) {
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
			if st.Status != StatusActive || st.Owner != m.Sender || m.Role != string(identity.Worker) {
				continue
			}
			st.Status = StatusReview
			st.CompletedAt = m.At
			st.UpdatedAt = m.At
		case "accept":
			if st.Status != StatusReview || (m.Role != string(identity.Architect) && m.Role != string(identity.Reviewer)) {
				continue
			}
			st.Status = StatusDone
			st.AcceptedBy = m.Sender
			st.UpdatedAt = m.At
		case "reject":
			if st.Status != StatusReview || m.Role != string(identity.Reviewer) {
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

// ParseAllowed reports a memo whose shape is in the grammar (role + verb +
// id): the fold applies only allowed shapes. Goal memos and malformed rows
// return false — replay is total, inapplicable rows are skipped.
func ParseAllowed(m Memo) bool {
	if !validVerb(m.Verb) || !roleAllowed(m.Role, m.Verb) {
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
	const tag = "[architect] goal:"
	if !strings.HasPrefix(rest, tag) {
		return "", false
	}
	goal := strings.TrimSpace(strings.TrimPrefix(rest, tag))
	return goal, goal != ""
}
