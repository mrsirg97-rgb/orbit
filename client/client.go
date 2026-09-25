package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"

	"github.com/mrsirg97-rgb/orbit/idl"
	"github.com/mrsirg97-rgb/orbit/sol"
)

type Action string

const (
	ActionBack Action = "back"
	ActionExit Action = "exit"
	ActionPost Action = "post"
)

type WriteResult struct {
	Signature string
	Memo      string
	Kind      string
	AmountIn  uint64
	MinOut    uint64
}

type TorchClient struct {
	Config
	IDL *idl.IDL
	API API
	RPC RPC
}

func New(cfg Config) (*TorchClient, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return wire(cfg)
}

func NewRead(cfg Config) (*TorchClient, error) {
	if cfg.RPC == "" {
		return nil, errors.New("config: rpc required")
	}
	if cfg.ProgramID != DevnetProgramID {
		return nil, fmt.Errorf("config: program %s is not the devnet program %s (devnet only)", cfg.ProgramID, DevnetProgramID)
	}
	return wire(cfg)
}

func wire(cfg Config) (*TorchClient, error) {
	id, err := idl.LoadIDL()
	if err != nil {
		return nil, err
	}
	if id.Address != cfg.ProgramID {
		return nil, fmt.Errorf("client: IDL address %s != config program %s", id.Address, cfg.ProgramID)
	}
	return &TorchClient{Config: cfg, IDL: id, API: NewAPI(cfg.Indexer), RPC: NewJSONRPC(cfg.RPC)}, nil
}

func (c *TorchClient) AgentPublic() string { return c.AgentKey.PublicBase58() }

func (c *TorchClient) VaultPDA() string { return TorchVaultPDA(c.ProgramID, c.VaultCreator) }

func (c *TorchClient) Route(m MarketRow) (string, error) {
	switch m.Status {
	case StatusBonding, StatusComplete:
		return "curve", nil
	case StatusMigrated:
		return "swap", nil
	case StatusReclaimed:
		return "", errors.New("route: market is reclaimed; writes refused")
	default:
		return "", fmt.Errorf("route: unknown status %q", m.Status)
	}
}

func (c *TorchClient) Intel(ctx context.Context, mint string, limit int) ([]MessageRow, error) {
	q := Q("mint", mint, "limit", fmt.Sprint(limit))
	return c.API.Messages(ctx, q)
}

type WalletState struct {
	Pnl      PnlSummary
	Holdings map[string]uint64
	VaultSOL uint64
	AgentSOL uint64
}

func (c *TorchClient) WalletRead(ctx context.Context) (WalletState, error) {
	wallet := c.AgentPublic()
	pnl, err := c.API.Pnl(ctx, wallet, c.VaultPDA())
	if err != nil {
		return WalletState{}, err
	}
	state := WalletState{Pnl: pnl, Holdings: map[string]uint64{}}
	tokenAccounts, err := c.RPC.GetTokenAccountsByOwner(ctx, wallet, Token2022Program)
	if err == nil {
		for _, a := range tokenAccounts {
			state.Holdings[a.Mint] += a.Amount
		}
	} else {

		return WalletState{}, fmt.Errorf("wallet: token accounts: %w", err)
	}
	vaultAcct, err := c.RPC.GetAccountInfo(ctx, c.VaultPDA())
	if err != nil {
		return WalletState{}, fmt.Errorf("wallet: vault: %w", err)
	}
	if !vaultAcct.Exists {
		return WalletState{}, errors.New("wallet: vault does not exist (bootstrap: create_vault + link_wallet + deposit_vault)")
	}
	vaultSol, err := c.RPC.GetAccountInfo(ctx, VaultSolPDA(c.ProgramID, c.VaultCreator))
	if err != nil {
		return WalletState{}, fmt.Errorf("wallet: vault sol: %w", err)
	}
	if vaultSol.Exists {
		state.VaultSOL = vaultSol.Lamports - RentExemptZeroData
	}
	balance, err := c.RPC.GetBalance(ctx, wallet)
	if err == nil {
		state.AgentSOL = balance
	}
	return state, nil
}

func (c *TorchClient) WriteAction(ctx context.Context, market MarketRow, action Action, memo string, amountSOL uint64) (WriteResult, error) {
	if !c.AllowWrite {
		return WriteResult{}, errors.New("write: disabled (devnet gate off)")
	}
	route, err := c.Route(market)
	if err != nil {
		return WriteResult{}, err
	}
	switch action {
	case ActionBack:
		return c.writeBuy(ctx, market, route, memo, amountSOL)
	case ActionPost:
		return c.writeBuy(ctx, market, route, memo, MemoBuyLamports)
	case ActionExit:
		return c.writeSell(ctx, market, route, memo)
	default:
		return WriteResult{}, fmt.Errorf("write: unknown action %q", action)
	}
}

const MemoBuyLamports uint64 = 1_000_000

func (c *TorchClient) writeBuy(ctx context.Context, market MarketRow, route, memo string, amount uint64) (WriteResult, error) {
	_, minOut, err := c.quoteBuy(ctx, market, route, amount)
	if err != nil {
		return WriteResult{}, err
	}
	kind := "buy"
	if route == "curve" {
		gc, err := c.globalConfig(ctx)
		if err != nil {
			return WriteResult{}, err
		}
		ixs, err := BuildBuyViaVault(c.disc("buy_via_vault"), BuyAccounts{
			Mint: market.Mint, Creator: market.Creator, DevWallet: gc.DevWallet,
			Buyer: c.AgentPublic(), VaultCreator: c.VaultCreator, ProgramID: c.ProgramID,
			WithATA: true,
		}, amount, minOut, memo)
		if err != nil {
			return WriteResult{}, err
		}
		return c.send(ctx, ixs, kind, memo, amount, minOut)
	}
	ixs, err := BuildVaultSwap(c.disc("vault_swap"), SwapAccounts{
		Mint: market.Mint, Signer: c.AgentPublic(), VaultCreator: c.VaultCreator,
		ProgramID: c.ProgramID, DeepPoolID: DeepPoolProgramID,
	}, amount, minOut, true, memo, true)
	if err != nil {
		return WriteResult{}, err
	}
	return c.send(ctx, ixs, "swap_buy", memo, amount, minOut)
}

func (c *TorchClient) writeSell(ctx context.Context, market MarketRow, route, memo string) (WriteResult, error) {
	holdings, err := c.holdingsFor(ctx, market.Mint)
	if err != nil {
		return WriteResult{}, err
	}
	if holdings == 0 {
		return WriteResult{}, errors.New("write: no holdings to cut")
	}
	minOut, err := c.quoteSell(ctx, market, route, holdings)
	if err != nil {
		return WriteResult{}, err
	}
	if route == "curve" {
		ixs, err := BuildSellViaVault(c.disc("sell_via_vault"), SellAccounts{
			Mint: market.Mint, Buyer: c.AgentPublic(), VaultCreator: c.VaultCreator,
			ProgramID: c.ProgramID,
		}, holdings, minOut, memo)
		if err != nil {
			return WriteResult{}, err
		}
		return c.send(ctx, ixs, "sell", memo, holdings, minOut)
	}
	ixs, err := BuildVaultSwap(c.disc("vault_swap"), SwapAccounts{
		Mint: market.Mint, Signer: c.AgentPublic(), VaultCreator: c.VaultCreator,
		ProgramID: c.ProgramID, DeepPoolID: DeepPoolProgramID,
	}, holdings, minOut, false, memo, true)
	if err != nil {
		return WriteResult{}, err
	}
	return c.send(ctx, ixs, "swap_sell", memo, holdings, minOut)
}

func (c *TorchClient) quoteBuy(ctx context.Context, market MarketRow, route string, amount uint64) (uint64, uint64, error) {
	if route == "curve" {
		tokens, minOut, _, err := QuoteBuy(amount,
			uint64(market.VirtualSol), uint64(market.VirtualToken), uint64(market.RealSol),
			uint64(market.SolTarget), market.IsCommunityToken, DefaultSlippageBPS)
		return tokens, minOut, err
	}
	detail, err := c.API.Market(ctx, market.Mint)
	if err != nil {
		return 0, 0, err
	}
	if detail.Reserves == nil || detail.Reserves.SolReserve == 0 || detail.Reserves.TokenReserve == 0 {
		return 0, 0, errors.New("quote: pool has no reserves")
	}
	tokens, minOut, err := QuoteSwapBuy(amount,
		uint64(detail.Reserves.SolReserve), uint64(detail.Reserves.TokenReserve), DefaultSlippageBPS)
	return tokens, minOut, err
}

func (c *TorchClient) quoteSell(ctx context.Context, market MarketRow, route string, amount uint64) (uint64, error) {
	if route == "curve" {
		_, minOut, err := QuoteSell(amount,
			uint64(market.VirtualSol), uint64(market.VirtualToken), uint64(market.VirtualToken), DefaultSlippageBPS)
		return minOut, err
	}
	detail, err := c.API.Market(ctx, market.Mint)
	if err != nil {
		return 0, err
	}
	if detail.Reserves == nil || detail.Reserves.SolReserve == 0 || detail.Reserves.TokenReserve == 0 {
		return 0, errors.New("quote: pool has no reserves")
	}
	_, minOut, err := QuoteSwapSell(amount,
		uint64(detail.Reserves.SolReserve), uint64(detail.Reserves.TokenReserve), DefaultSlippageBPS)
	return minOut, err
}

func (c *TorchClient) holdingsFor(ctx context.Context, mint string) (uint64, error) {
	vaultATA, err := c.ATAFor(mint)
	if err != nil {
		return 0, err
	}
	info, err := c.RPC.GetAccountInfo(ctx, vaultATA)
	if err != nil {
		return 0, err
	}
	if !info.Exists {
		return 0, nil
	}
	return decodeTokenAmount(info.Data), nil
}

func (c *TorchClient) send(ctx context.Context, ixs []sol.Instruction, kind, memo string, amountIn, minOut uint64) (WriteResult, error) {
	blockhash, err := c.RPC.GetLatestBlockhash(ctx)
	if err != nil {
		return WriteResult{}, err
	}
	msg, err := sol.Compile(blockhash, c.AgentPublic(), ixs)
	if err != nil {
		return WriteResult{}, err
	}
	signed := sol.SignVersionedTx(msg, c.AgentKey)
	sig, err := c.RPC.SendTransaction(ctx, signed)
	if err != nil {
		return WriteResult{}, fmt.Errorf("send %s: %w", kind, err)
	}
	return WriteResult{Signature: sig, Memo: memo, Kind: kind, AmountIn: amountIn, MinOut: minOut}, nil
}

type GlobalConfig struct {
	Authority           string
	Treasury            string
	DevWallet           string
	ProtocolFeeBPS      uint16
	TotalTokensLaunched uint64
	TotalVolumeSOL      uint64
	Bump                byte
}

func (c *TorchClient) globalConfig(ctx context.Context) (GlobalConfig, error) {
	info, err := c.RPC.GetAccountInfo(ctx, GlobalConfigPDA(c.ProgramID))
	if err != nil {
		return GlobalConfig{}, err
	}
	if !info.Exists {
		return GlobalConfig{}, errors.New("global config: account missing")
	}
	return DecodeGlobalConfig(info.Data)
}

func DecodeGlobalConfig(data []byte) (GlobalConfig, error) {
	if len(data) < 8+32+32+32+2+8+8+1 {
		return GlobalConfig{}, errors.New("global config: account data too short")
	}
	b := data[8:]
	adv := func(n int) []byte {
		out := b[:n]
		b = b[n:]
		return out
	}
	gc := GlobalConfig{}
	gc.Authority = sol.Encode(adv(32))
	gc.Treasury = sol.Encode(adv(32))
	gc.DevWallet = sol.Encode(adv(32))
	gc.ProtocolFeeBPS = binary.LittleEndian.Uint16(adv(2))
	gc.TotalTokensLaunched = binary.LittleEndian.Uint64(adv(8))
	gc.TotalVolumeSOL = binary.LittleEndian.Uint64(adv(8))
	gc.Bump = adv(1)[0]
	return gc, nil
}

func (c *TorchClient) ATAFor(mint string) (string, error) {
	return ATA(mint, TorchVaultPDA(c.ProgramID, c.VaultCreator), Token2022Program)
}

func decodeTokenAmount(data []byte) uint64 {
	if len(data) < 72 {
		return 0
	}
	return binary.LittleEndian.Uint64(data[64:72])
}

func (c *TorchClient) disc(name string) []byte {
	d, err := c.IDL.Discriminator(name)
	if err != nil {
		panic(err)
	}
	return d
}

func Q(kv ...string) url.Values {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		q.Set(kv[i], kv[i+1])
	}
	return q
}

func (c *TorchClient) VaultHoldings(ctx context.Context, mint string) (uint64, error) {
	return c.holdingsFor(ctx, mint)
}
