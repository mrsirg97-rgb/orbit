package board

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mrsirg97-rgb/orbit/client"
)

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
