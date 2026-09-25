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
	"github.com/mrsirg97-rgb/orbit/identity"
)

// MemoCap is the curve path's memo bound (the IDL-validated cap).
const MemoCap = client.CurveMemoCap

// Verbs is the board verb set, one memo per verb.
var Verbs = []string{"task", "brief", "claim", "note", "complete", "accept", "reject"}

// Roles is the tag vocabulary.
var Roles = []string{string(identity.Architect), string(identity.Worker), string(identity.Reviewer)}

// VerbRoles is the role a verb's memo tag may name; notes are open (any
// role may talk about shared work), accept is the architect's or the
// reviewer's.
var VerbRoles = map[string][]string{
	"task":     {string(identity.Architect)},
	"brief":    {string(identity.Architect)},
	"claim":    {string(identity.Worker)},
	"note":     Roles,
	"complete": {string(identity.Worker)},
	"accept":   {string(identity.Architect), string(identity.Reviewer)},
	"reject":   {string(identity.Reviewer)},
}

// Memo is one parsed board memo: the role tag, the verb, the task id (0
// when the verb names none), and the text.
type Memo struct {
	Role   string
	Verb   string
	ID     int
	Text   string
	Sender string
	At     string
}

// Shape is one memo's format: the exact bytes the act writes.
type Shape struct {
	Role string
	Verb string
	ID   int
	Text string
}

// MemoFor formats one board memo, role-tagged, capped at the curve memo
// bound. The verb must be in Verbs; the role must match the verb.
func MemoFor(s Shape) (string, error) {
	if !validVerb(s.Verb) {
		return "", fmt.Errorf("memo: unknown verb %q (%s)", s.Verb, strings.Join(Verbs, ", "))
	}
	if !roleAllowed(s.Role, s.Verb) {
		return "", fmt.Errorf("memo: role %s cannot %s", s.Role, s.Verb)
	}
	text := strings.TrimSpace(s.Text)
	var body string
	switch s.Verb {
	case "task", "brief":
		if s.ID <= 0 {
			return "", fmt.Errorf("memo: %s needs a task id", s.Verb)
		}
		body = fmt.Sprintf("%s %d: %s", s.Verb, s.ID, text)
	case "note", "reject":
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
	memo := identity.TagMemo(s.Role, body)
	if utf8.RuneCountInString(memo) > MemoCap {
		return "", fmt.Errorf("memo: %s over the %d-char cap", s.Verb, MemoCap)
	}
	return memo, nil
}

func validVerb(v string) bool {
	for _, verb := range Verbs {
		if v == verb {
			return true
		}
	}
	return false
}

func roleAllowed(role, verb string) bool {
	for _, r := range VerbRoles[verb] {
		if role == r {
			return true
		}
	}
	return false
}

// ParseMemo parses one memo into its shape. A memo outside the grammar
// returns (Memo{}, false): the fold skips it, never throws.
func ParseMemo(memo string) (Memo, bool) {
	rest := strings.TrimSpace(memo)
	role := ""
	for _, r := range Roles {
		tag := "[" + r + "] "
		if strings.HasPrefix(rest, tag) {
			role = r
			rest = strings.TrimSpace(strings.TrimPrefix(rest, tag))
			break
		}
	}
	if role == "" {
		return Memo{}, false
	}
	verb, tail, ok := strings.Cut(rest, " ")
	if !ok {
		verb = rest
		tail = ""
	}
	if !validVerb(verb) {
		return Memo{}, false
	}
	if !roleAllowed(role, verb) {
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
		if !ok || strings.TrimSpace(tail) == "" {
			return Memo{}, false
		}
	default:
		return Memo{}, false
	}
	return Memo{Role: role, Verb: verb, ID: id, Text: text}, true
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

// Allowed reports whether the identity row's role may use the verb (the
// tool's door; the store's swarm ops use the store's own role).
func Allowed(role, verb string) bool {
	if !validVerb(verb) {
		return false
	}
	return roleAllowed(role, verb)
}
