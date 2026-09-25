package board

import (
	"fmt"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/orbit/board/domain"
)

func renderBoard(label string, tasks []domain.Task, goal string, now time.Time) string {
	var b strings.Builder
	if goal != "" {
		fmt.Fprintf(&b, "[%s] board — %s\n", label, goal)
	} else {
		fmt.Fprintf(&b, "[%s] board\n", label)
	}

	done := 0
	next := 0
	for _, t := range tasks {
		id, err := atoiID(t.Id)
		if err != nil {
			continue
		}
		if t.Status == StatusDone {
			done++
		}
		if t.Status == StatusPending && (next == 0 || id < next) {
			next = id
		}
		b.WriteString(taskLine(t, now))
	}
	if len(tasks) == 0 {
		b.WriteString("No tasks yet — post and fund one with task.\n")
	}
	b.WriteString(summaryLine(label, len(tasks), done, next))
	return b.String()
}

func taskLine(t domain.Task, now time.Time) string {
	line := fmt.Sprintf("t%s %-7s %s", t.Id, t.Status, t.Title)
	if t.Funder != "" {
		line += fmt.Sprintf(" · funded by %s", shortAddr(t.Funder))
	}
	switch t.Status {
	case StatusActive:
		line += fmt.Sprintf(" · claimed by %s · %s", shortAddr(t.Owner), age(now, t.ClaimedAt))
	case StatusReview:
		line += fmt.Sprintf(" · completed by %s · %s", shortAddr(t.Owner), age(now, t.CompletedAt))
	case StatusDone:
		line += fmt.Sprintf(" · accepted by %s", shortAddr(t.AcceptedBy))
	}
	if t.RejectedBy != "" {
		line += fmt.Sprintf(" · rejected by %s", shortAddr(t.RejectedBy))
	}
	return line + "\n"
}

func summaryLine(label string, total, done, next int) string {
	if total == 0 {
		return fmt.Sprintf("[%s] 0/0 done\n", label)
	}
	s := fmt.Sprintf("[%s] %d/%d done", label, done, total)
	if next > 0 {
		s += fmt.Sprintf(" · next: t%d", next)
	}
	return s + "\n"
}

func age(now time.Time, at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return "—"
	}
	d := now.Sub(t).Truncate(time.Second)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func atoiID(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("empty id")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("id %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
