package metadata

import (
	_ "embed"
	"strings"
)

//go:embed extra.sql
var extraSQL []byte

// ExtraStatements: the embedded extra.sql as individual statements — what
// the DDL camera cannot emit (the signature idempotency index). Order is
// preserved; comment-only fragments drop.
func ExtraStatements() []string {
	var out []string
	for _, stmt := range strings.Split(string(extraSQL), ";") {
		var lines []string
		for _, l := range strings.Split(stmt, "\n") {
			if l = strings.TrimSpace(l); l == "" || strings.HasPrefix(l, "--") {
				continue
			}
			lines = append(lines, l)
		}
		if len(lines) > 0 {
			out = append(out, strings.Join(lines, "\n"))
		}
	}
	return out
}
