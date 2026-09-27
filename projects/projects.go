package projects

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/project"
	solpkg "github.com/mrsirg97-rgb/orbit/sol"
)

type Filter struct {
	Status   string
	GoalOnly bool
}

func List(ctx context.Context, tc *client.TorchClient, f Filter) ([]project.Row, error) {
	rows, err := project.List(ctx, tc.API, tc.RPC, tc.ProgramID, 50)
	if err != nil {
		return nil, err
	}
	out := make([]project.Row, 0, len(rows))
	for _, r := range rows {
		if f.Status != "" && StatusWord(r.Status) != f.Status {
			continue
		}
		if f.GoalOnly && r.Goal == "" {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].TreasurySOL > out[j].TreasurySOL
	})
	return out, nil
}

func Show(ctx context.Context, tc *client.TorchClient, input string) (project.ShowView, error) {
	mint, err := resolveMint(ctx, tc, input)
	if err != nil {
		return project.ShowView{}, err
	}
	return project.Show(ctx, tc.API, tc.RPC, tc.ProgramID, mint)
}

func ShowText(s project.ShowView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s (%s) — %s\n", fid8(s.Mint), s.Name, s.Symbol, StatusWord(s.Status))
	fmt.Fprintf(&b, "goal:       %s\n", dash(s.Goal))
	fmt.Fprintf(&b, "treasury:   %s SOL\n", client.FormatSOL(s.TreasurySOL))
	fmt.Fprintf(&b, "board:      %d/%d done · %s\n", s.DoneTasks, s.TotalTasks, claims(s.OpenClaims))
	for _, m := range s.Memos {
		fmt.Fprintf(&b, "memo:       %s %s: %s\n", m.At, shortAddr(m.Sender), m.Text)
	}
	return b.String()
}

func StatusWord(status string) string {
	switch status {
	case string(client.StatusBonding):
		return "bonding"
	case string(client.StatusComplete):
		return "ready"
	case string(client.StatusMigrated):
		return "migrated"
	case string(client.StatusReclaimed):
		return "reclaimed"
	default:
		return status
	}
}

func resolveMint(ctx context.Context, c *client.TorchClient, input string) (string, error) {
	if decoded, err := solpkg.Decode(input); err == nil && len(decoded) == 32 {
		if _, err := c.API.Market(ctx, input); err == nil {
			return input, nil
		}
	}
	markets, err := c.API.Markets(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", input, err)
	}
	for _, m := range markets {
		if fid8(m.Mint) == input {
			return m.Mint, nil
		}
	}
	return "", fmt.Errorf("no project with FID %s", input)
}

func fid8(mint string) string {
	if len(mint) <= 8 {
		return mint
	}
	return mint[len(mint)-8:]
}

func shortAddr(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:4] + "…" + s[len(s)-4:]
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func claims(n int) string {
	if n == 1 {
		return "1 open claim"
	}
	return fmt.Sprintf("%d open claims", n)
}
