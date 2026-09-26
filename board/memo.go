package board

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mrsirg97-rgb/orbit/client"
)

// MemoCap counts bytes, not runes: the memo rides the wire as UTF-8 bytes.
const MemoCap = client.CurveMemoCap

var Verbs = []string{"task", "brief", "claim", "note", "complete", "accept", "reject"}

type Memo struct {
	Verb   string
	ID     int
	Text   string
	Sender string
	At     string
}

type Shape struct {
	Verb string
	ID   int
	Text string
}

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
	if controlChar(body) {
		return "", fmt.Errorf("memo: %s: control characters are not allowed (a raw memo would break the board's one-line render)", s.Verb)
	}
	if len(body) > MemoCap {
		return "", fmt.Errorf("memo: %s over the %d-byte cap", s.Verb, MemoCap)
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

// controlChar rejects anything that would break the one-line-per-task
// render: newlines, tabs, escapes, and the other C0/DEL bytes.
func controlChar(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}

func ParseMemo(memo string) (Memo, bool) {
	if controlChar(memo) {
		return Memo{}, false
	}
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
	return id, strings.TrimSpace(tail), true
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
