package projects

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/project"
	solpkg "github.com/mrsirg97-rgb/orbit/sol"
)

const (
	FallbackBudget = 10
	UnknownTasks   = -1
)

type Filter struct {
	Status   string
	GoalOnly bool
}

func List(ctx context.Context, tc *client.TorchClient, store *board.Store, f Filter) ([]project.Row, error) {
	markets, err := tc.API.Markets(ctx, client.Q("limit", "50"))
	if err != nil {
		return nil, fmt.Errorf("projects list: markets: %w", err)
	}
	rows := make([]project.Row, 0, len(markets))
	for _, m := range markets {
		if f.Status != "" && StatusWord(string(m.Status)) != f.Status {
			continue
		}
		treasury, err := treasurySOL(ctx, tc, m.Mint)
		if err != nil {
			return nil, fmt.Errorf("projects list: treasury %s: %w", m.Mint, err)
		}
		rows = append(rows, project.Row{
			Mint: m.Mint, Name: m.Name, Symbol: m.Symbol, Status: string(m.Status),
			TreasurySOL: treasury, OpenTasks: UnknownTasks,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].TreasurySOL > rows[j].TreasurySOL
	})
	budget := FallbackBudget
	for i := range rows {
		if store != nil {
			sum, ok, err := store.Summary(ctx, rows[i].Mint)
			if err != nil {
				return nil, fmt.Errorf("projects list: board summary %s: %w", rows[i].Mint, err)
			}
			if ok {
				rows[i].Goal = sum.Goal
				rows[i].OpenTasks = sum.OpenTasks
				continue
			}
		}
		if budget <= 0 {
			continue
		}
		budget--
		msgs, err := tc.API.Messages(ctx, client.Q("mint", rows[i].Mint, "limit", "50"))
		if err != nil {
			return nil, fmt.Errorf("projects list: messages %s: %w", rows[i].Mint, err)
		}
		for _, msg := range msgs {
			if rows[i].Goal == "" {
				if g, ok := project.GoalFrom(msg.MemoText); ok {
					rows[i].Goal = g
				}
			}
		}
		rows[i].OpenTasks = project.OpenTasks(rows[i].Mint, msgs)
	}
	if f.GoalOnly {
		kept := rows[:0]
		for _, r := range rows {
			if r.Goal != "" {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	return rows, nil
}

func Show(ctx context.Context, tc *client.TorchClient, store *board.Store, input string) (project.ShowView, error) {
	mint, err := resolveMint(ctx, tc, input)
	if err != nil {
		return project.ShowView{}, err
	}
	s, err := project.Show(ctx, tc.API, tc.RPC, tc.ProgramID, mint)
	if err != nil {
		return project.ShowView{}, err
	}
	if store != nil {
		sum, ok, err := store.Summary(ctx, mint)
		if err != nil {
			return project.ShowView{}, fmt.Errorf("projects show: board summary %s: %w", mint, err)
		}
		if ok {
			s.Goal = sum.Goal
			s.TotalTasks = sum.TotalTasks
			s.DoneTasks = sum.DoneTasks
			s.OpenClaims = sum.OpenClaims
		}
	}
	return s, nil
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

func TasksText(n int) string {
	if n == UnknownTasks {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

func treasurySOL(ctx context.Context, tc *client.TorchClient, mint string) (uint64, error) {
	info, err := tc.RPC.GetAccountInfo(ctx, client.TreasurySolVaultPDA(tc.ProgramID, mint))
	if err != nil {
		return 0, err
	}
	if !info.Exists || info.Lamports < client.RentExemptZeroData {
		return 0, nil
	}
	return info.Lamports - client.RentExemptZeroData, nil
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
