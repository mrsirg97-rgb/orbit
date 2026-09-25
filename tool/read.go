package tool

import (
	"context"
	"sort"

	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
)

// Snapshot assembles the brief's ReadState from the live read side:
// markets, the wallet read, holdings value, sentiment, intel, positions.
// Pure projection — no writes, no state.
func Snapshot(ctx context.Context, c *client.TorchClient, identity brief.Identity) (brief.ReadState, error) {
	markets, err := c.API.Markets(ctx, nil)
	if err != nil {
		return brief.ReadState{}, err
	}
	wallet, err := c.WalletRead(ctx)
	if err != nil {
		return brief.ReadState{}, err
	}
	positions, err := c.API.Positions(ctx, client.Q("owner", c.AgentPublic(), "is_active", "true"))
	if err != nil {
		return brief.ReadState{}, err
	}
	state := brief.ReadState{
		Identity:  identity,
		PnL:       brief.PnlSummary{TotalRealizedPnl: wallet.Pnl.TotalRealizedPnl},
		Sentiment: map[string]float64{},
		VaultSOL:  wallet.VaultSOL,
	}
	for _, m := range wallet.Pnl.ByMint {
		state.PnL.ByMint = append(state.PnL.ByMint, brief.PnlByMint{
			Mint: m.Mint, TokensRemaining: uint64(m.TokensRemaining),
			CostBasisRemaining: m.CostBasisRemaining, RealizedPnl: m.RealizedPnl,
		})
	}
	for _, p := range positions {
		state.Positions = append(state.Positions, brief.PositionView{
			Mint: p.Mint, Side: string(p.Side), Health: string(p.Health),
			DebtSOL: float64(p.DebtAmount) / 1e9,
		})
	}
	// Sentiment + intel for the held/watched set (the top 6 by value, then
	// the top 4 new markets).
	for i, m := range markets {
		if i >= 6 {
			break
		}
		msgs, err := c.API.Messages(ctx, client.Q("mint", m.Mint, "limit", "10"))
		if err != nil {
			continue
		}
		views := make([]brief.MessageView, 0, len(msgs))
		for _, msg := range msgs {
			views = append(views, brief.MessageView{Mint: msg.Mint, Sender: msg.Sender, Text: truncate(msg.MemoText, 160)})
		}
		state.Sentiment[m.Mint] = brief.SentimentFrom(views)
		state.Intel = append(state.Intel, views...)
	}
	for _, m := range markets {
		raw := holdingsRaw(ctx, c, m.Mint)
		price := client.PriceSOL(m)
		if m.Status == client.StatusMigrated {
			detail, err := c.API.Market(ctx, m.Mint)
			if err == nil && detail.Reserves != nil {
				price = client.PriceSOL(m, detail.Reserves)
			}
		}
		value := float64(raw) / 1e6 * price
		state.Markets = append(state.Markets, brief.MarketView{
			Mint:      m.Mint,
			Name:      m.Name,
			Symbol:    m.Symbol,
			Status:    string(m.Status),
			PriceSOL:  price,
			MCAPSOL:   price * 1_000_000_000,
			IsHeld:    value > 0,
			ValueSOL:  value,
			PnLSOL:    pnlFor(&wallet.Pnl, m.Mint, value),
			Sentiment: state.Sentiment[m.Mint],
			HasLoan:   hasLoan(&state, m.Mint),
		})
	}
	sort.SliceStable(state.Markets, func(i, j int) bool {
		return state.Markets[i].ValueSOL > state.Markets[j].ValueSOL
	})
	return state, nil
}

func holdingsRaw(ctx context.Context, c *client.TorchClient, mint string) uint64 {
	raw, _ := c.VaultHoldings(ctx, mint)
	return raw
}

func pnlFor(pnl *client.PnlSummary, mint string, value float64) float64 {
	for _, m := range pnl.ByMint {
		if m.Mint == mint {
			unreal := value - float64(m.CostBasisRemaining)/1e6
			return float64(m.RealizedPnl)/1e9 + unreal
		}
	}
	return 0
}

func hasLoan(state *brief.ReadState, mint string) bool {
	for _, p := range state.Positions {
		if p.Mint == mint && p.Side == "long" && p.Health != "none" {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
