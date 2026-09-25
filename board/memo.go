// Package board is the shared board: the project's memo log on torch's
// chain folded into a task board. The memo shapes, the fold, the local
// cache, and the swarm surface (claim/note/complete/accept/reject/reap)
// live here. SPEC_BOARD governs.
package board

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mrsirg97-rgb/orbit/client"
)

// MemoCap is the curve path's memo bound (the IDL-validated cap).
const MemoCap = client.CurveMemoCap

// Verbs is the board verb set, one memo per verb. The verb says what
// happened — no role tag rides the memo.
var Verbs = []string{"task", "brief", "claim", "note", "complete", "accept", "reject"}

// Memo is one parsed board memo: the verb, the task id (0 when the verb
// names none), and the text. The sender is filled by the store's sync.
type Memo struct {
	Verb   string
	ID     int
	Text   string
	Sender string
	At     string
}

// Shape is one memo's format: the exact bytes the act writes.
type Shape struct {
	Verb string
	ID   int
	Text string
}

// MemoFor formats one board memo, capped at the curve memo bound. The verb
// must be in Verbs. Anyone may write any verb — ownership and stake are
// the fold's rules, not the grammar's.
func MemoFor(s Shape) (string, error) {
	if !validVerb(s.Verb) {
		return "", fmt.Errorf("memo: unknown verb %q (%s)", s.Verb, strings.Join(Verbs, ", "))
	}
	text := strings.TrimSpace(s.Text)
	var body string
	switch s.Verb {
	case "task", "brief", "note", "reject":
		if s.ID <= 0 {
			return "", fmt.Errorf("memo: %s needs a task id", s.Verb)
		}
		body = fmt.Sprintf("%s %d: %s", s.Verb, s.ID, text)
	case "claim", "complete", "accept":
		if s.ID <= 0 {
			return "", fmt.Errorf("memo: %s needs a task id", s.Verb)
		}
		body = fmt.Sprintf("%s %d", s.Verb, s.ID)
	default:
		return "", fmt.Errorf("memo: unknown verb %q", s.Verb)
	}
	if utf8.RuneCountInString(body) > MemoCap {
		return "", fmt.Errorf("memo: %s over the %d-char cap", s.Verb, MemoCap)
	}
	return body, nil
}

func validVerb(v string) bool {
	for _, verb := range Verbs {
		if v == verb {
			return true
		}
	}
	return false
}

// ParseMemo parses one memo into its shape. A memo outside the grammar
// returns (Memo{}, false): the fold skips it, never throws. The verb is
// the memo's first word; market memos (no board verb) are not board rows.
func ParseMemo(memo string) (Memo, bool) {
	rest := strings.TrimSpace(memo)
	verb, tail, ok := strings.Cut(rest, " ")
	if !ok {
		verb = rest
		tail = ""
	}
	if !validVerb(verb) {
		return Memo{}, false
	}
	id := 0
	text := ""
	switch verb {
	case "task", "brief", "note", "reject":
		id, tail, ok = cutID(tail)
		if !ok {
			return Memo{}, false
		}
		text = strings.TrimSpace(tail)
	case "claim", "complete", "accept":
		id, ok = parseMemoID(strings.TrimSpace(tail))
		if !ok {
			return Memo{}, false
		}
	default:
		return Memo{}, false
	}
	return Memo{Verb: verb, ID: id, Text: text}, true
}

func cutID(s string) (int, string, bool) {
	s = strings.TrimSpace(s)
	idStr, tail, ok := strings.Cut(s, ":")
	if !ok {
		return 0, "", false
	}
	id, ok := parseMemoID(strings.TrimSpace(idStr))
	if !ok {
		return 0, "", false
	}
	return id, strings.TrimSpace(strings.TrimPrefix(tail, " ")), true
}

func parseMemoID(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
