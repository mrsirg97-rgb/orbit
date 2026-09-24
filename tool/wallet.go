package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/world"
)

// Wallet is the wallet read: P&L, positions, health. Vault-attributed.
type Wallet struct {
	Client *client.TorchClient
}

func (w *Wallet) Name() string { return "wallet" }

func (w *Wallet) Description() string {
	return "the agent's wallet: P&L (FIFO over trades + swaps), positions with health, and the health nudge."
}

func (w *Wallet) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["pnl", "positions", "health"], "description": "Omit for all three."}
		}
	}`)
}

func (w *Wallet) Exec(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("wallet: args: %w", err)
	}
	wallet, err := w.Client.WalletRead(ctx)
	if err != nil {
		return "", fmt.Errorf("wallet: %w", err)
	}
	var lines []string
	if in.Action == "" || in.Action == "pnl" {
		lines = append(lines, fmt.Sprintf("PNL: total realized %s SOL, volume %s SOL, %d trades",
			sol(float64(wallet.Pnl.TotalRealizedPnl)/1e9),
			sol(float64(wallet.Pnl.TotalVolume)/1e9), wallet.Pnl.TotalTradeCount))
		for _, m := range wallet.Pnl.ByMint {
			lines = append(lines, fmt.Sprintf("  %s: realized %s, remaining %d tokens, cost basis %s",
				fid8(m.Mint), sol(float64(m.RealizedPnl)/1e9), m.TokensRemaining,
				sol(float64(m.CostBasisRemaining)/1e6)))
		}
	}
	if in.Action == "" || in.Action == "positions" {
		positions, err := w.Client.API.Positions(ctx, client.Q("owner", w.Client.AgentPublic(), "is_active", "true"))
		if err == nil {
			if len(positions) == 0 {
				lines = append(lines, "POSITIONS: none")
			}
			for _, p := range positions {
				lines = append(lines, fmt.Sprintf("POSITION %s %s: %s, debt %s SOL, collateral %s",
					fid8(p.Mint), p.Side, p.Health, sol(float64(p.DebtAmount)/1e9),
					sol(float64(p.CollateralAmount)/1e9)))
			}
		}
	}
	if in.Action == "" || in.Action == "health" {
		read := world.ReadState{
			PnL: world.PnlSummary{TotalRealizedPnl: wallet.Pnl.TotalRealizedPnl},
		}
		for _, m := range wallet.Pnl.ByMint {
			read.PnL.ByMint = append(read.PnL.ByMint, world.PnlByMint{
				Mint: m.Mint, CostBasisRemaining: m.CostBasisRemaining,
			})
		}
		line, nudge := world.HealthLine(read)
		lines = append(lines, "HLTH: "+line+"."+nudge)
		lines = append(lines, fmt.Sprintf("Vault SOL: %s (rent floor excluded)", sol(float64(wallet.VaultSOL)/1e9)))
	}
	return strings.Join(lines, "\n"), nil
}
