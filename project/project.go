package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/sol"
)

const (
	MaxNameLen = 32
	GoalTag    = "goal:"
)

// MaxGoalLen is the longest goal in bytes that fits the curve buy's memo
// budget once the "goal: " tag rides the memo (bytes, not runes).
var MaxGoalLen = client.CurveMemoCap - len(GoalTag) - 1

func GoalMemo(goal string) (string, error) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return "", errors.New("project: goal required")
	}
	if strings.Contains(goal, "\n") {
		return "", errors.New("project: goal must be one paragraph")
	}
	if len(goal) > MaxGoalLen {
		return "", fmt.Errorf("project: goal longer than %d bytes", MaxGoalLen)
	}
	return GoalTag + " " + goal, nil
}

func GoalFrom(memo string) (string, bool) {
	memo = strings.TrimSpace(memo)
	if !strings.HasPrefix(memo, GoalTag) {
		return "", false
	}
	goal := strings.TrimSpace(strings.TrimPrefix(memo, GoalTag))
	return goal, goal != ""
}

func SymbolFor(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			r = r - 'a' + 'A'
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		default:
			continue
		}
		b.WriteRune(r)
		if b.Len() >= 6 {
			break
		}
	}
	if b.Len() == 0 {
		return "PROJ"
	}
	return b.String()
}

type Row struct {
	Mint        string
	Name        string
	Symbol      string
	Status      string
	TreasurySOL uint64
	Goal        string
}

func List(ctx context.Context, api client.API, rpc client.RPC, programID string, limit int) ([]Row, error) {
	markets, err := api.Markets(ctx, client.Q("limit", fmt.Sprint(limit)))
	if err != nil {
		return nil, fmt.Errorf("project list: markets: %w", err)
	}
	rows := make([]Row, 0, len(markets))
	for _, m := range markets {
		msgs, err := api.Messages(ctx, client.Q("mint", m.Mint, "limit", "50"))
		if err != nil {
			return nil, fmt.Errorf("project list: messages %s: %w", m.Mint, err)
		}
		goal := ""
		for _, msg := range msgs {
			if g, ok := GoalFrom(msg.MemoText); ok {
				goal = g
				break
			}
		}
		var treasury uint64
		info, err := rpc.GetAccountInfo(ctx, client.TreasurySolVaultPDA(programID, m.Mint))
		if err != nil {
			return nil, fmt.Errorf("project list: treasury %s: %w", m.Mint, err)
		}
		if info.Exists && info.Lamports > client.RentExemptZeroData {
			treasury = info.Lamports - client.RentExemptZeroData
		}
		rows = append(rows, Row{
			Mint: m.Mint, Name: m.Name, Symbol: m.Symbol, Status: string(m.Status),
			TreasurySOL: treasury, Goal: goal,
		})
	}
	return rows, nil
}

type CreateResult struct {
	Mint             string
	Name             string
	Symbol           string
	Goal             string
	TreasuryLamports uint64
	CreateSignature  string
	BuySignature     string
}

func Create(ctx context.Context, tc *client.TorchClient, operator sol.Keypair, name, goal string, treasuryLamports uint64) (CreateResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return CreateResult{}, errors.New("project: name required")
	}
	if strings.Contains(name, "\n") {
		return CreateResult{}, errors.New("project: name must be one line")
	}
	if utf8.RuneCountInString(name) > MaxNameLen {
		return CreateResult{}, fmt.Errorf("project: name longer than %d chars", MaxNameLen)
	}
	if treasuryLamports == 0 {
		return CreateResult{}, errors.New("project: treasury must be > 0")
	}
	if operator.PublicBase58() != tc.VaultCreator {
		return CreateResult{}, errors.New("project: operator key is not ORBIT_VAULT_CREATOR")
	}
	memo, err := GoalMemo(goal)
	if err != nil {
		return CreateResult{}, err
	}
	mint, err := sol.GenerateKeypair()
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: mint keypair: %w", err)
	}
	createIx, err := client.BuildCreateToken(tc.ProgramID, client.CreateTokenAccounts{
		Creator: operator.PublicBase58(), Mint: mint.PublicBase58(), ProgramID: tc.ProgramID,
	}, client.CreateTokenArgs{
		Name: name, Symbol: SymbolFor(name), URI: "",
		SolTarget: client.BondingTargetLamports, CommunityToken: false,
	}, tc.IDL)
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: create_token: %w", err)
	}
	blockhash, err := tc.RPC.GetLatestBlockhash(ctx)
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: blockhash: %w", err)
	}
	msg, err := sol.Compile(blockhash, operator.PublicBase58(), []sol.Instruction{createIx})
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: compile create: %w", err)
	}
	signed, err := sol.SignVersionedTxMulti(msg, operator, mint)
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: sign create: %w", err)
	}
	createSig, err := tc.RPC.SendTransaction(ctx, signed)
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: send create_token: %w", err)
	}
	if err := waitConfirmed(ctx, tc.RPC, createSig, 30*time.Second); err != nil {
		return CreateResult{}, fmt.Errorf("project: create %s: %w", createSig, err)
	}
	market, err := waitMarket(ctx, tc.API, mint.PublicBase58(), 30*time.Second)
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: create %s confirmed, market read: %w", createSig, err)
	}
	if market.VirtualSol <= 0 || market.VirtualToken <= 0 || market.RealSol < 0 || market.SolTarget < 0 {
		return CreateResult{}, fmt.Errorf("project: market %s not quoted yet", mint.PublicBase58())
	}
	gc, err := globalConfig(ctx, tc)
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: global config: %w", err)
	}
	vaultSol, err := tc.RPC.GetAccountInfo(ctx, client.VaultSolPDA(tc.ProgramID, tc.VaultCreator))
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: vault sol: %w", err)
	}
	if vaultSol.Lamports < client.RentExemptZeroData+treasuryLamports {
		return CreateResult{}, fmt.Errorf("project: vault has %s SOL, need %s (deposit first)",
			client.FormatSOL(spendable(vaultSol)), client.FormatSOL(treasuryLamports))
	}
	_, minOut, _, err := client.QuoteBuy(treasuryLamports,
		uint64(market.VirtualSol), uint64(market.VirtualToken), uint64(market.RealSol),
		uint64(market.SolTarget), market.IsCommunityToken, client.DefaultSlippageBPS)
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: quote: %w", err)
	}
	buyDisc, err := tc.IDL.Discriminator("buy_via_vault")
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: buy: %w", err)
	}
	buyIxs, err := client.BuildBuyViaVault(buyDisc, client.BuyAccounts{
		Mint: mint.PublicBase58(), Creator: operator.PublicBase58(), DevWallet: gc.DevWallet,
		Buyer: operator.PublicBase58(), VaultCreator: tc.VaultCreator, ProgramID: tc.ProgramID,
		WithATA: true,
	}, treasuryLamports, minOut, memo)
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: buy: %w", err)
	}
	blockhash, err = tc.RPC.GetLatestBlockhash(ctx)
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: blockhash: %w", err)
	}
	msg, err = sol.Compile(blockhash, operator.PublicBase58(), buyIxs)
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: compile buy: %w", err)
	}
	buySig, err := tc.RPC.SendTransaction(ctx, sol.SignVersionedTx(msg, operator))
	if err != nil {
		return CreateResult{}, fmt.Errorf("project: send buy: %w", err)
	}
	return CreateResult{
		Mint: mint.PublicBase58(), Name: name, Symbol: SymbolFor(name),
		Goal: memo, TreasuryLamports: treasuryLamports,
		CreateSignature: createSig, BuySignature: buySig,
	}, nil
}

func globalConfig(ctx context.Context, tc *client.TorchClient) (client.GlobalConfig, error) {
	info, err := tc.RPC.GetAccountInfo(ctx, client.GlobalConfigPDA(tc.ProgramID))
	if err != nil {
		return client.GlobalConfig{}, err
	}
	if !info.Exists {
		return client.GlobalConfig{}, errors.New("global config account missing")
	}
	return client.DecodeGlobalConfig(info.Data)
}

func spendable(acct client.AccountInfo) uint64 {
	if !acct.Exists || acct.Lamports < client.RentExemptZeroData {
		return 0
	}
	return acct.Lamports - client.RentExemptZeroData
}

func waitConfirmed(ctx context.Context, rpc client.RPC, signature string, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		st, err := rpc.GetSignatureStatus(ctx, signature)
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}
		if st.Exists && st.Err != "" {
			return fmt.Errorf("tx failed: %s", st.Err)
		}
		if st.Exists && st.Confirmed {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return errors.New("not confirmed in time (the tx may still land; check before retrying)")
}

func waitMarket(ctx context.Context, api client.API, mint string, budget time.Duration) (client.MarketRow, error) {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		detail, err := api.Market(ctx, mint)
		if err == nil && detail.Market.Mint == mint {
			return detail.Market, nil
		}
		select {
		case <-ctx.Done():
			return client.MarketRow{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return client.MarketRow{}, errors.New("market not indexed in time")
}
