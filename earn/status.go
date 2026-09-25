package earn

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
)

type Rows struct {
	Held       int
	OpenClaims int
	LastMemo   string
	PnLSOL     float64
}

func (r Rows) Lines() []string {
	return []string{
		fmt.Sprintf("projects held: %d", r.Held),
		fmt.Sprintf("open claims: %d", r.OpenClaims),
		"last memo: " + r.LastMemo,
		fmt.Sprintf("PnL since start: %s SOL", signedSOL(r.PnLSOL)),
	}
}

func Status(ctx context.Context, tc *client.TorchClient, st *board.Store) (Rows, error) {
	if tc == nil {
		return Rows{}, fmt.Errorf("earn: no orbit config (run /earn)")
	}
	wallet, err := tc.WalletRead(ctx)
	if err != nil {
		return Rows{}, fmt.Errorf("earn: wallet read: %w", err)
	}
	return rowsFrom(ctx, tc, st, wallet.Holdings, wallet.Pnl.TotalRealizedPnl)
}

func RowsFromBrief(ctx context.Context, read brief.ReadState, tc *client.TorchClient, st *board.Store) (Rows, error) {
	holdings := make(map[string]uint64, len(read.Holdings))
	for _, h := range read.Holdings {
		holdings[h.Mint] = h.Raw
	}
	return rowsFrom(ctx, tc, st, holdings, read.PnL.TotalRealizedPnl)
}

func rowsFrom(ctx context.Context, tc *client.TorchClient, st *board.Store, holdings map[string]uint64, pnl int64) (Rows, error) {
	rows := Rows{PnLSOL: float64(pnl) / 1e9}
	hot := tc.AgentPublic()
	lastMemo := memoSeen{}
	held := 0
	foundMemo := false
	for mint, raw := range holdings {
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

func SnapshotPath(home string) string {
	return filepath.Join(home, "orbit", "status.json")
}

func Snapshot(path string) (Rows, bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Rows{}, false, nil
	}
	if err != nil {
		return Rows{}, false, fmt.Errorf("earn: snapshot: %w", err)
	}
	var snap snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return Rows{}, false, fmt.Errorf("earn: snapshot %s: %w", path, err)
	}
	return snap.Rows, true, nil
}

func WriteSnapshot(path string, rows Rows) error {
	snap := snapshot{At: time.Now().UTC().Format(time.RFC3339), Rows: rows}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("earn: snapshot: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("earn: snapshot: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("earn: snapshot: %w", err)
	}
	return nil
}

type snapshot struct {
	At   string `json:"at"`
	Rows Rows   `json:"rows"`
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
