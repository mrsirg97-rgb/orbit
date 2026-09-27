package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
	solpkg "github.com/mrsirg97-rgb/orbit/sol"
)

type Market struct {
	Client        func() (*client.TorchClient, error)
	Store         *board.Store
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
	return "read a project: rate, size, state, backing, holdings, gossip; act: invest, contract, work, release, short, post. One act per fire. invest and post stand in every state but closed. contract (task id + your own capital) needs a public project; work (task id + the treasury's leverage on your token holding) needs a public project with lending unlocked. release with an id frees a held task and closes its commitment; without an id it sells the investment. short with an id rejects a completion with capital behind it. Every write replies with the tx signature plus the memo."
}

func (m *Market) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"mint": {"type": "string", "description": "Full mint pubkey, or the 8-char FID suffix from PROJECTS."},
			"action": {"type": "string", "enum": ["invest", "contract", "work", "release", "short", "post"], "description": "Optional act. Omit to read only."},
			"id": {"type": "integer", "description": "Task id: required for contract and work; release with an id frees that task; short with an id rejects that completion."},
			"memo": {"type": "string", "description": "Memo text: required for post; the reason for a short that rejects; optional for invest and release."},
			"sol": {"type": "integer", "description": "Lamports: the invest or contract size, or the short's collateral (default the operator stake, 10000000 = 0.01 SOL)."},
			"tokens": {"type": "integer", "description": "work: raw token collateral from your holding (default the whole holding)."},
			"fraction": {"type": "integer", "description": "release of a work commitment: basis points to close (default 10000, full; under full keeps the task held)."},
			"min_out": {"type": "integer", "description": "Slippage guard on the position's atomic swap or surplus (default 0)."}
		},
		"required": ["mint"]
	}`)
}

type marketArgs struct {
	Mint     string `json:"mint"`
	Action   string `json:"action"`
	ID       int    `json:"id"`
	Memo     string `json:"memo"`
	SOL      uint64 `json:"sol"`
	Tokens   uint64 `json:"tokens"`
	Fraction uint16 `json:"fraction"`
	MinOut   uint64 `json:"min_out"`
}

func (m *Market) Exec(ctx context.Context, args json.RawMessage) (string, error) {
	tc, err := m.client()
	if err != nil {
		return "", err
	}
	var in marketArgs
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
	treasury, err := treasuryLamports(ctx, tc, market.Mint)
	if err != nil {
		treasury = 0
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
	gossip := brief.SentimentFrom(views)
	read := fmt.Sprintf(
		"%s: RATE %s SOL, SIZE %s SOL, STATE %s, BACKING %s SOL, LENDS %s, HOLDINGS %d (%s SOL), GOSSIP %+.0f",
		fid8(market.Mint), sol(price), sol(price*1_000_000_000), brief.StateWord(string(market.Status)),
		sol(float64(treasury)/1e9), yesno(treasury >= client.LendingUnlockLamports), holdings, sol(value), gossip)
	if in.Action == "" {
		return read, nil
	}
	reply, err := m.act(ctx, tc, market, holdings, in)
	if err != nil {
		return read + "\n" + fmt.Sprintf("%s FAILED: %v", in.Action, err), nil
	}
	return read + "\n" + reply, nil
}

func (m *Market) stake(in marketArgs) uint64 {
	if in.SOL > 0 {
		return in.SOL
	}
	if m.StakeLamports > 0 {
		return m.StakeLamports
	}
	return client.MemoBuyLamports
}

func (m *Market) act(ctx context.Context, tc *client.TorchClient, market client.MarketRow, holdings uint64, in marketArgs) (string, error) {
	project := board.Project{Mint: market.Mint, Label: market.Name}
	switch in.Action {
	case "invest":
		res, err := tc.WriteAction(ctx, market, client.ActionBack, in.Memo, m.stake(in))
		if err != nil {
			return "", err
		}
		return written("invest", market.Mint, res), nil
	case "post":
		res, err := tc.WriteAction(ctx, market, client.ActionPost, in.Memo, client.MemoBuyLamports)
		if err != nil {
			return "", err
		}
		return written("post", market.Mint, res), nil
	case "contract":
		if in.ID <= 0 {
			return "", fmt.Errorf("contract needs a task id")
		}
		if m.Store == nil {
			return "", fmt.Errorf("no board store")
		}
		return m.Store.Contract(ctx, project, in.ID, m.stake(in))
	case "work":
		if in.ID <= 0 {
			return "", fmt.Errorf("work needs a task id")
		}
		if m.Store == nil {
			return "", fmt.Errorf("no board store")
		}
		collateral := in.Tokens
		if collateral == 0 {
			collateral = holdings
		}
		if collateral == 0 {
			return "", fmt.Errorf("work collateral is the project's token you hold; invest first")
		}
		return m.Store.Work(ctx, project, in.ID, collateral, in.MinOut)
	case "release":
		if in.ID > 0 {
			if m.Store == nil {
				return "", fmt.Errorf("no board store")
			}
			return m.Store.Release(ctx, project, in.ID, in.Fraction, in.MinOut)
		}
		if m.Store != nil {
			held, err := m.Store.Held(ctx, project, tc.AgentPublic())
			if err == nil && len(held) > 0 {
				ids := make([]string, 0, len(held))
				for _, t := range held {
					ids = append(ids, fmt.Sprintf("t%d", t.ID))
				}
				return "", fmt.Errorf("release needs the task id: you hold %s on %s", strings.Join(ids, ", "), fid8(market.Mint))
			}
		}
		res, err := tc.WriteAction(ctx, market, client.ActionExit, in.Memo, 0)
		if err != nil {
			return "", err
		}
		return written("release", market.Mint, res), nil
	case "short":
		if in.ID > 0 {
			if m.Store == nil {
				return "", fmt.Errorf("no board store")
			}
			if strings.TrimSpace(in.Memo) == "" {
				return "", fmt.Errorf("a short that rejects needs a reason in memo")
			}
			return m.Store.ShortReject(ctx, project, in.ID, in.Memo, m.stake(in), in.MinOut)
		}
		index, err := tc.NextPositionIndex(ctx, market.Mint, client.SideShort)
		if err != nil {
			return "", err
		}
		res, err := tc.WritePosition(ctx, market, client.PositionWrite{
			Side: client.SideShort, Open: true, Index: index, Amount: m.stake(in), MinOut: in.MinOut, Memo: in.Memo,
		})
		if err != nil {
			return "", err
		}
		return written("short", market.Mint, res), nil
	}
	return "", fmt.Errorf("unknown act %q (invest, contract, work, release, short, post)", in.Action)
}

func written(act, mint string, res client.WriteResult) string {
	got := res.Memo
	if got == "" {
		got = "(no memo)"
	}
	return fmt.Sprintf("%s %s: %s %s", act, fid8(mint), res.Signature, got)
}

func treasuryLamports(ctx context.Context, tc *client.TorchClient, mint string) (uint64, error) {
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

func yesno(b bool) string {
	if b {
		return "T"
	}
	return "F"
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
