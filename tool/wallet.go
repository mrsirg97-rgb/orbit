package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
)

type Wallet struct {
	Client func() (*client.TorchClient, error)
	Store  *board.Store
}

func (w *Wallet) client() (*client.TorchClient, error) {
	if w.Client == nil {
		return nil, fmt.Errorf("wallet: no client seam (run /earn)")
	}
	tc, err := w.Client()
	if err != nil {
		return nil, fmt.Errorf("wallet: %w", err)
	}
	return tc, nil
}

func (w *Wallet) Name() string { return "wallet" }

func (w *Wallet) Description() string {
	return "the agent's wallet: earnings (FIFO over trades + swaps), commitments (positions) with standing, the earnings nudge, and the reputation ledger (invested, released at a surplus, shorts vindicated, accepts received, washed out)."
}

func (w *Wallet) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["earnings", "commitments", "standing", "reputation"], "description": "Omit for all four."}
		}
	}`)
}

func (w *Wallet) Exec(ctx context.Context, args json.RawMessage) (string, error) {
	tc, err := w.client()
	if err != nil {
		return "", err
	}
	var in struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("wallet: args: %w", err)
	}
	wallet, err := tc.WalletRead(ctx)
	if err != nil {
		return "", fmt.Errorf("wallet: %w", err)
	}
	var lines []string
	if in.Action == "" || in.Action == "earnings" {
		lines = append(lines, fmt.Sprintf("EARNINGS: total realized %s SOL, volume %s SOL, %d trades",
			sol(float64(wallet.Pnl.TotalRealizedPnl)/1e9),
			sol(float64(wallet.Pnl.TotalVolume)/1e9), wallet.Pnl.TotalTradeCount))
		for _, m := range wallet.Pnl.ByMint {
			lines = append(lines, fmt.Sprintf("  %s: realized %s, remaining %d tokens, cost basis %s",
				pid8(m.Mint), sol(float64(m.RealizedPnl)/1e9), m.TokensRemaining,
				sol(float64(m.CostBasisRemaining)/1e9)))
		}
	}
	if in.Action == "" || in.Action == "commitments" {
		positions, err := tc.API.Positions(ctx, client.Q("owner", tc.AgentPublic(), "is_active", "true"))
		if err == nil {
			if len(positions) == 0 {
				lines = append(lines, "COMMITMENTS: none")
			}
			for _, p := range positions {
				lines = append(lines, fmt.Sprintf("COMMITMENT %s %s #%d: standing %s, debt %s SOL, collateral %d",
					pid8(p.Mint), p.Side, p.PositionIndex, p.Health, sol(float64(p.DebtAmount)/1e9), p.CollateralAmount))
			}
		}
	}
	if in.Action == "" || in.Action == "standing" {
		read := brief.ReadState{
			PnL: brief.PnlSummary{TotalRealizedPnl: wallet.Pnl.TotalRealizedPnl},
		}
		for _, m := range wallet.Pnl.ByMint {
			read.PnL.ByMint = append(read.PnL.ByMint, brief.PnlByMint{
				Mint: m.Mint, CostBasisRemaining: m.CostBasisRemaining,
			})
		}
		line, nudge := brief.HealthLine(read)
		lines = append(lines, "EARNINGS: "+line+". "+nudge)
		lines = append(lines, fmt.Sprintf("Vault SOL: %s (rent floor excluded)", sol(float64(wallet.VaultSOL)/1e9)))
	}
	if in.Action == "" || in.Action == "reputation" {
		lines = append(lines, w.reputation(ctx, tc, wallet))
	}
	return strings.Join(lines, "\n"), nil
}

func (w *Wallet) reputation(ctx context.Context, tc *client.TorchClient, wallet client.WalletState) string {
	invested := 0
	for _, raw := range wallet.Holdings {
		if raw > 0 {
			invested++
		}
	}
	events, err := tc.API.PositionEvents(ctx, client.Q("owner", tc.VaultPDA(), "limit", "500"))
	if err != nil {
		events = nil
	}
	var released, shorts, washed int
	var releasedSOL, shortsSOL int64
	for _, ev := range events {
		switch ev.Kind {
		case "close":
			if ev.SurplusSol == nil || *ev.SurplusSol <= 0 {
				continue
			}
			if ev.Side == client.SideShort {
				shorts++
				shortsSOL += *ev.SurplusSol
			} else {
				released++
				releasedSOL += *ev.SurplusSol
			}
		case "liquidate":
			washed++
		}
	}
	accepts := 0
	if w.Store != nil {
		if n, err := w.Store.Accepts(ctx, tc.AgentPublic()); err == nil {
			accepts = n
		}
	}
	return fmt.Sprintf("REPUTATION: invested in %d projects · released at a surplus %d (+%s SOL) · shorts vindicated %d (+%s SOL) · accepts received %d · washed out %d",
		invested, released, sol(float64(releasedSOL)/1e9), shorts, sol(float64(shortsSOL)/1e9), accepts, washed)
}
