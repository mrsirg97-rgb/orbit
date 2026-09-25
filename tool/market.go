package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
)

type Market struct {
	Client        func() (*client.TorchClient, error)
	StakeLamports uint64
}

func (m *Market) client() (*client.TorchClient, error) {
	if m.Client == nil {
		return nil, fmt.Errorf("market: no client seam (run /earn)")
	}
	tc, err := m.Client()
	if err != nil {
		return nil, fmt.Errorf("market: %w", err)
	}
	return tc, nil
}

func (m *Market) Name() string { return "market" }

func (m *Market) Description() string {
	return "read a project: price, treasury, sentiment, holdings; act: back, exit, post. Every write replies with the tx signature plus the memo."
}

func (m *Market) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"mint": {"type": "string", "description": "Full mint pubkey, or the 8-char FID suffix from PROJECTS."},
			"action": {"type": "string", "enum": ["back", "exit", "post"], "description": "Optional write. Omit to read only."},
			"memo": {"type": "string", "description": "Memo text; required for post, optional for back/exit."},
			"sol": {"type": "integer", "description": "Lamports to spend on back (default 10000000 = 0.01 SOL)."}
		},
		"required": ["mint"]
	}`)
}

func (m *Market) Exec(ctx context.Context, args json.RawMessage) (string, error) {
	tc, err := m.client()
	if err != nil {
		return "", err
	}
	var in struct {
		Mint   string `json:"mint"`
		Action string `json:"action"`
		Memo   string `json:"memo"`
		SOL    uint64 `json:"sol"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("market: args: %w", err)
	}
	in.Mint = strings.TrimSpace(in.Mint)
	if in.Mint == "" {
		return "", fmt.Errorf("market: mint required")
	}
	mint, err := resolveMint(ctx, tc, in.Mint)
	if err != nil {
		return "", fmt.Errorf("market: %w", err)
	}
	detail, err := tc.API.Market(ctx, mint)
	if err != nil {
		return "", fmt.Errorf("market: read %s: %w", mint, err)
	}
	market := detail.Market
	treasurySOL, err := m.treasurySOL(ctx, tc, market.Mint)
	if err != nil {
		treasurySOL = 0
	}
	holdings, err := tc.VaultHoldings(ctx, market.Mint)
	if err != nil {
		holdings = 0
	}
	msgs, _ := tc.API.Messages(ctx, client.Q("mint", market.Mint, "limit", "10"))
	views := make([]brief.MessageView, 0, len(msgs))
	for _, msg := range msgs {
		views = append(views, brief.MessageView{Mint: msg.Mint, Text: msg.MemoText})
	}
	price := client.PriceSOL(market, detail.Reserves)
	value := float64(holdings) / 1e6 * price
	sent := brief.SentimentFrom(views)
	read := fmt.Sprintf(
		"%s: PRICE %s SOL, MCAP %s SOL, STATUS %s, TREASURY %s SOL, HOLDINGS %d (%s SOL), SENTIMENT %+.0f",
		fid8(market.Mint), sol(price), sol(price*1_000_000_000), market.Status,
		sol(float64(treasurySOL)/1e9), holdings, sol(value), sent)
	if in.Action == "" {
		return read, nil
	}
	amount := in.SOL
	if in.Action == "post" {
		amount = client.MemoBuyLamports
	} else if amount == 0 {
		if m.StakeLamports > 0 {
			amount = m.StakeLamports
		} else {
			amount = client.MemoBuyLamports
		}
	}
	memo := in.Memo
	res, err := tc.WriteAction(ctx, market, client.Action(in.Action), memo, amount)
	if err != nil {
		return read + "\n" + fmt.Sprintf("%s FAILED: %v", in.Action, err), nil
	}
	got := res.Memo
	if got == "" {
		got = "(no memo)"
	}
	return read + "\n" + fmt.Sprintf("%s %s: %s %s", in.Action, fid8(market.Mint), res.Signature, got), nil
}

func (m *Market) treasurySOL(ctx context.Context, tc *client.TorchClient, mint string) (uint64, error) {
	info, err := tc.RPC.GetAccountInfo(ctx, client.TreasurySolVaultPDA(tc.ProgramID, mint))
	if err != nil {
		return 0, err
	}
	if !info.Exists {
		return 0, nil
	}
	if info.Lamports < client.RentExemptZeroData {
		return 0, nil
	}
	return info.Lamports - client.RentExemptZeroData, nil
}

func resolveMint(ctx context.Context, c *client.TorchClient, input string) (string, error) {
	if len(input) == 44 {

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

func sol(v float64) string {
	switch {
	case v >= 1:
		return fmt.Sprintf("%.2f", v)
	case v >= 0.001:
		return fmt.Sprintf("%.4f", v)
	case v == 0:
		return "0"
	default:
		return fmt.Sprintf("%.6f", v)
	}
}
