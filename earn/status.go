// Package earn is the one-minute join path as a slash command: the /earn
// wizard (init, vault steps, roles, jobs) and the status rows the TUI's
// footer renders. The operator key is named at the call and never stored.
package earn

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/client"
)

// Rows is the /earn footer: projects held, open claims, last memo, PnL
// since start. The TUI renders the same rows under the footer and /earn
// status prints them.
type Rows struct {
	Held       int
	OpenClaims int
	LastMemo   string
	PnLSOL     float64
}

// Lines is the footer's four rows.
func (r Rows) Lines() []string {
	return []string{
		fmt.Sprintf("projects held: %d", r.Held),
		fmt.Sprintf("open claims: %d", r.OpenClaims),
		"last memo: " + r.LastMemo,
		fmt.Sprintf("PnL since start: %s SOL", signedSOL(r.PnLSOL)),
	}
}

// Status reads the earn footer: the wallet (held projects, lifetime PnL)
// and the board cache (the wallet's open claims and last memo across the
// held projects). The board cache is the window — a project the cache has
// never synced contributes nothing.
func Status(ctx context.Context, tc *client.TorchClient, st *board.Store) (Rows, error) {
	if tc == nil {
		return Rows{}, fmt.Errorf("earn: no orbit config (run /earn)")
	}
	wallet, err := tc.WalletRead(ctx)
	if err != nil {
		return Rows{}, fmt.Errorf("earn: wallet read: %w", err)
	}
	rows := Rows{PnLSOL: float64(wallet.Pnl.TotalRealizedPnl) / 1e9}
	hot := tc.AgentPublic()
	lastMemo := memoSeen{}
	held := 0
	foundMemo := false
	for mint, raw := range wallet.Holdings {
		if raw == 0 {
			continue
		}
		held++
		p := board.Project{Mint: mint}
		n, err := st.Claims(ctx, p, hot)
		if err != nil {
			continue
		}
		rows.OpenClaims += n
		if m, ok, err := st.LastMemo(ctx, p, hot); err == nil && ok {
			if !foundMemo || m.At > lastMemo.at {
				lastMemo = memoSeen{at: m.At, text: m.Text}
				foundMemo = true
			}
		}
	}
	rows.Held = held
	if !foundMemo {
		rows.LastMemo = "none"
	} else {
		rows.LastMemo = fmt.Sprintf("%s · %q", age(lastMemo.at), briefText(lastMemo.text))
	}
	return rows, nil
}

type memoSeen struct {
	at   string
	text string
}

func age(at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return "—"
	}
	d := time.Since(t).Truncate(time.Second)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func signedSOL(v float64) string {
	if v >= 0 {
		return fmt.Sprintf("+%.4f", v)
	}
	return fmt.Sprintf("-%.4f", -v)
}

// briefText keeps the memo row one line: the memo's first line, cut at 40
// runes.
func briefText(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return s
}
