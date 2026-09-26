package board

import (
	"sort"
	"strings"
	"time"
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
}

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
		// A memo landing after the lease expired sees the claim dead: the
		// task returns to pending before the memo applies.
		if st.Status == StatusActive && leaseExpired(st.ClaimedAt, parseAt(m.At)) {
			st.Status = StatusPending
			st.Owner = ""
			st.ClaimedAt = ""
		}
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
			st.Status = StatusActive
			st.Owner = m.Sender
			st.ClaimedAt = m.At
			st.UpdatedAt = m.At
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
			st.Status = StatusPending
			st.Owner = ""
			st.ClaimedAt = ""
			st.RejectedBy = m.Sender
			st.UpdatedAt = m.At
			st.Notes = append(st.Notes, Note{Sender: m.Sender, Text: m.Text, At: m.At})
		}
	}
	// The lease expires the claim only while it is still the task's live
	// state: a task that moved on (complete/accept/reject) keeps its state.
	for _, st := range states {
		if st.Status == StatusActive && leaseExpired(st.ClaimedAt, now) {
			st.Status = StatusPending
			st.Owner = ""
			st.ClaimedAt = ""
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
